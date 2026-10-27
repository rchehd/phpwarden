#!/bin/sh
set -eu
kind="$1"
minor="$2"
expected="$3"
test "$(uname -s)" = Linux
test "$(uname -m)" = x86_64
php_bin=php
if [ "$kind" = ddev ]; then
    php_bin="php$minor"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y --no-install-recommends "php$minor-dev" autoconf gcc libc6-dev make pkg-config zlib1g-dev
    phpize_bin="phpize$minor"
    php_config="php-config$minor"
else
    test -x /usr/local/bin/phpize
    phpize_bin=/usr/local/bin/phpize
    php_config=/usr/local/bin/php-config
    if ! command -v cc >/dev/null || ! command -v autoconf >/dev/null || ! command -v make >/dev/null || [ ! -f /usr/include/zlib.h ]; then
        if command -v apk >/dev/null; then
            apk add --no-cache autoconf build-base pkgconf zlib-dev
        elif command -v apt-get >/dev/null; then
            export DEBIAN_FRONTEND=noninteractive
            apt-get update
            apt-get install -y --no-install-recommends autoconf gcc libc6-dev make pkg-config zlib1g-dev
        else
            echo 'Unsupported PHP build environment' >&2
            exit 1
        fi
    fi
fi
"$php_bin" -r 'exit(PHP_ZTS || PHP_INT_SIZE !== 8 || PHP_MAJOR_VERSION !== 8 || PHP_MINOR_VERSION < 1 || PHP_MINOR_VERSION > 4 || extension_loaded("spx") ? 1 : 0);'
test "$("$php_bin" -r 'echo PHP_MAJOR_VERSION,".",PHP_MINOR_VERSION;')" = "$minor"
printf '%s  %s\n' "$expected" /tmp/phpwarden-spx.tar.gz | sha256sum -c -
mkdir /tmp/phpwarden-spx-source
tar -xzf /tmp/phpwarden-spx.tar.gz --strip-components=1 -C /tmp/phpwarden-spx-source
cd /tmp/phpwarden-spx-source
"$phpize_bin"
CI=true ./configure --enable-spx --with-php-config="$php_config"
make -j2
mkdir -p /opt/phpwarden-spx
cp modules/spx.so /opt/phpwarden-spx/spx.so
cp /tmp/phpwarden-spx.tar.gz /opt/phpwarden-spx/source.tar.gz
cp /tmp/phpwarden-spx-build.sh /opt/phpwarden-spx/build.sh
printf '%s\n' "$expected" > /opt/phpwarden-spx/source.sha256
chmod 0644 /opt/phpwarden-spx/source.tar.gz /opt/phpwarden-spx/build.sh /opt/phpwarden-spx/source.sha256
"$php_bin" -n -d extension=/opt/phpwarden-spx/spx.so -r 'exit(extension_loaded("spx") ? 0 : 1);'
cp LICENSE /opt/phpwarden-spx/LICENSE
cd /
rm -rf /tmp/phpwarden-spx-source /tmp/phpwarden-spx.tar.gz /tmp/phpwarden-spx-build.sh
