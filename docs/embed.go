// Package docs ships the user guide inside the binary, so the agent can
// answer questions about Zodim itself.
package docs

import _ "embed"

//go:embed USER_GUIDE.md
var Guide string
