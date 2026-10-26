#!/bin/sh
set -eu
case "$(uname -s):$(uname -m)" in
    Linux:x86_64) ;;
    *) printf '%s\n' 'This PHPWarden package supports Linux amd64 only.' >&2; exit 3 ;;
esac
PHPWARDEN_EXECUTABLE=$(readlink -f "$0")
package_root=$(dirname "$(dirname "$PHPWARDEN_EXECUTABLE")")
if ! (cd "$package_root/payload" && sha256sum --status -c SHA256SUMS); then
    printf '%s\n' 'PHPWarden executable checksum mismatch; reinstall the pinned package.' >&2
    exit 3
fi
PHPWARDEN_INSTALLATION=@@INSTALLATION@@
export PHPWARDEN_EXECUTABLE PHPWARDEN_INSTALLATION
exec "$package_root/payload/phpwarden" "$@"
