# check=skip=InvalidDefaultArgInFrom
ARG PHP_IMAGE

FROM golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc AS build

ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY buildinfo.go distribution_assets.go docker-bake.json ./
COPY examples/compose ./examples/compose
COPY cmd ./cmd
COPY internal ./internal
COPY schemas ./schemas
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/rchehd/phpwarden.Distribution=container -X github.com/rchehd/phpwarden.Version=${BUILD_VERSION} -X github.com/rchehd/phpwarden.Commit=${BUILD_COMMIT}" -o /out/phpwarden ./cmd/phpwarden

FROM node:22.22.3-alpine3.22@sha256:cd7807368cf24826297cbad5dca1a44972ccfd770647db52a8c7589eb4599ac8 AS node-tools

WORKDIR /opt/phpwarden/tools/node
COPY tools/node/package.json tools/node/package-lock.json ./
RUN npm ci --ignore-scripts --omit=dev --no-audit --no-fund

FROM node:22.22.3-alpine3.22@sha256:cd7807368cf24826297cbad5dca1a44972ccfd770647db52a8c7589eb4599ac8 AS semgrep-tools

RUN apk add --no-cache python3 ca-certificates
WORKDIR /opt/phpwarden/tools/semgrep
COPY tools/semgrep/sources.json tools/semgrep/prepare.py ./
ADD --checksum=sha256:6263233c3f2e7b05c2d89b1de7408e91c0238014ab67bb68746c0c452d33b828 https://files.pythonhosted.org/packages/7d/3e/52c40aacf843d10820525b78dfe57af1bac936ab63b38cbf79dda4632b8a/semgrep-1.182.0-cp310.cp311.cp312.cp313.cp314.py310.py311.py312.py313.py314-none-musllinux_1_2_x86_64.whl /tmp/semgrep.whl
ADD --checksum=sha256:780047c94f5cb5ee7b6093484e232cf2dc5106cc6b01a0609a2d15d5b7518c85 https://codeload.github.com/semgrep/semgrep/tar.gz/240da9f30cd06fb4ee00c2c31c114c1c1f5ee0f4 /tmp/semgrep-source.tar.gz
RUN python3 prepare.py --platform musllinux_1_2_x86_64 --wheel-archive /tmp/semgrep.whl --source-archive /tmp/semgrep-source.tar.gz \
    && rm /tmp/semgrep.whl /tmp/semgrep-source.tar.gz

FROM ${PHP_IMAGE} AS tools

ARG PHP_RUNTIME

RUN apk add --no-cache unzip ca-certificates python3
ADD --checksum=sha256:7a2d379d5b8ffdaa028580ef26494c36d2feef4b178d3dd1473a4dbc5e17c8d6 https://getcomposer.org/download/2.10.3/composer.phar /usr/local/bin/composer
WORKDIR /opt/phpwarden/tools/yaml
COPY tools/yaml/composer.json tools/yaml/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("symfony/yaml") !== "v6.4.48") { exit(1); }'

WORKDIR /opt/phpwarden/tools/composer-require-checker/php${PHP_RUNTIME}
COPY tools/composer-require-checker/php${PHP_RUNTIME}/composer.json tools/composer-require-checker/php${PHP_RUNTIME}/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative

WORKDIR /opt/phpwarden/tools/composer-unused
COPY tools/composer-unused/composer.json tools/composer-unused/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative

WORKDIR /opt/phpwarden/tools/parallel-lint
COPY tools/parallel-lint/composer.json tools/parallel-lint/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative

WORKDIR /opt/phpwarden/tools/deptrac
COPY tools/deptrac/composer.json tools/deptrac/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("deptrac/deptrac") !== "4.4.0" || Composer\InstalledVersions::getPrettyVersion("phpstan/phpstan") !== "2.3.1") { exit(1); }'

WORKDIR /opt/phpwarden/tools/rector
COPY tools/rector/composer.json tools/rector/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("rector/rector") !== "2.6.1" || Composer\InstalledVersions::getPrettyVersion("palantirnet/drupal-rector") !== "1.1.3") { exit(1); }'

WORKDIR /opt/phpwarden/tools/phpcs
COPY tools/phpcs/composer.json tools/phpcs/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("squizlabs/php_codesniffer") !== "4.0.4" || !extension_loaded("mbstring")) { exit(1); }'

