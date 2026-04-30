#!/bin/sh

set -eu

repo_archive_url="https://codeload.github.com/SarahRoseLives/session/tar.gz/refs/heads/main"
install_dir="${INSTALL_DIR:-/usr/local/bin}"

require() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "session installer: missing required command: $1" >&2
		exit 1
	fi
}

require curl
require tar
require go
require install

tmpdir="$(mktemp -d)"
cleanup() {
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

archive="$tmpdir/session.tar.gz"
srcdir="$tmpdir/session-main"

curl -fsSL "$repo_archive_url" -o "$archive"
tar -xzf "$archive" -C "$tmpdir"

cd "$srcdir"
go build -o session .

if [ -w "$install_dir" ]; then
	install -m 0755 session "$install_dir/session"
elif command -v sudo >/dev/null 2>&1; then
	sudo install -m 0755 session "$install_dir/session"
else
	echo "session installer: cannot write to $install_dir and sudo is not available" >&2
	exit 1
fi

echo "installed session to $install_dir/session"
