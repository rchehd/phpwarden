---
type: practice
title: Run the Python harness and Go suites from the repo root
description: >-
  Exact commands for Python harness checks, Go vet/race tests, bundle installs
  and native worker acceptance.
tags:
  - testing
  - ci
  - commands
kk_schema_version: 3
kk_id: practice-run-python-and-go-test-suites
kk_derived_from:
  - docs/TESTING.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Python harness (Python 3.12 and Git): create `.venv`, `pip install -r requirements-dev.txt`, then `.venv/bin/python -m ruff check tests`, `ruff format --check tests`, and `python -m unittest discover -s tests -v`. The harness itself needs only the standard library and Git, so plain `python3 -m unittest discover -s tests -v` works without Ruff. All commands are prefixed with `rtk proxy`.

Go (1.26.5): `go vet ./...`, `go test -race ./...`, `go build -o bin/phpwarden ./cmd/phpwarden`, `bin/phpwarden config validate --config examples/drupal.phpwarden.yaml`, `php -l` on each `internal/checker/*/helper.php`, then `python3 -m unittest tests.acceptance.worker_cli -v`. `worker_cli` is run explicitly and is not part of fixture discovery.

Native Go and worker tests need the locked tool bundles installed first: `composer install --working-dir tools/<bundle> --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction` for each bundle, plus `npm ci --prefix tools/node --ignore-scripts --no-audit --no-fund`. They also need PHP 8.4 with tokenizer/mbstring/XML, Node >=22.12, and Gitleaks 8.30.1 on PATH. Native PHPCS tests fail on a host without `mbstring`; CI provides it.

<!-- kk:citations:start -->
# Citations

[1] [docs/TESTING.md](../../../docs/TESTING.md)
<!-- kk:citations:end -->
