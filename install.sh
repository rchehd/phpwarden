#!/bin/sh
set -eu
umask 077
export LC_ALL=C

usage() {
    cat <<'EOF'
Usage: sh install.sh [--version VERSION] [--dir DIR] [--force]
       sh install.sh --archive FILE [--checksums FILE] [--version VERSION] [--dir DIR] [--force]

Install PHPWarden for Linux amd64 without sudo, PHP, Go or Docker.
Default: latest stable GitHub release into $HOME/.local/bin.
Local archives use the adjacent SHA256SUMS unless --checksums is given.
--force atomically replaces an existing regular executable; symlinks and package directories are refused.
--commit SHA requires the exact executable source commit; --require-published refuses source/dev archives.
EOF
}

fail() { printf 'PHPWarden installer: %s\n' "$*" >&2; exit 3; }
invalid() { printf 'PHPWarden installer: %s\n' "$*" >&2; exit 2; }

version=
archive=
checksums=
install_dir=
force=false
expected_commit=
require_published=false
work=
staged=
seen=' '
while [ "$#" -gt 0 ]; do
    option=$1
    shift
    case "$option" in
        --help|-h) usage; exit 0 ;;
        --force) value=true ;;
        --require-published) value=true ;;
        --version|--archive|--checksums|--dir|--commit)
            [ "$#" -gt 0 ] && [ -n "$1" ] || invalid "$option requires a value"
            case "$1" in --*) invalid "$option requires a value" ;; esac
            value=$1
            shift
            ;;
        *) invalid "unknown option: $option; use --help" ;;
    esac
    case "$seen" in *" $option "*) invalid "duplicate option: $option" ;; esac
    seen="$seen$option "
    case "$option" in
        --version) version="v${value#v}" ;;
        --archive) archive=$value ;;
        --checksums) checksums=$value ;;
        --dir) install_dir=$value ;;
        --force) force=true ;;
        --commit) expected_commit=$value ;;
        --require-published) require_published=true ;;
    esac
done

command -v uname >/dev/null 2>&1 || fail "required command is missing: uname"
[ "$(uname -s)" = Linux ] || fail "only Linux amd64 is supported"
case "$(uname -m)" in x86_64|amd64) ;; *) fail "only Linux amd64 is supported" ;; esac
for tool in awk cat chmod cmp cp dirname flock grep gzip head mkdir mktemp mv pwd rm sed sha256sum tar timeout uname wc; do
    command -v "$tool" >/dev/null 2>&1 || fail "required command is missing: $tool"
done

valid_version() {
    printf '%s\n' "$1" | awk '
        NR != 1 {exit 1}
        length($0) > 128 || $0 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$/ {exit 1}
        { sub(/^v[0-9]+\.[0-9]+\.[0-9]+/, ""); sub(/^-/, ""); n=split($0, parts, ".") }
        { for (i=1; i<=n; i++) if (parts[i] ~ /^0[0-9]+$/) exit 1 }
    '
}

if [ -n "$version" ]; then
    valid_version "$version" || invalid "version must be MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-prerelease without leading zeroes"
fi
if [ -n "$expected_commit" ]; then
    printf '%s\n' "$expected_commit" | awk 'NR != 1 || length($0) != 40 || /[^0-9a-f]/ {bad=1} END {exit bad}' || invalid "commit must be an exact lowercase Git SHA"
fi
[ -z "$checksums" ] || [ -n "$archive" ] || invalid "--checksums requires --archive"
if [ -z "$install_dir" ]; then
    [ -n "${HOME:-}" ] || invalid "set HOME or pass --dir"
    install_dir=$HOME/.local/bin
fi

