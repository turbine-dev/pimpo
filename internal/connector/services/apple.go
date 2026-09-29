package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/turbine-dev/pimpo/internal/capability"
)

// Apple's own apps on this Mac: Reminders (which sync to the iPhone and
// the Watch), Notes and Calendar, through their scripting interface.
// Nothing leaves the computer except what iCloud syncs as usual; macOS
// asks the owner once to let Pimpo use each app.
func init() {
	if runtime.GOOS != "darwin" {
		return
	}
	register(Kind{
		ID: "apple", Title: "Lembretes, Notas e Calendário da Apple", Description: "Os apps da Apple neste Mac: lembretes que tocam no iPhone e no relógio, notas e a agenda.",
		Help: "Informe a pasta das Notas onde o Pimpo escreve (ela é criada se não existir) e, se quiser, a lista de Lembretes padrão. Na primeira vez, o macOS pede para deixar o Pimpo usar cada app: permita em Ajustes do Sistema › Privacidade e Segurança › Automação.",
		Fields: []Field{
			{Name: "notes_folder", Label: "Pasta das Notas", Placeholder: "Pimpo"},
			{Name: "reminders_list", Label: "Lista de Lembretes padrão", Placeholder: "Lembretes", Optional: true},
		},
		Specs: []capability.Spec{
			{Name: "apple.reminders.list", Risk: capability.Read, Signature: "apple.reminders.list({list, max})", Returns: "[{id, title, due, list, notes}] open reminders, soonest due first; list filters by list name",
				Schema: obj(`"list":{"type":"string"},"max":{"type":"integer"}`)},
			{Name: "apple.reminders.add", Risk: capability.Reversible, Signature: "apple.reminders.add({title, due, list, notes})", Returns: "{id}; a reminder in the Reminders app, which alerts on the iPhone and the Watch at due (ISO 8601); list defaults to the owner's; can be undone",
				Schema: obj(`"title":{"type":"string"},"due":{"type":"string","description":"ISO 8601 with offset"},"list":{"type":"string"},"notes":{"type":"string"}`, "title")},
			{Name: "apple.reminders.complete", Risk: capability.Reversible, Signature: "apple.reminders.complete({id})", Returns: "{ok}; marks a reminder done; can be undone",
				Schema: obj(`"id":{"type":"string"}`, "id")},
			{Name: "apple.notes.search", Risk: capability.Read, Signature: "apple.notes.search({query, max})", Returns: "[{id, title, folder, modified, text}] notes whose title or text contain query; text is the start of the note",
				Schema: obj(`"query":{"type":"string"},"max":{"type":"integer"}`, "query")},
			{Name: "apple.notes.append", Risk: capability.Reversible, Signature: "apple.notes.append({note, text})", Returns: "{id}; adds a paragraph to the note with that title in Pimpo's folder, creating it if missing; can be undone",
				Schema: obj(`"note":{"type":"string","description":"the note's title"},"text":{"type":"string"}`, "note", "text")},
			{Name: "apple.calendar.events", Risk: capability.Read, Signature: "apple.calendar.events({from, to})", Returns: "[{id, title, start, end, location, calendar, notes}] events of the Mac's Calendar app between from and to (ISO 8601)",
				Schema: obj(`"from":{"type":"string"},"to":{"type":"string"}`, "from", "to")},
			// Undo steps, called by Pimpo itself.
			{Name: "apple.reminders.delete", Risk: capability.Irreversible, Signature: "apple.reminders.delete({id})", Returns: "{ok}; deletes a reminder for good; used to undo apple.reminders.add", Schema: obj(`"id":{"type":"string"}`, "id")},
			{Name: "apple.reminders.reopen", Risk: capability.Reversible, Signature: "apple.reminders.reopen({id})", Returns: "{ok}; marks a done reminder open again", Schema: obj(`"id":{"type":"string"}`, "id")},
			{Name: "apple.notes.restore", Risk: capability.Irreversible, Signature: "apple.notes.restore({id, body})", Returns: "{ok}; puts a note's earlier body back (an empty body deletes the note); used to undo apple.notes.append", Schema: obj(`"id":{"type":"string"},"body":{"type":"string"}`, "id", "body")},
		},
		Call: callApple,
		Probe: func(ctx context.Context, cfg Config) error {
			if _, err := callApple(ctx, cfg, "apple.reminders.list", "", map[string]any{"max": 1}); err != nil {
				return err
			}
			_, err := callApple(ctx, cfg, "apple.notes.search", "", map[string]any{"query": "pimpo-probe", "max": 1})
			return err
		},
	})
}

