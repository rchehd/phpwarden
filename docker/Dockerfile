FROM golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc AS build

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY schemas ./schemas
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/phpwarden ./cmd/phpwarden

FROM php:8.4.26-cli-alpine3.23@sha256:da7a7a945a493ac9c015f327231afd820bbe57fc6ae22f4f520a79fbd635ba39 AS worker

RUN apk add --no-cache git ca-certificates \
    && addgroup -S -g 10001 phpwarden \
    && adduser -S -D -H -u 10001 -G phpwarden phpwarden \
    && git config --system --add safe.directory /workspace \
    && php -r 'if (!function_exists("token_get_all") || PHP_VERSION !== "8.4.26") { exit(1); }'

COPY --from=build /out/phpwarden /usr/local/bin/phpwarden
LABEL org.opencontainers.image.title="PHPWarden static worker" \
    io.phpwarden.protocol="2" \
    io.phpwarden.toolset="static-dev-v1-php8.4" \
    io.phpwarden.php-version="8.4.26" \
    io.phpwarden.checkers="debug,syntax"

WORKDIR /workspace
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/phpwarden"]
CMD ["worker", "check"]
