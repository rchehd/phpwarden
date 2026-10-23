FROM golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc AS build

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY schemas ./schemas
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/phpwarden ./cmd/phpwarden

FROM php:8.4.26-cli-alpine3.23@sha256:da7a7a945a493ac9c015f327231afd820bbe57fc6ae22f4f520a79fbd635ba39 AS tools

RUN apk add --no-cache unzip ca-certificates
ADD --checksum=sha256:7a2d379d5b8ffdaa028580ef26494c36d2feef4b178d3dd1473a4dbc5e17c8d6 https://getcomposer.org/download/2.10.3/composer.phar /usr/local/bin/composer
WORKDIR /opt/phpwarden/tools/yaml
COPY tools/yaml/composer.json tools/yaml/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("symfony/yaml") !== "v7.4.21") { exit(1); }'

WORKDIR /opt/phpwarden/tools/phpcs
COPY tools/phpcs/composer.json tools/phpcs/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("squizlabs/php_codesniffer") !== "4.0.4" || !extension_loaded("mbstring")) { exit(1); }'

WORKDIR /opt/phpwarden/tools/phpstan
COPY tools/phpstan/composer.json tools/phpstan/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("phpstan/phpstan") !== "2.3.1") { exit(1); }'

FROM php:8.4.26-cli-alpine3.23@sha256:da7a7a945a493ac9c015f327231afd820bbe57fc6ae22f4f520a79fbd635ba39 AS worker

RUN apk add --no-cache git ca-certificates \
    && addgroup -S -g 10001 phpwarden \
    && adduser -S -D -H -u 10001 -G phpwarden phpwarden \
    && git config --system --add safe.directory /workspace \
    && php -r 'if (!function_exists("token_get_all") || PHP_VERSION !== "8.4.26") { exit(1); }'

COPY --from=build /out/phpwarden /usr/local/bin/phpwarden
COPY --from=tools /opt/phpwarden/tools/yaml /usr/local/lib/phpwarden/yaml
COPY --from=tools /opt/phpwarden/tools/phpcs /usr/local/lib/phpwarden/phpcs
COPY --from=tools /opt/phpwarden/tools/phpstan /usr/local/lib/phpwarden/phpstan
LABEL org.opencontainers.image.title="PHPWarden static worker" \
    io.phpwarden.protocol="2" \
    io.phpwarden.toolset="static-dev-v6-php8.4" \
    io.phpwarden.php-version="8.4.26" \
    io.phpwarden.checkers="debug,drupal-config,phpcs,phpstan,syntax,whitespace,yaml" \
    io.phpwarden.symfony-yaml-version="7.4.21" \
    io.phpwarden.phpcs-version="4.0.4" \
    io.phpwarden.coder-version="9.0.1" \
    io.phpwarden.phpcompatibility-version="10.0.0-alpha2" \
    io.phpwarden.phpstan-version="2.3.1" \
    io.phpwarden.phpstan-drupal-version="2.2.2" \
    io.phpwarden.phpstan-deprecations-version="2.0.5"

WORKDIR /workspace
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/phpwarden"]
CMD ["worker", "check"]
