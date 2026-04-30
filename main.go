package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"session/internal/session"
)

type config struct {
	resume      bool
	list        bool
	daemon      bool
	id          string
	shell       string
	sessionName string
}

type daemonServer struct {
	store      *session.Store
	meta       session.Metadata
	ptmx       *os.File
	transcript *os.File

	mu           sync.Mutex
	client       net.Conn
	getPTYSize   func() (rows, cols int, err error)
	setPTYSize   func(rows, cols int) error
	windowChange func() error
	getPGRP      func() (int, error)
	signalPGRP   func(int) error
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "session:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		return err
	}

	store, err := session.NewStore()
	if err != nil {
		return err
	}
	if err := store.Ensure(); err != nil {
		return err
	}

	switch {
	case cfg.daemon:
		return runDaemon(store, cfg)
	case cfg.list:
		return printSessions(store, os.Stdout)
	case cfg.resume:
		return runResume(store)
	default:
		return runNewSession(store, cfg)
	}
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("session", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	cfg := config{}
	fs.BoolVar(&cfg.resume, "resume", false, "List saved sessions and reconnect")
	fs.BoolVar(&cfg.list, "list", false, "List saved sessions")
	fs.StringVar(&cfg.sessionName, "name", "", "Name a new session")
	fs.BoolVar(&cfg.daemon, "daemon", false, "")
	fs.StringVar(&cfg.id, "id", "", "")
	fs.StringVar(&cfg.shell, "shell", "", "")

	if err := fs.Parse(args); err != nil {
		return cfg, usageError(err)
	}
	if fs.NArg() != 0 {
		return cfg, usageError(fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " ")))
	}
	if cfg.resume && cfg.list {
		return cfg, usageError(errors.New("use either --resume or --list"))
	}
	cfg.sessionName = strings.TrimSpace(cfg.sessionName)
	if cfg.sessionName == "" && hasNameFlag(args) {
		return cfg, usageError(errors.New("--name requires a non-empty value"))
	}
	if cfg.sessionName != "" && (cfg.resume || cfg.list) {
		return cfg, usageError(errors.New("--name can only be used when creating a new session"))
	}
	if cfg.daemon && cfg.id == "" {
		return cfg, errors.New("missing session id for daemon mode")
	}

	return cfg, nil
}

func usageError(err error) error {
	return fmt.Errorf("%w\n\nusage:\n  session                 start a new session\n  session --name NAME     start a named session\n  session --resume        list sessions and reconnect\n  session --list          list saved sessions", err)
}

func hasNameFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--name" || strings.HasPrefix(arg, "--name=") {
			return true
		}
	}
	return false
}

func runNewSession(store *session.Store, cfg config) error {
	id := session.NewID()
	shell := defaultShell()

	if err := startDaemon(id, shell, cfg.sessionName); err != nil {
		return err
	}

	socketPath := store.SocketPath(id)
	if err := waitForSocket(socketPath, 4*time.Second); err != nil {
		return err
	}

	if cfg.sessionName != "" {
		fmt.Fprintf(os.Stderr, "session: %s (%s)\n", cfg.sessionName, id)
	} else {
		fmt.Fprintf(os.Stderr, "session: %s\n", id)
	}
	return runClient(socketPath, store.LogPath(id))
}

func startDaemon(id, shell, sessionName string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()

	args := []string{"--daemon", "--id", id, "--shell", shell}
	if sessionName != "" {
		args = append(args, "--name", sessionName)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}

func runResume(store *session.Store) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("--resume requires a terminal")
	}

	sessions, err := store.List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return errors.New("no resumable sessions found")
	}

	selected, err := runResumeSelector(sessions)
	if err != nil {
		if errors.Is(err, errResumeCancelled) {
			return nil
		}
		return err
	}

	return runClient(selected.SocketPath, selected.LogPath)
}

func printSessions(store *session.Store, w io.Writer) error {
	sessions, err := store.List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		_, err = fmt.Fprintln(w, "No saved sessions.")
		return err
	}

	return printSessionList(sessions, w)
}

func printSessionList(sessions []session.Metadata, w io.Writer) error {
	for i, saved := range sessions {
		if _, err := fmt.Fprintf(
			w,
			"%d. %s  status=%s  app=%s  cwd=%s  shell=%s  started=%s  %s\n",
			i+1,
			sessionLabel(saved),
			sessionStatus(saved),
			sessionRunningSummary(saved),
			sessionCWD(saved),
			filepath.Base(saved.Shell),
			humanizeAge(time.Since(saved.CreatedAt)),
			lastConnectedSummary(saved),
		); err != nil {
			return err
		}
	}

	return nil
}

