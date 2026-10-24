---
type: practice
title: JSCPD clones must come from consecutive reference windows
description: >-
  The adapter constrains JSCPD matching to consecutive windows; it never clamps
  ranges or drops invalid clones.
tags:
  - jscpd
  - adapter
  - duplication
kk_schema_version: 3
kk_id: practice-jscpd-matches-consecutive-reference-windows-only
kk_derived_from:
  - docs/BUGFIXES-2026-10-24.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
JSCPD's native Rabin-Karp matching joins hash windows even when the reference jumps backwards or to another source, which can yield negative ranges. The adapter restricts matching to consecutive reference windows using the pinned native Detector, tokenization, clone constructors and validators.

Invalid ranges are never clamped and invalid clones are never discarded to make a report pass; the root cause is fixed in matching instead.

<!-- kk:citations:start -->
# Citations

[1] [docs/BUGFIXES-2026-10-24.md](../../../docs/BUGFIXES-2026-10-24.md)
<!-- kk:citations:end -->