cleanup() {
    if [ -n "$staged" ]; then rm -f -- "$staged"; fi
    if [ -n "$work" ]; then rm -rf -- "$work"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
work=$(mktemp -d "${TMPDIR:-/tmp}/phpwarden-install.XXXXXX") || fail "cannot create a temporary directory"
archive_name=phpwarden-linux-amd64.tar.gz
local_archive=$work/$archive_name

download() {
    curl -q --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --connect-timeout 15 --max-time 180 --retry 2 --max-filesize "$3" --output "$2" "$1" ||
        fail "download failed; check the release version and network, or use --archive with a CI artifact"
}

if [ -n "$archive" ]; then
    [ -f "$archive" ] || fail "archive does not exist: $archive"
    [ "$(wc -c < "$archive")" -le 134217728 ] || fail "archive exceeds 128 MiB"
    if [ -z "$checksums" ]; then checksums=$(dirname -- "$archive")/SHA256SUMS; fi
    [ -f "$checksums" ] || fail "checksums file does not exist: $checksums"
    [ "$(wc -c < "$checksums")" -le 65536 ] || fail "checksums file exceeds 64 KiB"
    cp -- "$archive" "$local_archive" || fail "cannot copy the archive"
    cp -- "$checksums" "$work/SHA256SUMS" || fail "cannot copy checksums"
else
    command -v curl >/dev/null 2>&1 || fail "curl is required to download a release; use --archive for offline installation"
    base=https://github.com/rchehd/phpwarden/releases/latest/download
    if [ -n "$version" ]; then base=https://github.com/rchehd/phpwarden/releases/download/$version; fi
    printf 'Downloading PHPWarden %s…\n' "${version:-latest stable}" >&2
    download "$base/SHA256SUMS" "$work/SHA256SUMS" 65536
    download "$base/$archive_name" "$local_archive" 134217728
fi

expected=$(awk -v name="$archive_name" '
    NF == 0 {next}
    NF != 2 || length($1) != 64 || $1 ~ /[^0-9a-fA-F]/ {bad=1; exit}
    $2 == name || $2 == "*" name {matches++; hash=tolower($1)}
    END {if (bad || matches != 1) exit 1; print hash}
' "$work/SHA256SUMS") || fail "SHA256SUMS must contain exactly one valid checksum for $archive_name"
printf '%s  %s\n' "$expected" "$local_archive" | sha256sum --status -c - || fail "archive checksum mismatch; nothing was installed"
timeout 30 gzip -t -- "$local_archive" || fail "invalid gzip archive"
timeout 30 tar -tzf "$local_archive" > "$work/entries" || fail "invalid tar archive"
awk '
    $0 != "phpwarden" && $0 != "README.md" && $0 != "COMMIT" && $0 != "release.json" && $0 != "compatibility.json" && $0 != "THIRD_PARTY_NOTICES.txt" && $0 != "dependencies.json" {bad=1}
    {if (seen[$0]++) bad=1}
    END {if (bad || !seen["phpwarden"] || !seen["release.json"] || !seen["COMMIT"] || !seen["README.md"]) exit 1}
' "$work/entries" || fail "archive contains unexpected, duplicate or missing entries"

extract() {
    listing=$(timeout 30 tar -tvzf "$local_archive" -- "$1") || fail "cannot inspect archive entry: $1"
    case "$listing" in -*) ;; *) fail "archive entries must be regular files: $1" ;; esac
    timeout 30 tar -xOzf "$local_archive" -- "$1" | head -c "$(( $2 + 1 ))" > "$work/$1" || fail "cannot extract $1"
    size=$(wc -c < "$work/$1")
    [ "$size" -gt 0 ] && [ "$size" -le "$2" ] || fail "archive entry is empty or exceeds its size limit: $1"
}
extract phpwarden 134217728
extract release.json 1048576
extract COMMIT 1024
for notice in THIRD_PARTY_NOTICES.txt dependencies.json; do
    if grep -qx "$notice" "$work/entries"; then extract "$notice" 1048576; fi
done
chmod 755 "$work/phpwarden" || fail "cannot prepare executable"
timeout 15 "$work/phpwarden" --build-info > "$work/build-info.json" || fail "downloaded CLI cannot run on this platform"
cmp -s "$work/build-info.json" "$work/release.json" || fail "CLI identity differs from release.json"

