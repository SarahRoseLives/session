package main

import "testing"

func TestParseConfigAcceptsSessionName(t *testing.T) {
	cfg, err := parseConfig([]string{"--name", "workbench"})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	if cfg.sessionName != "workbench" {
		t.Fatalf("session name = %q, want %q", cfg.sessionName, "workbench")
	}
	if cfg.resume || cfg.list || cfg.daemon {
		t.Fatalf("unexpected config flags set: %+v", cfg)
	}
}

func TestParseConfigRejectsSessionWithResume(t *testing.T) {
	if _, err := parseConfig([]string{"--resume", "--name", "workbench"}); err == nil {
		t.Fatal("expected parseConfig to reject --name with --resume")
	}
}
