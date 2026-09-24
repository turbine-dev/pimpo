// Package protection ships the signed starter protection list inside the
// binary, used until a newer list is downloaded.
package protection

import _ "embed"

//go:embed list.json
var List []byte

// Keys are the maintainers' public keys a list must be signed with.
var Keys = []string{"M5A3gNnanvJht/NVx2j+5f4VaowV4BIcf89fEN0numI="}