// appleScript is the JavaScript for Automation that does every call. The
// arguments arrive as one JSON string, never inside the code.
const appleScript = `
function iso(d) { return d ? d.toISOString() : null }
function run(argv) {
  const a = JSON.parse(argv[0])
  if (a.op.startsWith('reminders.')) {
    const R = Application('Reminders')
    const list = (name) => name ? R.lists.byName(name) : R.defaultList()
    switch (a.op) {
      case 'reminders.list': {
        const lists = a.list ? [R.lists.byName(a.list)] : R.lists()
        let out = []
        for (const l of lists) {
          const rs = l.reminders.whose({completed: false})
          const ids = rs.id(), names = rs.name(), dues = rs.dueDate(), bodies = rs.body()
          for (let i = 0; i < ids.length; i++) out.push({id: ids[i], title: names[i], due: iso(dues[i]), list: l.name(), notes: bodies[i] || ''})
        }
        out.sort((x, y) => (x.due || '9') < (y.due || '9') ? -1 : 1)
        return JSON.stringify(out.slice(0, a.max || 100))
      }
      case 'reminders.add': {
        const props = {name: a.title}
        if (a.due) props.dueDate = new Date(a.due)
        if (a.notes) props.body = a.notes
        const r = R.Reminder(props)
        list(a.list).reminders.push(r)
        return JSON.stringify({id: r.id()})
      }
      case 'reminders.complete': R.reminders.byId(a.id).completed = true; return '{"ok":true}'
      case 'reminders.reopen': R.reminders.byId(a.id).completed = false; return '{"ok":true}'
      case 'reminders.delete': R.delete(R.reminders.byId(a.id)); return '{"ok":true}'
    }
  }
  if (a.op.startsWith('notes.')) {
    const N = Application('Notes')
    switch (a.op) {
      case 'notes.search': {
        const q = a.query
        const found = N.notes.whose({_or: [{name: {_contains: q}}, {plaintext: {_contains: q}}]})
        const ids = found.id(), names = found.name(), mods = found.modificationDate(), texts = found.plaintext()
        const out = []
        for (let i = 0; i < ids.length && out.length < (a.max || 20); i++) {
          let folder = ''
          try { folder = N.notes.byId(ids[i]).container().name() } catch (e) {}
          out.push({id: ids[i], title: names[i], folder: folder, modified: iso(mods[i]), text: (texts[i] || '').slice(0, 2000)})
        }
        return JSON.stringify(out)
      }
      case 'notes.append': {
        let folder = N.folders.whose({name: a.folder})
        if (folder.length === 0) { N.folders.push(N.Folder({name: a.folder})); folder = N.folders.whose({name: a.folder}) }
        const f = folder[0]
        const existing = f.notes.whose({name: a.note})
        const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br>')
        if (existing.length === 0) {
          const n = N.Note({name: a.note, body: '<div><h1>' + esc(a.note) + '</h1></div><div>' + esc(a.text) + '</div>'})
          f.notes.push(n)
          return JSON.stringify({id: n.id(), created: true})
        }
        const n = existing[0]
        const before = n.body()
        n.body = before + '<div>' + esc(a.text) + '</div>'
        return JSON.stringify({id: n.id(), before: before})
      }
      case 'notes.restore': {
        const n = N.notes.byId(a.id)
        if (a.body === '') { N.delete(n) } else { n.body = a.body }
        return '{"ok":true}'
      }
    }
  }
  if (a.op === 'calendar.events') {
    const C = Application('Calendar')
    const from = new Date(a.from), to = new Date(a.to)
    const out = []
    for (const c of C.calendars()) {
      const evs = c.events.whose({_and: [{startDate: {_lessThan: to}}, {endDate: {_greaterThan: from}}]})
      const ids = evs.uid(), titles = evs.summary(), starts = evs.startDate(), ends = evs.endDate(), locs = evs.location(), notes = evs.description()
      for (let i = 0; i < ids.length; i++) out.push({id: ids[i], title: titles[i], start: iso(starts[i]), end: iso(ends[i]), location: locs[i] || '', calendar: c.name(), notes: (notes[i] || '').slice(0, 1000)})
    }
    out.sort((x, y) => x.start < y.start ? -1 : 1)
    return JSON.stringify(out.slice(0, 200))
  }
  throw new Error('unknown operation ' + a.op)
}`

