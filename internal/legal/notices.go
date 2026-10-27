package legal

import _ "embed"

//go:embed THIRD_PARTY_NOTICES.txt
var Notices []byte

//go:embed dependencies.json
var Dependencies []byte
