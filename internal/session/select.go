package session

import (
	"fmt"
	"strconv"
	"strings"
)

func SelectSession(sessions []Metadata, input string) (Metadata, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Metadata{}, fmt.Errorf("select a session by number or id")
	}

	if index, err := strconv.Atoi(trimmed); err == nil {
		if index < 1 || index > len(sessions) {
			return Metadata{}, fmt.Errorf("session %d is out of range", index)
		}
		return sessions[index-1], nil
	}

	var matches []Metadata
	for _, candidate := range sessions {
		if candidate.ID == trimmed || strings.HasPrefix(candidate.ID, trimmed) {
			matches = append(matches, candidate)
		}
	}

	switch len(matches) {
	case 0:
		return Metadata{}, fmt.Errorf("unknown session %q", trimmed)
	case 1:
		return matches[0], nil
	default:
		return Metadata{}, fmt.Errorf("session %q matches more than one saved session", trimmed)
	}
}
