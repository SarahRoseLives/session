package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListKeepsLiveSessionsAndRemovesStaleOnes(t *testing.T) {
	store := newStore(t.TempDir())
	if err := store.Ensure(); err != nil {
		t.Fatalf("ensure store: %v", err)
	}

	now := time.Now().UTC()
	live := Metadata{
		ID:         "live-session",
		Shell:      "/bin/sh",
		CreatedAt:  now,
		DaemonPID:  os.Getpid(),
		SocketPath: store.SocketPath("live-session"),
		LogPath:    store.LogPath("live-session"),
		MetaPath:   store.MetaPath("live-session"),
	}
	stale := Metadata{
		ID:         "stale-session",
		Shell:      "/bin/sh",
		CreatedAt:  now.Add(-time.Hour),
		DaemonPID:  999999,
		SocketPath: store.SocketPath("stale-session"),
		LogPath:    store.LogPath("stale-session"),
		MetaPath:   store.MetaPath("stale-session"),
	}

	if err := os.WriteFile(live.SocketPath, nil, 0o600); err != nil {
		t.Fatalf("create live socket placeholder: %v", err)
	}
	if err := store.Save(live); err != nil {
		t.Fatalf("save live session: %v", err)
	}
	if err := store.Save(stale); err != nil {
		t.Fatalf("save stale session: %v", err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}

	if len(sessions) != 0 {
		t.Fatalf("sessions length = %d, want 0 because placeholder is not a socket", len(sessions))
	}
	if _, err := os.Stat(store.MetaPath("stale-session")); !os.IsNotExist(err) {
		t.Fatalf("stale metadata still exists, stat err = %v", err)
	}
}

func TestSelectSessionSupportsIndexAndPrefix(t *testing.T) {
	sessions := []Metadata{
		{ID: "20260430-010203-aa11bb"},
		{ID: "20260430-020304-cc22dd"},
	}

	selected, err := SelectSession(sessions, "2")
	if err != nil {
		t.Fatalf("select by index: %v", err)
	}
	if selected.ID != sessions[1].ID {
		t.Fatalf("selected id = %s, want %s", selected.ID, sessions[1].ID)
	}

	selected, err = SelectSession(sessions, "20260430-010203")
	if err != nil {
		t.Fatalf("select by prefix: %v", err)
	}
	if selected.ID != sessions[0].ID {
		t.Fatalf("selected id = %s, want %s", selected.ID, sessions[0].ID)
	}
}

func TestStorePaths(t *testing.T) {
	base := t.TempDir()
	store := newStore(base)

	if got, want := store.MetaPath("abc"), filepath.Join(base, "meta", "abc.json"); got != want {
		t.Fatalf("meta path = %s, want %s", got, want)
	}
	if got, want := store.SocketPath("abc"), filepath.Join(base, "s", "abc.sock"); got != want {
		t.Fatalf("socket path = %s, want %s", got, want)
	}
	if got, want := store.LogPath("abc"), filepath.Join(base, "logs", "abc.log"); got != want {
		t.Fatalf("log path = %s, want %s", got, want)
	}
}

func TestSortMetadataNewestFirst(t *testing.T) {
	oldest := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	newest := oldest.Add(2 * time.Hour)

	sessions := []Metadata{
		{ID: "older-b", CreatedAt: oldest},
		{ID: "newer", CreatedAt: newest},
		{ID: "older-a", CreatedAt: oldest},
	}

	sortMetadata(sessions)

	got := []string{sessions[0].ID, sessions[1].ID, sessions[2].ID}
	want := []string{"newer", "older-b", "older-a"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted ids = %v, want %v", got, want)
		}
	}
}
