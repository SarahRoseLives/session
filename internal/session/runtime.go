package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type runtimeSnapshot struct {
	currentDir     string
	runningCommand string
}

type processStat struct {
	pid   int
	pgrp  int
	tpgid int
	comm  string
}

func loadRuntimeSnapshot(shellPID int) (runtimeSnapshot, error) {
	return loadRuntimeSnapshotFromProc("/proc", shellPID)
}

func loadRuntimeSnapshotFromProc(procRoot string, shellPID int) (runtimeSnapshot, error) {
	shellStat, err := readProcessStat(procRoot, shellPID)
	if err != nil {
		return runtimeSnapshot{}, err
	}

	shellDir, _ := readProcessCWD(procRoot, shellPID)
	if shellStat.tpgid <= 0 || shellStat.tpgid == shellStat.pgrp {
		return runtimeSnapshot{currentDir: shellDir}, nil
	}

	foregroundPID, foregroundName, err := selectForegroundProcess(procRoot, shellStat.tpgid, shellPID)
	if err != nil {
		return runtimeSnapshot{currentDir: shellDir}, nil
	}

	currentDir, err := readProcessCWD(procRoot, foregroundPID)
	if err != nil {
		currentDir = shellDir
	}

	return runtimeSnapshot{
		currentDir:     currentDir,
		runningCommand: foregroundName,
	}, nil
}

func selectForegroundProcess(procRoot string, pgrp, shellPID int) (int, string, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, "", err
	}

	type candidate struct {
		pid  int
		name string
	}

	var matches []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		stat, err := readProcessStat(procRoot, pid)
		if err != nil || stat.pgrp != pgrp {
			continue
		}

		name, err := readProcessName(procRoot, pid)
		if err != nil {
			name = stat.comm
		}

		matches = append(matches, candidate{pid: pid, name: name})
	}

	if len(matches) == 0 {
		return 0, "", fmt.Errorf("no processes found for pgrp %d", pgrp)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].pid == pgrp {
			return true
		}
		if matches[j].pid == pgrp {
			return false
		}
		if matches[i].pid == shellPID {
			return false
		}
		if matches[j].pid == shellPID {
			return true
		}
		return matches[i].pid < matches[j].pid
	})

	chosen := matches[0]
	if chosen.pid == shellPID && len(matches) > 1 {
		chosen = matches[1]
	}

	return chosen.pid, chosen.name, nil
}

func readProcessStat(procRoot string, pid int) (processStat, error) {
	payload, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return processStat{}, err
	}

	return parseProcessStat(string(payload))
}

func parseProcessStat(payload string) (processStat, error) {
	end := strings.LastIndex(payload, ")")
	start := strings.Index(payload, "(")
	if start == -1 || end == -1 || end <= start {
		return processStat{}, fmt.Errorf("invalid stat payload")
	}

	pidValue, err := strconv.Atoi(strings.TrimSpace(payload[:start]))
	if err != nil {
		return processStat{}, err
	}

	fields := strings.Fields(strings.TrimSpace(payload[end+1:]))
	if len(fields) < 6 {
		return processStat{}, fmt.Errorf("invalid stat payload field count")
	}

	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return processStat{}, err
	}

	tpgid, err := strconv.Atoi(fields[5])
	if err != nil {
		return processStat{}, err
	}

	return processStat{
		pid:   pidValue,
		pgrp:  pgrp,
		tpgid: tpgid,
		comm:  payload[start+1 : end],
	}, nil
}

func readProcessCWD(procRoot string, pid int) (string, error) {
	return os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "cwd"))
}

func readProcessName(procRoot string, pid int) (string, error) {
	payload, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return "", err
	}

	parts := strings.Split(string(payload), "\x00")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		return filepath.Base(part), nil
	}

	payload, err = os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(payload)), nil
}
