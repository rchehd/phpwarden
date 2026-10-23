# Semgrep CE engine bundle

`python3 tools/semgrep/prepare.py` downloads checksum-locked official Semgrep 1.182.0 wheels and upstream sources. It extracts the CE engine and its sibling shared libraries, preserving their `$ORIGIN/libs` layout. No pip dependencies, package hooks or compilation run. Native Linux amd64 uses the glibc 2.34+ wheel; Alpine uses the musl wheel. Docker passes both archives explicitly.

The ignored `vendor` directory contains upstream notices, the LGPL-2.1 license, the pinned source archive and download metadata alongside the unmodified binary. Source revision: `240da9f30cd06fb4ee00c2c31c114c1c1f5ee0f4`. Keep these files when distributing the engine. Review the notices and shared-library distribution requirements before publishing a release.

PHPWarden invokes the pinned `semgrep-core` protocol directly. It does not install the Python CLI, Pro engine or registry rules. Rules in `internal/checker/semgrep/rules.yaml` were authored for PHPWarden and versioned independently as `1.0.0`. Project rules must be local PHP search/taint YAML rules. Remote rule references, Jsonnet, validators, executable predicates, Pro analysis and autofixes are unsupported.

Preparation and native/Docker validation are deferred until the user authorizes the combined build and test run.
