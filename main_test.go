package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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

func TestNotifyWindowChangeUsesForegroundProcessGroup(t *testing.T) {
	server := &daemonServer{}

	var gotPGRP int
	server.getPGRP = func() (int, error) {
		return 4321, nil
	}
	server.signalPGRP = func(processGroupID int) error {
		gotPGRP = processGroupID
		return nil
	}

	if err := server.notifyWindowChange(); err != nil {
		t.Fatalf("notify window change: %v", err)
	}
	if gotPGRP != 4321 {
		t.Fatalf("process group = %d, want %d", gotPGRP, 4321)
	}
}

func TestApplyInitialResizeForcesRedrawWhenSizeMatches(t *testing.T) {
	server := &daemonServer{}

	server.getPTYSize = func() (int, int, error) {
		return 40, 120, nil
	}

	var sizes [][2]int
	server.setPTYSize = func(rows, cols int) error {
		sizes = append(sizes, [2]int{rows, cols})
		return nil
	}

	var signalCalls int
	server.windowChange = func() error {
		signalCalls++
		return nil
	}

	if err := server.applyInitialResize(40, 120); err != nil {
		t.Fatalf("apply initial resize: %v", err)
	}

	if len(sizes) != 2 {
		t.Fatalf("resize calls = %d, want 2", len(sizes))
	}
	if sizes[0] == sizes[1] {
		t.Fatalf("forced resize did not change size: %v", sizes)
	}
	if sizes[1] != [2]int{40, 120} {
		t.Fatalf("final resize = %v, want %v", sizes[1], [2]int{40, 120})
	}
	if signalCalls != 1 {
		t.Fatalf("signal calls = %d, want 1", signalCalls)
	}
}

func TestApplyInitialResizeSkipsForcedCycleWhenSizeDiffers(t *testing.T) {
	server := &daemonServer{}

	server.getPTYSize = func() (int, int, error) {
		return 24, 80, nil
	}

	var sizes [][2]int
	server.setPTYSize = func(rows, cols int) error {
		sizes = append(sizes, [2]int{rows, cols})
		return nil
	}
	server.windowChange = func() error { return nil }

	if err := server.applyInitialResize(40, 120); err != nil {
		t.Fatalf("apply initial resize: %v", err)
	}

	if len(sizes) != 1 || sizes[0] != [2]int{40, 120} {
		t.Fatalf("resize calls = %v, want only final target", sizes)
	}
}

func TestReplayTranscriptWritesClearAndLogContents(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "session.log")
	if err := os.WriteFile(logPath, []byte("hello\nworld\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	var output bytes.Buffer
	if err := replayTranscript(&output, logPath); err != nil {
		t.Fatalf("replay transcript: %v", err)
	}

	if got, want := output.String(), "\x1b[2;1H\x1b[Jhello\nworld\n"; got != want {
		t.Fatalf("replay output = %q, want %q", got, want)
	}
}

func TestDetachParserRecognizesCtrlBThenD(t *testing.T) {
	parser := detachParser{}

	output, detach := parser.parse([]byte{detachPrefix, 'd'})
	if detach != true {
		t.Fatal("expected detach to be true")
	}
	if len(output) != 0 {
		t.Fatalf("output = %v, want empty", output)
	}
}

func TestDetachParserForwardsOtherCtrlBSequences(t *testing.T) {
	parser := detachParser{}

	output, detach := parser.parse([]byte{detachPrefix, 'x'})
	if detach {
		t.Fatal("did not expect detach")
	}
	if got, want := string(output), string([]byte{detachPrefix, 'x'}); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderSessionHeaderIncludesTitleAndHint(t *testing.T) {
	got := renderSessionHeader("workbench", 40)
	if !bytes.Contains([]byte(got), []byte("Session - workbench")) {
		t.Fatalf("header = %q, missing title", got)
	}
	if !bytes.Contains([]byte(got), []byte("Ctrl-B d detach")) {
		t.Fatalf("header = %q, missing detach hint", got)
	}
}

func TestContentRowsReservesHeaderLine(t *testing.T) {
	if got := contentRows(24); got != 23 {
		t.Fatalf("content rows = %d, want %d", got, 23)
	}
	if got := contentRows(1); got != 1 {
		t.Fatalf("content rows = %d, want %d", got, 1)
	}
}

func TestSessionChromeWriterDetectsClearSequenceAcrossWrites(t *testing.T) {
	writer := newSessionChromeWriter(&bytes.Buffer{}, "workbench")

	if writer.needsRedraw([]byte("hello")) {
		t.Fatal("did not expect redraw for plain text")
	}
	writer.rememberTail([]byte("\x1b[H"))
	if !writer.needsRedraw([]byte("\x1b[2J")) {
		t.Fatal("expected redraw when clear sequence completes across writes")
	}
}
