PCOV 1.0.12 is compiled against each worker's pinned PHP runtime. The extension
and upstream PHP-3.01 license are shipped under `/usr/local/lib/phpwarden/pcov/`.
`sources.json` records the immutable archive and SHA256 used by the Dockerfile.

PCOV remains disabled for static checks and ordinary project tests. Only opt-in
project test coverage enables it with `-d pcov.enabled=1`; it runs against the
private test workspace and does not change the application's PHP container.
Native php-code-coverage produces line coverage and CRAP reports; PCOV does not
provide branch/path coverage. The host CLI never unserializes PHP coverage data.
