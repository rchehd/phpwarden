#!/bin/sh
set -eu
umask 077
export LC_ALL=C

usage() {
    cat <<'EOF'
Usage: sh setup.sh status [--project DIR] [--cli FILE]
       sh setup.sh install [--project DIR] [--version VERSION] [--dir DIR]
       sh setup.sh install --archive FILE [--checksums FILE] [--version VERSION] [--project DIR] [--dir DIR]
       sh setup.sh install --ci-run ID [--version VERSION] [--project DIR] [--dir DIR]

status reports selected host CLI identity and project prerequisites without downloads or project writes.
install reuses an existing CLI; otherwise it invokes the bundled canonical installer.
Project vendor/bin and node_modules/.bin wrappers take precedence over PATH and ~/.local/bin.
CI installation requires gh authentication, an exact successful official Build run and published workers.
No sudo, hooks, MCP, project init, Docker builds or analyzer runs. Existing CLI files are not replaced.
EOF
}

fail() { printf 'PHPWarden setup: %s\n' "$*" >&2; exit 3; }
invalid() { printf 'PHPWarden setup: %s\n' "$*" >&2; exit 2; }
single_line() {
    printf '%s\n' "$1" | awk 'NR != 1 || /[[:cntrl:]]/ {bad=1} END {exit bad}'
}

action=${1:---help}
case "$action" in --help|-h) usage; exit 0 ;; status|install) shift ;; *) invalid 'expected status or install; see --help' ;; esac
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
project=$(pwd -P)
invocation=$project
explicit_cli=
version=
archive=
checksums=
install_dir=
ci_run=
work=
seen=' '
cleanup() { if [ -n "$work" ]; then rm -rf -- "$work"; fi; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
while [ "$#" -gt 0 ]; do
    option=$1
    shift
    case "$option" in
        --help|-h) usage; exit 0 ;;
        --project|--cli|--version|--archive|--checksums|--dir|--ci-run) ;;
        *) invalid 'unknown option; see --help' ;;
    esac
    case "$seen" in *" $option "*) invalid "duplicate option: $option" ;; esac
    seen="$seen$option "
    [ "$#" -gt 0 ] && [ -n "$1" ] || invalid "$option requires a value"
    case "$1" in --*) invalid "$option requires a value" ;; esac
    single_line "$1" || invalid 'option values must be single-line paths or identifiers'
    value=$1
    shift
    case "$option" in
        --project) project=$value ;;
        --cli) explicit_cli=$value ;;
        --version) version=$value ;;
        --archive) archive=$value ;;
        --checksums) checksums=$value ;;
        --dir) install_dir=$value ;;
        --ci-run) ci_run=$value ;;
    esac
done
if [ "$action" = status ]; then
    [ -z "$version$archive$checksums$install_dir$ci_run" ] || invalid 'installation options require install'
fi
[ -z "$archive" ] || [ -z "$ci_run" ] || invalid 'choose an offline archive or a CI run'
[ -z "$checksums" ] || [ -n "$archive" ] || invalid '--checksums requires --archive'
if [ -n "$ci_run" ]; then
    printf '%s\n' "$ci_run" | grep -Eq '^[1-9][0-9]{0,19}$' || invalid 'CI run must be a numeric ID'
fi
if [ "$action" = install ]; then
    case "$(uname -s):$(uname -m)" in Linux:x86_64|Linux:amd64) ;; *) fail 'CLI installation supports Linux amd64 only' ;; esac
