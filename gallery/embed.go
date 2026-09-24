// Package gallery ships the starter gallery index inside the binary, used
// when the community index cannot be reached.
package gallery

import _ "embed"

//go:embed index.json
var Index []byte
