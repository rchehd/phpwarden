---
type: practice
title: Expose all project settings in the generated init config
description: >-
  Init makes supported settings discoverable, keeping optional settings
  commented and existing project configuration intact.
tags:
  - config
  - init
  - usability
kk_schema_version: 3
kk_id: practice-expose-all-project-settings-in-the-generated-init-config
kk_derived_from:
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:practice:0'
kk_relates_to:
  - practice-project-config-owns-rules
  - practice-keep-native-tool-options-in-native-configs
kk_depends_on: []
kk_confidence: high
---
`phpwarden init` generates a complete, discoverable `.phpwarden.yaml`: the core settings and checker switches are active, while optional settings are shown as commented examples. Users should be able to see what can be changed without guessing undocumented keys. Existing project configuration is preserved on repeated init; the full example supplies blocks to copy into it.

<!-- kk:related:start -->
# Related

- Related: [practice-project-config-owns-rules](practice-project-config-owns-rules.md)
- Related: [practice-keep-native-tool-options-in-native-configs](practice-keep-native-tool-options-in-native-configs.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 01a1693d-cb80-7003-ae47-1468fd6435b0:practice:0
<!-- kk:citations:end -->