field() { sed -n "s/^[[:space:]]*\"$1\":[[:space:]]*\"\([^\"]*\)\"[,]*[[:space:]]*$/\1/p" "$work/build-info.json"; }
actual_version=$(field version)
distribution=$(field distribution)
commit=$(field commit)
[ "$(field platform)" = linux/amd64 ] || fail "CLI platform does not match Linux amd64"
valid_version "$actual_version" || fail "CLI reports an invalid version"
printf '%s\n' "$commit" | grep -Eq '^[0-9a-f]{40}$' || fail "CLI reports an invalid source commit"
[ "$(cat "$work/COMMIT")" = "$commit" ] || fail "COMMIT differs from the executable identity"
[ -z "$expected_commit" ] || [ "$commit" = "$expected_commit" ] || fail "archive source commit differs from the requested commit"
[ -z "$version" ] || [ "$actual_version" = "$version" ] || fail "archive contains $actual_version, expected $version"
manifest_hash=$(field manifest_sha256)
case "$distribution" in
    standalone)
        printf '%s\n' "$manifest_hash" | grep -Eq '^[0-9a-f]{64}$' || fail "CLI has no valid worker manifest identity"
        grep -qx compatibility.json "$work/entries" || fail "standalone archive omitted its worker manifest"
        extract compatibility.json 1048576
        printf '%s  %s\n' "$manifest_hash" "$work/compatibility.json" | sha256sum --status -c - || fail "worker manifest checksum mismatch"
        ;;
    source)
        [ "$require_published" = false ] || fail "a published worker manifest is required; this archive is a source/development CLI"
        [ -n "$archive" ] || fail "published releases must contain a standalone CLI"
        [ -z "$manifest_hash" ] && ! grep -qx compatibility.json "$work/entries" || fail "development archive claims standalone compatibility"
        ;;
    *) fail "only standalone or local development CLI archives can be installed" ;;
esac
if [ -z "$archive" ] && [ -z "$version" ]; then
    case "$actual_version" in *-*) fail "latest release is a prerelease; select it explicitly with --version" ;; esac
fi

mkdir -p -- "$install_dir" || fail "cannot create installation directory; choose a writable --dir"
install_dir=$(cd -- "$install_dir" && pwd -P) || fail "cannot resolve installation directory"
case "$install_dir/" in
    /usr/bin/|/bin/|*/vendor/*|*/node_modules/*|*/Cellar/*|/nix/store/*|/snap/*)
        fail "directory belongs to a managed installation; use its package manager or another --dir" ;;
esac
target=$install_dir/phpwarden
exec 9< "$install_dir"
flock -w 30 9 || fail "timed out waiting for another installation"
[ ! -L "$target" ] || fail "destination is a symlink; choose another --dir"
if [ -e "$target" ]; then
    [ -f "$target" ] || fail "destination is not a regular file"
    if [ -x "$target" ] && cmp -s "$work/phpwarden" "$target"; then
        printf 'PHPWarden %s is already installed: %s\n' "$actual_version" "$target"
        exit 0
    fi
    [ "$force" = true ] || fail "destination already exists; use phpwarden update or --force to replace it"
fi
staged=$(mktemp "$install_dir/.phpwarden-install.XXXXXX") || fail "directory is not writable; choose another --dir"
cat "$work/phpwarden" > "$staged" || fail "cannot stage the executable"
chmod 755 "$staged" || fail "cannot set executable permissions"
mv -T -- "$staged" "$target" || fail "cannot atomically install the executable"
staged=
printf 'Installed PHPWarden %s: %s\n' "$actual_version" "$target"
if [ "$distribution" = source ]; then printf 'Development CLI: checks require matching preloaded worker images.\n'; fi
case ":${PATH:-}:" in *":$install_dir:"*) ;; *) printf 'Add %s to PATH.\n' "$install_dir" ;; esac
