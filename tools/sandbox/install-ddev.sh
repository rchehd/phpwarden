#!/bin/sh
set -eu
test "$(uname -s)" = Linux
test "$(uname -m)" = x86_64
destination=${1:?Usage: install-ddev.sh DESTINATION}
mkdir -p "$destination"
archive=$(mktemp)
trap 'rm -f "$archive"' EXIT
curl -qfsSL --proto '=https' --tlsv1.2 --connect-timeout 30 --max-time 180 --retry 2 \
    -o "$archive" https://github.com/ddev/ddev/releases/download/v1.25.4/ddev_linux-amd64.v1.25.4.tar.gz
printf '%s  %s\n' 65fb822f0d2874220c8f9a6b2dfec095d37c0fdc555e34dc5b0e5f4177beeb93 "$archive" | sha256sum -c -
tar -xzf "$archive" -C "$destination" ddev ddev-hostname mkcert