func runClient(socketPath, logPath string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("session requires a terminal for interactive sessions")
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	if err := replayTranscript(os.Stdout, logPath); err != nil {
		return err
	}

	var writeMu sync.Mutex
	send := func(kind byte, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return session.WriteFrame(conn, kind, payload)
	}

	if err := sendCurrentSize(send); err != nil {
		return err
	}

	stopResize := make(chan struct{})
	defer close(stopResize)
	if err := forwardResizes(send, stopResize); err != nil {
		return err
	}

	inputErr := make(chan error, 1)
	go func() {
		inputErr <- forwardInput(conn, send)
	}()

	_, copyErr := io.Copy(os.Stdout, conn)
	_ = conn.Close()

	if inErr := <-inputErr; inErr != nil && !errors.Is(inErr, net.ErrClosed) {
		return inErr
	}
	if copyErr != nil && !errors.Is(copyErr, net.ErrClosed) && !errors.Is(copyErr, io.EOF) {
		return copyErr
	}

	return nil
}

func replayTranscript(w io.Writer, logPath string) error {
	if strings.TrimSpace(logPath) == "" {
		return nil
	}

	logFile, err := os.Open(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer logFile.Close()

	if _, err := io.WriteString(w, "\x1b[H\x1b[2J"); err != nil {
		return err
	}

	_, err = io.Copy(w, logFile)
	return err
}

func forwardInput(conn net.Conn, send func(byte, []byte) error) error {
	buffer := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			if writeErr := send(session.FrameInput, chunk); writeErr != nil {
				return writeErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if unixConn, ok := conn.(*net.UnixConn); ok {
				return unixConn.CloseWrite()
			}
			return nil
		}
		return err
	}
}

func forwardResizes(send func(byte, []byte) error, stop <-chan struct{}) error {
	resizeSignals := make(chan os.Signal, 1)
	signal.Notify(resizeSignals, syscall.SIGWINCH)

	go func() {
		defer signal.Stop(resizeSignals)
		for {
			select {
			case <-stop:
				return
			case <-resizeSignals:
				_ = sendCurrentSize(send)
			}
		}
	}()

	return nil
}

func sendCurrentSize(send func(byte, []byte) error) error {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return err
	}

	return send(session.FrameResize, session.EncodeResize(rows, cols))
}

func waitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := os.Stat(socketPath)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(40 * time.Millisecond)
	}

	return fmt.Errorf("session did not become ready: %s", socketPath)
}

func runDaemon(store *session.Store, cfg config) error {
	socketPath := store.SocketPath(cfg.id)
	logPath := store.LogPath(cfg.id)
	metaPath := store.MetaPath(cfg.id)

	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return err
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	shell := cfg.shell
	if shell == "" {
		shell = defaultShell()
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "SESSION_ID="+cfg.id)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	meta := session.Metadata{
		ID:         cfg.id,
		Name:       cfg.sessionName,
		Shell:      shell,
		CreatedAt:  time.Now().UTC(),
		DaemonPID:  os.Getpid(),
		ShellPID:   cmd.Process.Pid,
		SocketPath: socketPath,
		LogPath:    logPath,
		MetaPath:   metaPath,
	}
	if err := store.Save(meta); err != nil {
		return err
	}
	defer store.Remove(cfg.id)

	server := &daemonServer{
		store:      store,
		meta:       meta,
		ptmx:       ptmx,
		transcript: logFile,
	}

	copyDone := make(chan error, 1)
	go func() {
		copyDone <- server.pumpOutput()
	}()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case waitErr := <-waitDone:
				<-copyDone
				if exitErr, ok := waitErr.(*exec.ExitError); ok {
					return exitErr
				}
				return nil
			default:
			}

			if errors.Is(err, net.ErrClosed) {
				break
			}
			return err
		}

		go func(c net.Conn) {
			_ = server.handleClient(c)
		}(conn)
	}

	if err := <-copyDone; err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	if waitErr := <-waitDone; waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			return exitErr
		}
		return waitErr
	}

	return nil
}

func (d *daemonServer) pumpOutput() error {
	buffer := make([]byte, 4096)
	for {
		n, err := d.ptmx.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			if _, writeErr := d.transcript.Write(chunk); writeErr != nil {
				return writeErr
			}

			d.mu.Lock()
			client := d.client
			d.mu.Unlock()

			if client != nil {
				if _, writeErr := client.Write(chunk); writeErr != nil {
					_ = client.Close()
					if releaseErr := d.releaseClient(client); releaseErr != nil {
						fmt.Fprintln(os.Stderr, "session:", releaseErr)
					}
				}
			}
		}

		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
			return nil
		}
		return err
	}
}

