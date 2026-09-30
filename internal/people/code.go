package people

import (
	"crypto/rand"
	"crypto/subtle"
	"math/big"
	"strings"
	"time"
)

// Pairing codes (the owner's and invites) make a chat someone's, so they
// are long enough that guessing is hopeless and they do not last.
const (
	// codeAlphabet leaves out look-alikes (0/O, 1/I/L).
	codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	codeLength   = 10
	// InviteLife is how long an invite works; the People page shows a
	// fresh one after that.
	InviteLife = 24 * time.Hour
)

// NewCode returns a random code from the unambiguous alphabet.
func NewCode() string {
	b := make([]byte, codeLength)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b)
}

// NormCode is how a typed code is compared: case, spaces and dashes do
// not matter.
func NormCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(code)))
}

// SameCode compares two codes in constant time; an empty code matches
// nothing.
func SameCode(a, b string) bool {
	a, b = NormCode(a), NormCode(b)
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// inviteMatches says whether an invite code opens p: it must be p's
// current, unexpired invite.
func inviteMatches(p Person, code string, now time.Time) bool {
	return p.Invite != "" && now.Before(p.InviteUntil) && SameCode(p.Invite, code)
}
