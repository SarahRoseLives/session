package session

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLoadRuntimeSnapshotUsesForegroundProcess(t *testing.T) {
	procRoot := t.TempDir()

	writeProcFixture(t, procRoot, 101, "101 (zsh) S 1 101 101 0 202 0 0 0 0", "/work/shell", "/bin/zsh", "zsh")
	writeProcFixture(t, procRoot, 202, "202 (nvim) S 101 202 101 0 202 0 0 0 0", "/work/app", "/usr/bin/nvim", "nvim")

	snapshot, err := loadRuntimeSnapshotFromProc(procRoot, 101)
	if err != nil {
		t.Fatalf("load runtime snapshot: %v", err)
	}

	if snapshot.currentDir != "/work/app" {
		t.Fatalf("current dir = %q, want %q", snapshot.currentDir, "/work/app")
	}
	if snapshot.runningCommand != "nvim" {
		t.Fatalf("running command = %q, want %q", snapshot.runningCommand, "nvim")
	}
}

func TestLoadRuntimeSnapshotFallsBackToShellWhenIdle(t *testing.T) {
	procRoot := t.TempDir()

	writeProcFixture(t, procRoot, 101, "101 (zsh) S 1 101 101 0 101 0 0 0 0", "/work/shell", "/bin/zsh", "zsh")

	snapshot, err := loadRuntimeSnapshotFromProc(procRoot, 101)
	if err != nil {
		t.Fatalf("load runtime snapshot: %v", err)
	}

	if snapshot.currentDir != "/work/shell" {
		t.Fatalf("current dir = %q, want %q", snapshot.currentDir, "/work/shell")
	}
	if snapshot.runningCommand != "" {
		t.Fatalf("running command = %q, want empty", snapshot.runningCommand)
	}
}

func TestParseProcessStat(t *testing.T) {
	stat, err := parseProcessStat("4321 (bash) S 1 4321 4321 0 9876 0 0 0 0")
	if err != nil {
		t.Fatalf("parse process stat: %v", err)
	}

	if stat.pid != 4321 || stat.pgrp != 4321 || stat.tpgid != 9876 || stat.comm != "bash" {
		t.Fatalf("unexpected stat: %+v", stat)
	}
}

func writeProcFixture(t *testing.T, procRoot string, pid int, stat, cwd, cmdline, comm string) {
	t.Helper()

	processDir := filepath.Join(procRoot, strconv.Itoa(pid))
	if err := os.MkdirAll(processDir, 0o755); err != nil {
		t.Fatalf("mkdir process dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(processDir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(processDir, "cmdline"), []byte(cmdline+"\x00"), 0o644); err != nil {
		t.Fatalf("write cmdline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(processDir, "comm"), []byte(comm+"\n"), 0o644); err != nil {
		t.Fatalf("write comm: %v", err)
	}
	if err := os.Symlink(cwd, filepath.Join(processDir, "cwd")); err != nil {
		t.Fatalf("symlink cwd: %v", err)
	}
}
