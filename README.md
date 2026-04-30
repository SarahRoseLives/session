# session

`session` is a lightweight terminal session manager for persistent shells.

It is designed for the simple workflow:
- start a session
- disconnect from SSH or close your terminal
- come back later
- run `session --resume` and jump back in

Unlike `screen`, this project stays focused on **session persistence and quick re-entry**, not full terminal multiplexing.

## Features

- **Persistent shell sessions** backed by a PTY
- **Resume picker** built with Bubble Tea
- **Newest-first session list**
- **Named sessions** with `--name`
- **Live session details** in the list and resume UI:
  - session name
  - current working directory
  - detected foreground application
  - shell and daemon PIDs
- **Single-binary workflow**

## Status

This is an intentionally small first version.

Current behavior:
- one client attached to a session at a time
- one shell per session
- sessions are stored under the user state directory:
  - `$XDG_STATE_HOME/session`
  - or `~/.local/state/session`

## Install

One-line install to `/usr/local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/SarahRoseLives/session/main/install.sh | sh
```

The installer downloads the latest GitHub release binary and places it at `/usr/local/bin/session`. It currently supports **Linux amd64** and **Linux arm64**. If `/usr/local/bin` is not writable, it will use `sudo`.

Build it locally:

```bash
go build -o session .
```

Or install it to your Go bin directory:

```bash
go install .
```

## Usage

Start a new session:

```bash
session
```

Start a named session:

```bash
session --name workbench
```

List saved sessions:

```bash
session --list
```

Open the interactive resume picker:

```bash
session --resume
```

## Resume UI

The `--resume` screen supports:

- **arrow keys** or `j` / `k` to move
- **enter** to reconnect
- **q** or **esc** to cancel

It adapts to smaller terminals by switching to a compact layout and trimming long values.

## How it works

When you start a session, `session` launches a detached background daemon that:

1. starts your shell inside a PTY
2. stores session metadata on disk
3. exposes a Unix socket for reconnecting
4. forwards terminal input, output, and resize events

The resume view reads saved sessions, checks which ones are still alive, enriches them with runtime details from `/proc`, and presents them newest first.

## Compared to screen

`screen` is a powerful multiplexer.

`session` is narrower by design:

- **better for:** “keep this shell alive and let me get back to it later”
- **not trying to be:** multiwindow management, copy mode, splits, or a full multiplexer replacement

## Development

Run tests:

```bash
go test ./...
```