WORKDIR /opt/phpwarden/tools/phpstan
COPY tools/phpstan/composer.json tools/phpstan/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("phpstan/phpstan") !== "2.3.1" || Composer\InstalledVersions::getPrettyVersion("shipmonk/dead-code-detector") !== "1.4.2" || Composer\InstalledVersions::getPrettyVersion("phpstan/phpstan-strict-rules") !== "2.1.0") { exit(1); }'

WORKDIR /opt/phpwarden/tools/twig-cs-fixer
COPY tools/twig-cs-fixer/composer.json tools/twig-cs-fixer/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("vincentlanglet/twig-cs-fixer") !== "4.1.1") { exit(1); }'

WORKDIR /opt/phpwarden/tools/composer
COPY tools/composer/composer.json tools/composer/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\Composer::getVersion() !== "2.10.3") { exit(1); }'

WORKDIR /opt/phpwarden/tools/phpmd
COPY tools/phpmd/composer.json tools/phpmd/composer.lock ./
RUN php /usr/local/bin/composer install --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction --classmap-authoritative \
    && php -r 'require "vendor/autoload.php"; if (Composer\InstalledVersions::getPrettyVersion("phpmd/phpmd") !== "3.0.0" || !extension_loaded("xml")) { exit(1); }'

ADD --checksum=sha256:551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz /tmp/gitleaks.tar.gz
RUN tar -xzf /tmp/gitleaks.tar.gz -C /usr/local/bin gitleaks \
    && rm /tmp/gitleaks.tar.gz \
    && test "$(gitleaks version)" = "8.30.1"

WORKDIR /opt/phpwarden/tools/eventhorizon
COPY tools/eventhorizon/sources.json tools/eventhorizon/prepare.py ./
ADD --checksum=sha256:f02e6f78411f90ef6db1d9e7727f4e6a66d220938bc195a67cd902fcb8eec430 https://codeload.github.com/qed42/eventhorizon-cli-mode/tar.gz/ae11df1cd8e4e2b843c6c53bac60026ef344935f /tmp/eventhorizon.tar.gz
ADD --checksum=sha256:d76623373421df22fb4cf8817020cbb7ef15c725b9d5e45f17e189bfc384190f https://files.pythonhosted.org/packages/05/8e/961c0007c59b8dd7729d542c61a4d537767a59645b82a0b521206e1e25c2/pyyaml-6.0.3.tar.gz /tmp/pyyaml.tar.gz
RUN python3 prepare.py --source-archive /tmp/eventhorizon.tar.gz --yaml-archive /tmp/pyyaml.tar.gz \
    && rm /tmp/eventhorizon.tar.gz /tmp/pyyaml.tar.gz


FROM ${PHP_IMAGE} AS worker

ARG REQUIRE_CHECKER_VERSION=4.16.1

ARG PHP_VERSION
ARG PHP_RUNTIME
ARG TOOLSET_ID
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown

RUN test -n "$TOOLSET_ID" || { echo "TOOLSET_ID is required; use docker buildx bake" >&2; exit 1; }
RUN apk add --no-cache git ca-certificates libstdc++ python3 \
    && addgroup -S -g 10001 phpwarden \
    && adduser -S -D -H -u 10001 -G phpwarden phpwarden \
    && git config --system --add safe.directory /workspace \
    && php -r 'if (!function_exists("token_get_all") || PHP_VERSION !== getenv("PHP_VERSION") || PHP_MAJOR_VERSION . "." . PHP_MINOR_VERSION !== getenv("PHP_RUNTIME")) { exit(1); }'

