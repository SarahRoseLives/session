package main

import (
	"errors"
	"testing"
)

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

func TestApplyResizeSignalsWindowChange(t *testing.T) {
	server := &daemonServer{}

	var gotRows, gotCols int
	var resizeCalls, signalCalls int
	server.setPTYSize = func(rows, cols int) error {
		resizeCalls++
		gotRows, gotCols = rows, cols
		return nil
	}
	server.windowChange = func() error {
		signalCalls++
		return nil
	}

	if err := server.applyResize(40, 120); err != nil {
		t.Fatalf("apply resize: %v", err)
	}
	if resizeCalls != 1 || signalCalls != 1 {
		t.Fatalf("calls = resize %d signal %d, want 1 each", resizeCalls, signalCalls)
	}
	if gotRows != 40 || gotCols != 120 {
		t.Fatalf("resize = (%d, %d), want (40, 120)", gotRows, gotCols)
	}
}

func TestApplyResizeStopsOnResizeError(t *testing.T) {
	server := &daemonServer{}
	server.setPTYSize = func(rows, cols int) error {
		return errors.New("resize failed")
	}
	server.windowChange = func() error {
		t.Fatal("window change should not be called when resize fails")
		return nil
	}

	if err := server.applyResize(40, 120); err == nil {
		t.Fatal("expected applyResize to return an error")
	}
}
