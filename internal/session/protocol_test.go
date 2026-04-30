package session

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buffer bytes.Buffer

	if err := WriteFrame(&buffer, FrameInput, []byte("hello")); err != nil {
		t.Fatalf("write frame: %v", err)
	}

	frameType, payload, err := ReadFrame(&buffer)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}

	if frameType != FrameInput {
		t.Fatalf("frame type = %d, want %d", frameType, FrameInput)
	}
	if string(payload) != "hello" {
		t.Fatalf("payload = %q, want %q", payload, "hello")
	}
}

func TestResizeRoundTrip(t *testing.T) {
	payload := EncodeResize(42, 120)

	rows, cols, err := DecodeResize(payload)
	if err != nil {
		t.Fatalf("decode resize: %v", err)
	}

	if rows != 42 || cols != 120 {
		t.Fatalf("resize = (%d, %d), want (42, 120)", rows, cols)
	}
}