COPY --from=node-tools /usr/local/bin/node /usr/local/bin/node
COPY --from=node-tools /opt/phpwarden/tools/node /usr/local/lib/phpwarden/node
COPY --from=tools /opt/phpwarden/tools/yaml /usr/local/lib/phpwarden/yaml
COPY --from=tools /opt/phpwarden/tools/phpcs /usr/local/lib/phpwarden/phpcs
COPY --from=tools /opt/phpwarden/tools/phpstan /usr/local/lib/phpwarden/phpstan
COPY --from=tools /opt/phpwarden/tools/twig-cs-fixer /usr/local/lib/phpwarden/twig-cs-fixer
COPY --from=tools /opt/phpwarden/tools/composer /usr/local/lib/phpwarden/composer
COPY --from=tools /opt/phpwarden/tools/phpmd /usr/local/lib/phpwarden/phpmd
COPY --from=tools /opt/phpwarden/tools/rector /usr/local/lib/phpwarden/rector
COPY --from=tools /opt/phpwarden/tools/deptrac /usr/local/lib/phpwarden/deptrac
COPY --from=tools /opt/phpwarden/tools/composer-require-checker /usr/local/lib/phpwarden/composer-require-checker
COPY --from=tools /opt/phpwarden/tools/composer-unused /usr/local/lib/phpwarden/composer-unused
COPY --from=tools /opt/phpwarden/tools/parallel-lint /usr/local/lib/phpwarden/parallel-lint
COPY --from=tools /usr/local/bin/gitleaks /usr/local/bin/gitleaks
COPY --from=tools /opt/phpwarden/tools/eventhorizon/vendor /usr/local/lib/phpwarden/eventhorizon/vendor
COPY --from=semgrep-tools /opt/phpwarden/tools/semgrep/vendor /usr/local/lib/phpwarden/semgrep/vendor
COPY --from=build /out/phpwarden /usr/local/bin/phpwarden
LABEL org.opencontainers.image.title="PHPWarden static worker" \
    org.opencontainers.image.version="${BUILD_VERSION}" \
    org.opencontainers.image.revision="${BUILD_COMMIT}" \
    org.opencontainers.image.source="https://github.com/rchehd/phpwarden" \
    io.phpwarden.protocol="2" \
    io.phpwarden.toolset="${TOOLSET_ID}" \
    io.phpwarden.php-version="${PHP_VERSION}" \
    io.phpwarden.checkers="composer-audit,composer-require-checker,composer-unused,composer-validate,dead-code,debug,deptrac,drupal-config,eslint,eventhorizon,gitleaks,jscpd,phpcs,phpmd,phpstan,rector,semgrep,stylelint,syntax,twig-cs-fixer,whitespace,yaml" \
    io.phpwarden.symfony-yaml-version="6.4.48" \
    io.phpwarden.phpcs-version="4.0.4" \
    io.phpwarden.coder-version="9.0.1" \
    io.phpwarden.phpcompatibility-version="10.0.0-alpha2" \
    io.phpwarden.phpstan-version="2.3.1" \
    io.phpwarden.phpstan-drupal-version="2.2.2" \
    io.phpwarden.phpstan-deprecations-version="2.0.5" \
    io.phpwarden.twig-cs-fixer-version="4.1.1" \
    io.phpwarden.twig-version="3.30.0" \
    io.phpwarden.composer-version="2.10.3" \
    io.phpwarden.phpmd-version="3.0.0" \
    io.phpwarden.pdepend-version="3.0.0" \
    io.phpwarden.dead-code-detector-version="1.4.2" \
    io.phpwarden.node-version="22.22.3" \
    io.phpwarden.eslint-version="9.39.5" \
    io.phpwarden.stylelint-version="17.16.0" \
    io.phpwarden.jscpd-version="4.3.0" \
    io.phpwarden.gitleaks-version="8.30.1" \
    io.phpwarden.composer-require-checker-version="${REQUIRE_CHECKER_VERSION}" \
    io.phpwarden.composer-unused-version="0.9.7" \
    io.phpwarden.parallel-lint-version="1.4.0" \
    io.phpwarden.deptrac-version="4.4.0" \
    io.phpwarden.deptrac-phpstan-version="2.3.1" \
    io.phpwarden.rector-version="2.6.1" \
    io.phpwarden.drupal-rector-version="1.1.3" \
    io.phpwarden.rector-phpstan-version="2.3.1" \
    io.phpwarden.drupal-finder-version="1.3.1" \
    io.phpwarden.semgrep-version="1.182.0" \
    io.phpwarden.semgrep-rules-version="1.0.0" \
    io.phpwarden.eventhorizon-version="0.1.0" \
    io.phpwarden.eventhorizon-revision="ae11df1cd8e4e2b843c6c53bac60026ef344935f" \
    io.phpwarden.pyyaml-version="6.0.3" \
    io.phpwarden.phpstan-strict-rules-version="2.1.0"

WORKDIR /workspace
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/phpwarden"]
CMD ["worker", "check"]
