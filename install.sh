#!/bin/sh

set -eu

repo_url="https://github.com/SarahRoseLives/session"
install_dir="${INSTALL_DIR:-/usr/local/bin}"

require() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "session installer: missing required command: $1" >&2
		exit 1
	fi
}

require curl
require tar
require install

tmpdir="$(mktemp -d)"
cleanup() {
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

os_name="$(uname -s)"
arch_name="$(uname -m)"

case "$os_name" in
Linux) platform="linux" ;;
*)
	echo "session installer: unsupported operating system: $os_name" >&2
	exit 1
	;;
esac

case "$arch_name" in
x86_64|amd64) arch="amd64" ;;
aarch64|arm64) arch="arm64" ;;
*)
	echo "session installer: unsupported architecture: $arch_name" >&2
	exit 1
	;;
esac

asset="session_${platform}_${arch}.tar.gz"
download_url="$repo_url/releases/latest/download/$asset"
archive="$tmpdir/$asset"

curl -fsSL "$download_url" -o "$archive"
tar -xzf "$archive" -C "$tmpdir"

binary="$tmpdir/session"
if [ ! -f "$binary" ]; then
	echo "session installer: downloaded archive did not contain session binary" >&2
	exit 1
fi

if [ -w "$install_dir" ]; then
	install -m 0755 "$binary" "$install_dir/session"
elif command -v sudo >/dev/null 2>&1; then
	sudo install -m 0755 "$binary" "$install_dir/session"
else
	echo "session installer: cannot write to $install_dir and sudo is not available" >&2
	exit 1
fi

echo "installed session to $install_dir/session"