fi
project=$(CDPATH= cd -- "$project" && pwd -P) || invalid 'project must be an existing directory'
single_line "$project" || invalid 'project path contains terminal controls'
case "$archive" in ''|/*) ;; *) archive=$invocation/$archive ;; esac
case "$checksums" in ''|/*) ;; *) checksums=$invocation/$checksums ;; esac
case "$install_dir" in ''|/*) ;; *) install_dir=$invocation/$install_dir ;; esac
case "$explicit_cli" in ''|/*) ;; *) explicit_cli=$invocation/$explicit_cli ;; esac
cd -- "$project"
cli=
source=
select_cli() {
    candidate=$1
    single_line "$candidate" || fail 'selected CLI path contains terminal controls'
    [ -e "$candidate" ] || [ -L "$candidate" ] || return 1
    [ -f "$candidate" ] && [ -x "$candidate" ] || fail "selected CLI is not executable: $candidate"
    case "$candidate" in /*) cli=$candidate ;; *) cli=$project/$candidate ;; esac
    source=$2
}
if [ -n "$explicit_cli" ]; then
    select_cli "$explicit_cli" explicit || fail 'explicit CLI is missing; use --dir to select a new installation destination'
elif select_cli "$project/vendor/bin/phpwarden" composer; then :
elif select_cli "$project/node_modules/.bin/phpwarden" npm; then :
elif candidate=$(command -v phpwarden 2>/dev/null) && select_cli "$candidate" PATH; then :
elif [ -n "${HOME:-}" ] && select_cli "$HOME/.local/bin/phpwarden" user; then :
fi
probe() {
    if [ -z "$work" ]; then work=$(mktemp -d "${TMPDIR:-/tmp}/phpwarden-bootstrap.XXXXXX") || fail 'cannot create private scratch'; fi
    if ! (ulimit -f 128; timeout 15 "$cli" --build-info > "$work/info" 2> "$work/error"); then
        fail "selected CLI cannot report its build identity: $cli"
    fi
    field() { sed -n "s/^[[:space:]]*\"$1\":[[:space:]]*\"\([^\"]*\)\"[,]*[[:space:]]*$/\1/p" "$work/info"; }
    cli_version=$(field version)
    cli_commit=$(field commit)
    distribution=$(field distribution)
    cli_platform=$(field platform)
    manifest=$(field manifest_sha256)
    single_line "$cli_version$cli_commit$distribution$cli_platform$manifest" || fail 'CLI identity has duplicate or unsafe fields'
    [ -n "$cli_version" ] && [ -n "$cli_commit" ] && [ "$cli_platform" = linux/amd64 ] || fail 'CLI identity is missing required Linux amd64 fields'
    case "$distribution" in source|standalone) ;; *) fail 'use the host CLI; container/Compose commands cannot bootstrap the host' ;; esac
    if [ "$distribution" = standalone ]; then
        printf '%s\n' "$manifest" | grep -Eq '^[0-9a-f]{64}$' || fail 'standalone CLI has no valid published worker manifest identity'
    fi
}
if [ -n "$cli" ]; then
    probe
    [ -z "$version" ] || [ "v${version#v}" = "$cli_version" ] || fail 'selected CLI has another version; use its update command or package manager explicitly'
elif [ "$action" = install ]; then
    expected_commit=
    if [ -n "$ci_run" ]; then
        command -v gh >/dev/null 2>&1 || fail 'gh is required for CI downloads; alternatively provide --archive and --checksums'
        metadata=$(gh run view "$ci_run" --repo rchehd/phpwarden --json workflowName,headSha,status,conclusion --jq '[.workflowName,.headSha,.status,.conclusion]|@tsv') || fail 'cannot read the official CI run'
        IFS=' ' read -r workflow expected_commit state conclusion <<EOF
$(printf '%s' "$metadata" | tr '\t' ' ')
EOF
        [ "$workflow" = Build ] && [ "$state" = completed ] && [ "$conclusion" = success ] || fail 'CI source must be a completed successful official Build run'
        printf '%s\n' "$expected_commit" | grep -Eq '^[0-9a-f]{40}$' || fail 'CI run has no exact source commit'
        work=$(mktemp -d "${TMPDIR:-/tmp}/phpwarden-bootstrap.XXXXXX") || fail 'cannot create private scratch'
        gh run download "$ci_run" --repo rchehd/phpwarden --name "phpwarden-linux-amd64-$expected_commit" --dir "$work/download" || fail 'CI run has no downloadable CLI artifact'
        [ -f "$work/download/compatibility.json" ] && [ ! -L "$work/download/compatibility.json" ] || fail 'CI candidate has no published worker manifest; use Build all with image publication, not CLI-only'
        archive=$work/download/phpwarden-linux-amd64.tar.gz
        checksums=$work/download/SHA256SUMS
    fi
    set -- sh "$script_dir/install.sh"
    if [ -n "$archive" ]; then set -- "$@" --archive "$archive"; fi
    if [ -n "$checksums" ]; then set -- "$@" --checksums "$checksums"; fi
    if [ -n "$version" ]; then set -- "$@" --version "$version"; fi
    if [ -n "$expected_commit" ]; then set -- "$@" --commit "$expected_commit" --require-published; fi
    if [ -n "$install_dir" ]; then set -- "$@" --dir "$install_dir"; else
        [ -n "${HOME:-}" ] || invalid 'set HOME or pass --dir'
        install_dir=$HOME/.local/bin
    fi
    "$@" || fail 'CLI installation failed; use an available release or an exact published-worker CI candidate'
    cli=$(CDPATH= cd -- "$install_dir" && pwd -P)/phpwarden
    source=user
    probe
fi
printf 'PHPWarden setup status\nProject: %s\n' "$project"
if [ -n "$cli" ]; then
    printf 'CLI: %s\nSource: %s\nVersion: %s\nCommit: %s\nDistribution: %s\n' "$cli" "$source" "$cli_version" "$cli_commit" "$distribution"
    if [ "$distribution" = standalone ] && printf '%s\n' "$manifest" | grep -Eq '^[0-9a-f]{64}$'; then
        printf 'Worker delivery: published manifest; selected image can be downloaded by the CLI\n'
    else
        printf 'Worker delivery: matching preloaded development images required\n'
    fi
else
    printf 'CLI: missing\n'
fi
if [ -L "$project/.phpwarden.yaml" ]; then
    printf 'Project config: symlink; review before setup\n'
elif [ -f "$project/.phpwarden.yaml" ]; then
    printf 'Project config: present (not validated)\n'
else
    printf 'Project config: missing\n'
fi
if command -v docker >/dev/null 2>&1; then printf 'Docker CLI: available (daemon not checked)\n'; else printf 'Docker CLI: missing\n'; fi
printf 'Worker selection and availability: not checked; use the selected CLI resolve/doctor for this project\n'
[ -n "$cli" ] || exit 3
