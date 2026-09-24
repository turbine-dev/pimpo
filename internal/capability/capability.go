// Package capability defines what a routine may do in the world. A routine
// reaches nothing else: every call goes through a capability by name.
package capability

import (
	"fmt"
	"sort"
	"strings"
)

type Risk int

const (
	// Read has no side effects.
	Read Risk = iota
	// Notify sends a message to the owner and nobody else.
	Notify
	// Reversible changes something that can be undone (archive, label, draft).
	Reversible
	// Irreversible cannot be undone (send to others, delete forever, pay).
	Irreversible
)

func (r Risk) String() string {
	return [...]string{"read", "notify", "reversible", "irreversible"}[r]
}

// Spec documents one capability for the compiler, the policy engine and the UI.
type Spec struct {
	Name string
	Risk Risk
	// Signature is how routines call it, e.g. `gmail.search({query, days, unread, max})`.
	Signature string
	Returns   string
	// Scoped capabilities take a scope after a colon in the manifest,
	// e.g. `http.getJSON:api.open-meteo.com`.
	Scoped bool
}

func (s Spec) Writes() bool { return s.Risk != Read }

// Catalog is every capability Vigia knows. Connectors register theirs here.
var Catalog = map[string]Spec{}

func Register(s Spec) { Catalog[s.Name] = s }

func init() {
	for _, s := range []Spec{
		{Name: "calendar.events", Risk: Read, Signature: "calendar.events({from, to})", Returns: "[{id, title, start, end, location, attendees: [email], calendar}] with ISO 8601 times"},
		{Name: "gmail.search", Risk: Read, Signature: "gmail.search({query, days, unread, max})", Returns: "[{id, from, from_name, to, subject, snippet, date, labels: [string], replied: bool, unread: bool, can_unsubscribe: bool}] newest first; query uses Gmail search syntax"},
		{Name: "gmail.archive", Risk: Reversible, Signature: "gmail.archive({id})", Returns: "{ok}"},
		{Name: "gmail.label", Risk: Reversible, Signature: "gmail.label({id, label})", Returns: "{ok}"},
		{Name: "gmail.trash", Risk: Reversible, Signature: "gmail.trash({id})", Returns: "{ok}; moves the message to Trash, where it can be restored"},
		{Name: "gmail.delete", Risk: Irreversible, Signature: "gmail.delete({id})", Returns: "{ok}; deletes the message permanently"},
		{Name: "gmail.draft", Risk: Reversible, Signature: "gmail.draft({to, subject, body})", Returns: "{ok}; saves a draft without sending"},
		{Name: "gmail.unsubscribe", Risk: Irreversible, Signature: "gmail.unsubscribe({id})", Returns: "{ok, method}; asks the sender to stop sending (one-click link or unsubscribe email); only for messages where can_unsubscribe is true"},
		{Name: "gmail.send", Risk: Irreversible, Signature: "gmail.send({to, subject, body})", Returns: "{ok}; sends an email to someone else"},
		{Name: "http.getJSON", Risk: Read, Signature: "http.getJSON(url)", Returns: "the parsed JSON body; only hosts named in the manifest scope are reachable", Scoped: true},
		{Name: "telegram.send", Risk: Notify, Signature: "telegram.send({text})", Returns: "{ok}; sends a message to the owner only"},
		{Name: "whatsapp.send", Risk: Notify, Signature: "whatsapp.send({text})", Returns: "{ok}; sends a WhatsApp message to the owner only"},
		{Name: "whatsapp.send_to", Risk: Irreversible, Signature: "whatsapp.send_to({to, text})", Returns: "{ok}; sends a WhatsApp message to someone else (to is a phone number); always needs approval"},
	} {
		Register(s)
	}
}

// Parse splits a manifest entry into capability name and scope.
func Parse(entry string) (Spec, string, error) {
	name, scope, _ := strings.Cut(entry, ":")
	s, ok := Catalog[name]
	if !ok {
		return Spec{}, "", fmt.Errorf("unknown capability %q", name)
	}
	if s.Scoped && scope == "" {
		return Spec{}, "", fmt.Errorf("capability %q needs a scope, e.g. %s:example.com", name, name)
	}
	if !s.Scoped && scope != "" {
		return Spec{}, "", fmt.Errorf("capability %q takes no scope", name)
	}
	return s, scope, nil
}

// Names lists the catalog in a stable order.
func Names() []string {
	out := make([]string, 0, len(Catalog))
	for n := range Catalog {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