func (d *daemonServer) handleClient(conn net.Conn) (err error) {
	if err := d.claimClient(conn); err != nil {
		_, _ = io.WriteString(conn, "session is already connected elsewhere\n")
		_ = conn.Close()
		return err
	}
	defer func() {
		if releaseErr := d.releaseClient(conn); err == nil && releaseErr != nil {
			err = releaseErr
		}
		_ = conn.Close()
	}()

	firstResize := true
	for {
		frameType, payload, err := session.ReadFrame(conn)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		switch frameType {
		case session.FrameInput:
			if _, err := d.ptmx.Write(payload); err != nil {
				return err
			}
		case session.FrameResize:
			rows, cols, err := session.DecodeResize(payload)
			if err != nil {
				return err
			}

			if firstResize {
				firstResize = false
				if err := d.applyInitialResize(rows, cols); err != nil {
					return err
				}
				continue
			}

			if err := d.applyResize(rows, cols); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown frame type: %d", frameType)
		}
	}
}

func (d *daemonServer) claimClient(conn net.Conn) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client != nil {
		return errors.New("session already has an active client")
	}

	now := time.Now().UTC()
	d.meta.LastConnectedAt = &now
	d.meta.ActiveClient = true
	if err := d.store.Save(d.meta); err != nil {
		return err
	}

	d.client = conn
	return nil
}

func (d *daemonServer) applyInitialResize(rows, cols int) error {
	currentRows, currentCols, err := d.currentPTYSize()
	if err != nil {
		return d.applyResize(rows, cols)
	}

	if currentRows != rows || currentCols != cols {
		return d.applyResize(rows, cols)
	}

	alternateRows, alternateCols := rows, cols
	if cols < 0xffff {
		alternateCols++
	} else if cols > 1 {
		alternateCols--
	} else if rows < 0xffff {
		alternateRows++
	} else if rows > 1 {
		alternateRows--
	}

	if alternateRows != rows || alternateCols != cols {
		if err := d.resizePTY(alternateRows, alternateCols); err != nil {
			return err
		}
	}

	return d.applyResize(rows, cols)
}

func (d *daemonServer) applyResize(rows, cols int) error {
	if err := d.resizePTY(rows, cols); err != nil {
		return err
	}
	return d.notifyWindowChange()
}

func (d *daemonServer) currentPTYSize() (rows, cols int, err error) {
	if d.getPTYSize != nil {
		return d.getPTYSize()
	}
	return pty.Getsize(d.ptmx)
}

func (d *daemonServer) resizePTY(rows, cols int) error {
	if d.setPTYSize != nil {
		return d.setPTYSize(rows, cols)
	}

	return pty.Setsize(d.ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func (d *daemonServer) notifyWindowChange() error {
	if d.windowChange != nil {
		return d.windowChange()
	}

	processGroupID, err := d.foregroundProcessGroupID()
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}

	if err := d.sendSIGWINCH(processGroupID); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}

	return nil
}

func (d *daemonServer) foregroundProcessGroupID() (int, error) {
	if d.getPGRP != nil {
		return d.getPGRP()
	}

	if d.ptmx != nil {
		processGroupID, err := unix.IoctlGetInt(int(d.ptmx.Fd()), unix.TIOCGPGRP)
		if err == nil && processGroupID > 0 {
			return processGroupID, nil
		}
		if err != nil && !errors.Is(err, syscall.ENOTTY) {
			return 0, err
		}
	}

	return syscall.Getpgid(d.meta.ShellPID)
}

func (d *daemonServer) sendSIGWINCH(processGroupID int) error {
	if d.signalPGRP != nil {
		return d.signalPGRP(processGroupID)
	}
	return syscall.Kill(-processGroupID, syscall.SIGWINCH)
}

func (d *daemonServer) releaseClient(conn net.Conn) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.client == conn {
		d.client = nil
		d.meta.ActiveClient = false
		return d.store.Save(d.meta)
	}

	return nil
}

func defaultShell() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}

func humanizeAge(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
	}
}

func lastConnectedSummary(meta session.Metadata) string {
	if meta.LastConnectedAt == nil {
		return "never connected"
	}
	return "last connected " + humanizeAge(time.Since(*meta.LastConnectedAt))
}

func sessionStatus(meta session.Metadata) string {
	if meta.ActiveClient {
		return "attached"
	}
	return "ready"
}

func sessionLabel(meta session.Metadata) string {
	if meta.Name == "" {
		return meta.ID
	}
	return fmt.Sprintf("%s (%s)", meta.Name, meta.ID)
}

func sessionRunningSummary(meta session.Metadata) string {
	if meta.RunningCommand == "" {
		return "shell idle"
	}
	return meta.RunningCommand
}

func sessionCWD(meta session.Metadata) string {
	if meta.CurrentDir == "" {
		return "unknown"
	}
	return meta.CurrentDir
}
