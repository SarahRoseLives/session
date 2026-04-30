package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"session/internal/session"
)

func TestResumeSelectorFitsNarrowScreen(t *testing.T) {
	now := time.Date(2026, time.April, 30, 3, 0, 0, 0, time.UTC)
	model := resumeSelectorModel{
		sessions: []session.Metadata{
			{
				ID:         "20260430-030000-abcdef",
				Shell:      "/bin/zsh",
				CreatedAt:  now,
				DaemonPID:  12345,
				ShellPID:   12346,
				SocketPath: "/tmp/session/very/long/path/to/socket/that/should/not/overflow.sock",
				LogPath:    "/tmp/session/very/long/path/to/log/that/should/not/overflow.log",
			},
		},
		width:  48,
		height: 20,
	}

	assertFitsViewport(t, model)
}

func TestResumeSelectorFitsShortScreen(t *testing.T) {
	now := time.Date(2026, time.April, 30, 3, 0, 0, 0, time.UTC)
	model := resumeSelectorModel{
		sessions: []session.Metadata{
			{
				ID:         "20260430-030000-abcdef",
				Shell:      "/bin/zsh",
				CreatedAt:  now,
				DaemonPID:  12345,
				ShellPID:   12346,
				SocketPath: "/tmp/session/socket.sock",
				LogPath:    "/tmp/session/log.log",
			},
			{
				ID:         "20260430-025900-fedcba",
				Shell:      "/bin/bash",
				CreatedAt:  now.Add(-time.Hour),
				DaemonPID:  999,
				ShellPID:   1000,
				SocketPath: "/tmp/session/socket-2.sock",
				LogPath:    "/tmp/session/log-2.log",
			},
		},
		cursor: 1,
		width:  42,
		height: 10,
	}

	assertFitsViewport(t, model)
}

func assertFitsViewport(t *testing.T, model resumeSelectorModel) {
	t.Helper()

	view := model.View()
	lines := strings.Split(view, "\n")
	if len(lines) > model.height {
		t.Fatalf("line count = %d, want <= %d", len(lines), model.height)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > model.width {
			t.Fatalf("line width = %d, want <= %d: %q", lipgloss.Width(line), model.width, line)
		}
	}
}