// osascript runs the script; tests replace it.
var osascript = func(ctx context.Context, arg string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", appleScript, arg)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		switch {
		case strings.Contains(msg, "-1743") || strings.Contains(msg, "Not authorized") || strings.Contains(msg, "not allowed"):
			return nil, errors.New("macOS did not let Pimpo use this app; allow it in System Settings › Privacy & Security › Automation")
		case strings.Contains(msg, "-1728") || strings.Contains(msg, "Can't get"):
			return nil, errors.New("not found: " + lastPart(msg))
		}
		return nil, fmt.Errorf("the Apple app refused: %s", lastPart(msg))
	}
	return out, nil
}

func lastPart(s string) string {
	if i := strings.LastIndex(s, ": "); i >= 0 && i+2 < len(s) {
		return strings.TrimSpace(s[i+2:])
	}
	return s
}

func callApple(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	m, _ := args.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	in := map[string]any{"op": strings.TrimPrefix(name, "apple.")}
	for k, v := range m {
		in[k] = v
	}
	str := func(k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
	switch name {
	case "apple.reminders.add":
		if str("title") == "" {
			return nil, errors.New("a reminder needs a title")
		}
		if d := str("due"); d != "" {
			if _, err := time.Parse(time.RFC3339, d); err != nil {
				return nil, errors.New("due must be ISO 8601 with its offset, e.g. 2026-09-29T09:00:00-03:00")
			}
		}
		if str("list") == "" {
			if l, _ := cfg(ctx, "reminders_list"); strings.TrimSpace(l) != "" {
				in["list"] = strings.TrimSpace(l)
			}
		}
	case "apple.reminders.complete", "apple.reminders.delete", "apple.reminders.reopen", "apple.notes.restore":
		if str("id") == "" {
			return nil, errors.New("which one? id is missing")
		}
	case "apple.notes.search":
		if str("query") == "" {
			return nil, errors.New("apple.notes.search needs a query")
		}
	case "apple.notes.append":
		if str("note") == "" || str("text") == "" {
			return nil, errors.New("apple.notes.append needs a note title and a text")
		}
		folder, _ := cfg(ctx, "notes_folder")
		in["folder"] = firstNonEmptyStr(strings.TrimSpace(folder), "Pimpo")
	case "apple.calendar.events":
		for _, k := range []string{"from", "to"} {
			if _, err := time.Parse(time.RFC3339, str(k)); err != nil {
				return nil, fmt.Errorf("%s must be ISO 8601 with its offset", k)
			}
		}
	case "apple.reminders.list":
	default:
		return nil, fmt.Errorf("unknown capability %s", name)
	}
	raw, _ := json.Marshal(in)
	out, err := osascript(ctx, string(raw))
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(bytes.TrimSpace(out), &v); err != nil {
		return nil, fmt.Errorf("unreadable answer from the Apple app: %s", strings.TrimSpace(string(out)))
	}
	// Changes say how to undo themselves.
	if r, ok := v.(map[string]any); ok {
		switch name {
		case "apple.reminders.add":
			r["undo"] = map[string]any{"capability": "apple.reminders.delete", "args": map[string]any{"id": r["id"]}}
		case "apple.reminders.complete":
			r["undo"] = map[string]any{"capability": "apple.reminders.reopen", "args": map[string]any{"id": str("id")}}
		case "apple.notes.append":
			before, _ := r["before"].(string)
			r["undo"] = map[string]any{"capability": "apple.notes.restore", "args": map[string]any{"id": r["id"], "body": before}}
			delete(r, "before")
		}
	}
	return v, nil
}

func firstNonEmptyStr(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
