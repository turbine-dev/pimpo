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
	// Schema is the JSON Schema of the arguments, shown to the explorer.
	Schema string
	// Scope names the argument that holds a URL whose host must be in the
	// manifest scope, for scoped capabilities other than http.getJSON.
	ScopeArg string
}

func (s Spec) Writes() bool { return s.Risk != Read }

// Catalog is every capability Pimpo knows. Connectors register theirs here.
var Catalog = map[string]Spec{}

func Register(s Spec) { Catalog[s.Name] = s }

func Unregister(name string) { delete(Catalog, name) }

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
		{Name: "audio.send", Risk: Notify, Signature: "audio.send({title, text, language})",
			Returns: "{ok, delivered: [destination], seconds}; reads text aloud (language like pt-BR, en-US, es-ES) and sends the recording to the destinations the owner chose for this routine, like notify.send; text up to 20000 characters, written to be heard (no markdown, no URLs read aloud)",
			Schema:  `{"type":"object","required":["title","text","language"],"properties":{"title":{"type":"string"},"text":{"type":"string"},"language":{"type":"string","description":"pt-BR, en-US, es-ES, fr-FR, de-DE, it-IT…"}}}`},
		{Name: "reminder.set", Risk: Notify, Signature: "reminder.set({at, text}) or reminder.set({in, text})",
			Returns: "{id, at}; at is an ISO 8601 time with its offset, in is a delay such as 30m, 2h or 1d; the message goes to the person who asked, once",
			Schema:  `{"type":"object","required":["text"],"properties":{"at":{"type":"string","description":"ISO 8601 time with offset, e.g. 2026-09-29T09:00:00-03:00"},"in":{"type":"string","description":"delay from now: 30m, 2h, 1d"},"text":{"type":"string","description":"what to remind, as it should be read"}}}`},
		{Name: "reminder.list", Risk: Read, Signature: "reminder.list()", Returns: "[{id, at, text}] the pending reminders, soonest first"},
		{Name: "reminder.cancel", Risk: Reversible, Signature: "reminder.cancel({id})", Returns: "{ok}",
			Schema: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`},
		{Name: "ask.owner", Risk: Notify, Signature: "ask.owner({question, options, key})",
			Returns: "{asked}; asks the person the routine works for, with 2 to 6 options as buttons; their answer runs this routine again with event.answer = {key, question, choice, index, asked}; a new question with the same key replaces a pending one; unanswered questions expire after 24 hours",
			Schema:  `{"type":"object","required":["question","options"],"properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"},"minItems":2,"maxItems":6},"key":{"type":"string","description":"names the question, e.g. treino; defaults to the question"}}}`},
		{Name: "sheets.read", Risk: Read, Signature: "sheets.read({sheet, range})", Returns: "{range, values: [[cell]]}; sheet is the spreadsheet's address or id, range like Gastos!A:D (default A:Z)",
			Schema: `{"type":"object","required":["sheet"],"properties":{"sheet":{"type":"string"},"range":{"type":"string"}}}`},
		{Name: "sheets.append", Risk: Reversible, Signature: "sheets.append({sheet, range, values})", Returns: "{ok, range, rows}; adds rows after the table in range (e.g. Gastos!A:D); values is a list of rows, each a list of cells; can be undone",
			Schema: `{"type":"object","required":["sheet","values"],"properties":{"sheet":{"type":"string"},"range":{"type":"string"},"values":{"type":"array","items":{"type":"array"}}}}`},
		{Name: "sheets.clear", Risk: Irreversible, Signature: "sheets.clear({sheet, range})", Returns: "{ok}; empties the cells of a range; used to undo sheets.append",
			Schema: `{"type":"object","required":["sheet","range"],"properties":{"sheet":{"type":"string"},"range":{"type":"string"}}}`},
		{Name: "spotify.now", Risk: Read, Signature: "spotify.now()", Returns: "{playing, title, by, uri, device}; what the owner's Spotify is playing"},
		{Name: "spotify.devices", Risk: Read, Signature: "spotify.devices()", Returns: "[{name, type, active, volume}] the owner's Spotify devices"},
		{Name: "spotify.play", Risk: Reversible, Signature: "spotify.play({query, kind, uri, device})", Returns: "{ok, playing}; plays the first result for query (kind: track, playlist, album, artist, show or episode; default track), or a spotify: uri, or resumes when neither; device is a device name; can be undone",
			Schema: `{"type":"object","properties":{"query":{"type":"string"},"kind":{"type":"string","enum":["track","playlist","album","artist","show","episode"]},"uri":{"type":"string"},"device":{"type":"string"}}}`},
		{Name: "spotify.pause", Risk: Reversible, Signature: "spotify.pause({device})", Returns: "{ok}; pauses the owner's Spotify; can be undone",
			Schema: `{"type":"object","properties":{"device":{"type":"string"}}}`},
		{Name: "spotify.volume", Risk: Reversible, Signature: "spotify.volume({percent, device})", Returns: "{ok}; sets the volume, 0 to 100",
			Schema: `{"type":"object","required":["percent"],"properties":{"percent":{"type":"integer","minimum":0,"maximum":100},"device":{"type":"string"}}}`},
		{Name: "web.read", Risk: Read, Signature: "web.read({url})", Returns: "{url, title, description, text, truncated, data: [JSON-LD objects, e.g. a Product with offers.price], links: [{text, url}]}; a web page as readable text; only hosts named in the manifest scope are reachable", Scoped: true, ScopeArg: "url",
			Schema: `{"type":"object","required":["url"],"properties":{"url":{"type":"string","description":"https URL of a web page"}}}`},
		{Name: "telegram.send", Risk: Notify, Signature: "telegram.send({text})", Returns: "{ok}; sends a message to the owner only"},
		{Name: "notify.send", Risk: Notify, Signature: "notify.send({text})", Returns: "{ok, delivered: [destination]}; sends to the destinations the owner chose for this routine (Telegram bots, WhatsApp, Slack, Discord, email), or to the owner's usual channel. Prefer this over telegram.send", Schema: `{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}`},
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
