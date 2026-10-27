ARG WORKER_IMAGE=phpwarden:static-php8.3-dev
FROM ${WORKER_IMAGE}
USER root
RUN apk add --no-cache libpng-dev sqlite && docker-php-ext-install -j2 gd
