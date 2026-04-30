package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Metadata struct {
	ID              string     `json:"id"`
	Name            string     `json:"name,omitempty"`
	Shell           string     `json:"shell"`
	CreatedAt       time.Time  `json:"created_at"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	ActiveClient    bool       `json:"active_client"`
	DaemonPID       int        `json:"daemon_pid"`
	ShellPID        int        `json:"shell_pid"`
	SocketPath      string     `json:"socket_path"`
	LogPath         string     `json:"log_path"`
	MetaPath        string     `json:"meta_path"`
	CurrentDir      string     `json:"-"`
	RunningCommand  string     `json:"-"`
}

type Store struct {
	BaseDir   string
	MetaDir   string
	SocketDir string
	LogDir    string
}

func NewStore() (*Store, error) {
	stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if stateHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		stateHome = filepath.Join(homeDir, ".local", "state")
	}

	return newStore(filepath.Join(stateHome, "session")), nil
}

func newStore(baseDir string) *Store {
	return &Store{
		BaseDir:   baseDir,
		MetaDir:   filepath.Join(baseDir, "meta"),
		SocketDir: filepath.Join(baseDir, "s"),
		LogDir:    filepath.Join(baseDir, "logs"),
	}
}

func (s *Store) Ensure() error {
	for _, dir := range []string{s.BaseDir, s.MetaDir, s.SocketDir, s.LogDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Save(meta Metadata) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	if meta.MetaPath == "" {
		meta.MetaPath = s.MetaPath(meta.ID)
	}
	if meta.SocketPath == "" {
		meta.SocketPath = s.SocketPath(meta.ID)
	}
	if meta.LogPath == "" {
		meta.LogPath = s.LogPath(meta.ID)
	}

	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	tempPath := meta.MetaPath + ".tmp"
	if err := os.WriteFile(tempPath, append(payload, '\n'), 0o600); err != nil {
		return err
	}

	return os.Rename(tempPath, meta.MetaPath)
}

func (s *Store) Load(id string) (Metadata, error) {
	path := s.MetaPath(id)
	payload, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}

	var meta Metadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return Metadata{}, err
	}

	return meta, nil
}

func (s *Store) List() ([]Metadata, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.MetaDir)
	if err != nil {
		return nil, err
	}

	sessions := make([]Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		payload, err := os.ReadFile(filepath.Join(s.MetaDir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var meta Metadata
		if err := json.Unmarshal(payload, &meta); err != nil {
			return nil, err
		}

		if !s.isLive(meta) {
			_ = s.Remove(meta.ID)
			continue
		}

		s.enrichRuntimeDetails(&meta)
		sessions = append(sessions, meta)
	}

	sortMetadata(sessions)

	return sessions, nil
}

func (s *Store) Remove(id string) error {
	var joined error
	for _, path := range []string{s.MetaPath(id), s.SocketPath(id)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (s *Store) MetaPath(id string) string {
	return filepath.Join(s.MetaDir, id+".json")
}

func (s *Store) SocketPath(id string) string {
	return filepath.Join(s.SocketDir, id+".sock")
}

func (s *Store) LogPath(id string) string {
	return filepath.Join(s.LogDir, id+".log")
}

func (s *Store) isLive(meta Metadata) bool {
	if meta.DaemonPID <= 0 {
		return false
	}
	if err := syscall.Kill(meta.DaemonPID, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}

	info, err := os.Stat(meta.SocketPath)
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeSocket != 0
}

func NewID() string {
	random := make([]byte, 3)
	if _, err := rand.Read(random); err != nil {
		panic(fmt.Errorf("generate session id: %w", err))
	}

	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(random))
}

func sortMetadata(sessions []Metadata) {
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			return sessions[i].ID > sessions[j].ID
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
}

func (s *Store) enrichRuntimeDetails(meta *Metadata) {
	snapshot, err := loadRuntimeSnapshot(meta.ShellPID)
	if err != nil {
		return
	}

	meta.CurrentDir = snapshot.currentDir
	meta.RunningCommand = snapshot.runningCommand
}
