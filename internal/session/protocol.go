package session

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	FrameInput byte = 1 + iota
	FrameResize
)

func WriteFrame(w io.Writer, frameType byte, payload []byte) error {
	header := make([]byte, 5)
	header[0] = frameType
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))

	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}

	_, err := w.Write(payload)
	return err
}

func ReadFrame(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	size := binary.BigEndian.Uint32(header[1:])
	payload := make([]byte, size)
	if size > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	return header[0], payload, nil
}

func EncodeResize(rows, cols int) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], uint16(rows))
	binary.BigEndian.PutUint16(payload[2:4], uint16(cols))
	return payload
}

func DecodeResize(payload []byte) (int, int, error) {
	if len(payload) != 4 {
		return 0, 0, fmt.Errorf("resize payload must be 4 bytes, got %d", len(payload))
	}

	rows := int(binary.BigEndian.Uint16(payload[0:2]))
	cols := int(binary.BigEndian.Uint16(payload[2:4]))
	if rows <= 0 || cols <= 0 {
		return 0, 0, fmt.Errorf("resize payload must contain positive dimensions")
	}

	return rows, cols, nil
}
