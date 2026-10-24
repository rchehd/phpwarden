---
type: map
title: 'Semgrep CE engine bundle: preparation, invocation and supported rules'
description: >-
  tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle;
  PHPWarden calls it directly and accepts only local PHP rules.
tags:
  - semgrep
  - bundle
  - licensing
  - rules
kk_schema_version: 3
kk_id: map-semgrep-engine-bundle-and-rule-limits
kk_derived_from:
  - tools/semgrep/README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
`tools/semgrep/prepare.py` downloads checksum-locked official Semgrep wheels (glibc and musl variants) plus the upstream source archive, and extracts only the CE engine and its sibling shared libraries, keeping their `$ORIGIN/libs` layout. No pip dependencies, package hooks or compilation run. The ignored `vendor` directory holds the unmodified binary beside upstream notices, the LGPL-2.1 license, the pinned source and download metadata; these files must ship with any distributed engine.

PHPWarden speaks the `semgrep-core` protocol directly; it does not install the Python CLI, the Pro engine or registry rules. The built-in rules in `internal/checker/semgrep/rules.yaml` are authored for PHPWarden and versioned independently. Project rules must be local PHP search/taint YAML rules; remote rule references, Jsonnet, validators, executable predicates, Pro analysis and autofixes are unsupported.

<!-- kk:citations:start -->
# Citations

[1] [tools/semgrep/README.md](../../../../tools/semgrep/README.md)
<!-- kk:citations:end -->
