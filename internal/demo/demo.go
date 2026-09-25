// Package demo runs Vigia without accounts or model costs: an in-memory
// mailbox and calendar, a scripted explorer and ready routines. It exists
// for trying the product, for browser tests and for screenshots.
package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/denerFernandes/vigia/internal/judge"
	"github.com/denerFernandes/vigia/internal/llm"
	"github.com/denerFernandes/vigia/internal/routine"
)

// Mailbox is an in-memory inbox that behaves like the mail connector.
type Mailbox struct {
	mu     sync.Mutex
	Now    func() time.Time
	emails []map[string]any
	Sent   []map[string]any
}

func NewMailbox(now func() time.Time) *Mailbox {
	t := now()
	at := func(h float64) string { return t.Add(-time.Duration(h * float64(time.Hour))).Format(time.RFC3339) }
	return &Mailbox{Now: now, emails: []map[string]any{
		{"id": "INBOX/41", "from": "ana@acme.com", "from_name": "Ana Souza", "subject": "Contrato Q4 precisa da sua assinatura hoje", "snippet": "O jurídico precisa da assinatura até as 17h. Segue o link do DocuSign.", "date": at(1), "labels": []any{"INBOX", "UNREAD"}, "unread": true, "replied": false},
		{"id": "INBOX/40", "from": "news@techweekly.com", "from_name": "Tech Weekly", "subject": "As 10 notícias de IA da semana", "snippet": "Nesta edição: modelos menores, agentes e mais.", "date": at(3), "labels": []any{"INBOX", "UNREAD"}, "unread": true, "replied": false, "can_unsubscribe": true},
		{"id": "INBOX/39", "from": "cobranca@enel.com.br", "from_name": "Enel", "subject": "Sua conta de luz de setembro", "snippet": "Total a pagar R$ 189,90 — vence em " + t.AddDate(0, 0, 3).Format("02/01/2006") + ".", "date": at(5), "labels": []any{"INBOX", "UNREAD"}, "unread": true, "replied": false},
		{"id": "INBOX/38", "from": "promo@megaloja.com", "from_name": "Mega Loja", "subject": "Só hoje: 70% OFF em tudo", "snippet": "Aproveite as ofertas imperdíveis.", "date": at(6), "labels": []any{"INBOX", "UNREAD"}, "unread": true, "replied": false},
		{"id": "INBOX/37", "from": "bruno@acme.com", "from_name": "Bruno Lima", "subject": "Revisão do deck para sexta", "snippet": "Consegue olhar os números do slide 7 até amanhã?", "date": at(20), "labels": []any{"INBOX", "UNREAD"}, "unread": true, "replied": false},
		{"id": "INBOX/36", "from": "digest@medium.com", "from_name": "Medium Daily Digest", "subject": "Histórias escolhidas para você", "snippet": "5 leituras de 4 minutos.", "date": at(26), "labels": []any{"INBOX", "UNREAD"}, "unread": true, "replied": false, "can_unsubscribe": true},
	}}
}

func (m *Mailbox) Capabilities() []string {
	return []string{"gmail.search", "gmail.archive", "gmail.label", "gmail.trash", "gmail.delete", "gmail.draft", "gmail.send", "gmail.unsubscribe"}
}

func (m *Mailbox) Call(_ context.Context, name, _ string, args any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, _ := args.(map[string]any)
	id, _ := a["id"].(string)
	switch name {
	case "gmail.search":
		all := make([]any, len(m.emails))
		for i, e := range m.emails {
			all[i] = e
		}
		norm := map[string]any{}
		b, _ := json.Marshal(args)
		json.Unmarshal(b, &norm)
		return routine.FilterResponse(name, norm, all, m.Now()), nil
	case "gmail.archive", "gmail.trash", "gmail.delete":
		for i, e := range m.emails {
			if e["id"] == id {
				m.emails = append(m.emails[:i], m.emails[i+1:]...)
				box := map[string]string{"gmail.archive": "Archive", "gmail.trash": "Trash"}[name]
				return map[string]any{"ok": true, "moved_to": box, "message_id": "<" + id + "@demo>"}, nil
			}
		}
		return nil, fmt.Errorf("message %s not found", id)
	case "gmail.label":
		return map[string]any{"ok": true, "label": a["label"], "message_id": "<" + id + "@demo>"}, nil
	case "gmail.draft":
		return map[string]any{"ok": true, "saved_in": "Drafts", "message_id": "<draft@demo>"}, nil
	case "gmail.unsubscribe":
		return map[string]any{"ok": true, "method": "one-click"}, nil
	case "gmail.send":
		m.Sent = append(m.Sent, a)
		return map[string]any{"ok": true, "message_id": "<sent@demo>"}, nil
	}
	return nil, fmt.Errorf("demo mailbox cannot %s", name)
}

// Restore and Remove make undo work in the demo.
func (m *Mailbox) Restore(context.Context, string, string) error { return nil }
func (m *Mailbox) Remove(context.Context, string, string) error  { return nil }

// Calendar returns a realistic day.
type Calendar struct{ Now func() time.Time }

func (c Calendar) Capabilities() []string { return []string{"calendar.events"} }
func (c Calendar) Call(_ context.Context, _, _ string, args any) (any, error) {
	t := c.Now()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	ev := func(h, m, dur int, title, cal string, who ...string) map[string]any {
		s := day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
		if who == nil {
			who = []string{}
		}
		return map[string]any{"id": title, "title": title, "start": s.Format(time.RFC3339), "end": s.Add(time.Duration(dur) * time.Minute).Format(time.RFC3339), "attendees": who, "calendar": cal, "all_day": false}
	}
	all := []any{
		ev(9, 30, 15, "Standup do time", "Trabalho", "bruno@acme.com"),
		ev(14, 0, 60, "Revisão de contrato com Jurídico", "Trabalho", "ana@acme.com"),
		ev(19, 30, 90, "Jantar com a Carol", "Pessoal"),
	}
	norm := map[string]any{}
	b, _ := json.Marshal(args)
	json.Unmarshal(b, &norm)
	return routine.FilterResponse("calendar.events", norm, all, t), nil
}

// Judge decides with simple keywords, standing in for a model.
type Judge struct{}

func (Judge) Ask(_ context.Context, question string, item any) (judge.Answer, error) {
	b, _ := json.Marshal(item)
	text := strings.ToLower(string(b))
	q := strings.ToLower(question)
	promo := strings.Contains(text, "off") || strings.Contains(text, "digest") || strings.Contains(text, "news@") || strings.Contains(text, "promo")
	switch {
	case strings.Contains(q, "newsletter") || strings.Contains(q, "promo"):
		if promo {
			return judge.Answer{P: 0.94, Backend: "demo"}, nil
		}
		return judge.Answer{P: 0.06, Backend: "demo"}, nil
	default:
		if promo {
			return judge.Answer{P: 0.05, Backend: "demo"}, nil
		}
		return judge.Answer{P: 0.9, Backend: "demo"}, nil
	}
}

// Notices collects what would go to Telegram.
type Telegram struct {
	mu   sync.Mutex
	Sent []string
}

func (t *Telegram) Capabilities() []string { return []string{"telegram.send"} }
func (t *Telegram) Call(_ context.Context, _, _ string, args any) (any, error) {
	a, _ := args.(map[string]any)
	t.mu.Lock()
	t.Sent = append(t.Sent, fmt.Sprint(a["text"]))
	t.mu.Unlock()
	return map[string]any{"ok": true}, nil
}

// Agent explores like a model would, choosing a script from the request.
type Agent struct{ Delay time.Duration }

type step struct {
	tool string
	args map[string]any
}

func (a Agent) Run(ctx context.Context, r llm.AgentRequest) (llm.Response, error) {
	kind := Kind(r.Prompt)
	var steps []step
	var summary string
	switch kind {
	case "triage":
		steps = []step{
			{"gmail_search", map[string]any{"query": "is:unread", "days": 7}},
			{"decide", map[string]any{"judgment": "newsletter", "question": "Este e-mail é uma newsletter ou promoção?", "item": "INBOX/40", "yes": true, "confidence": 0.95}},
			{"decide", map[string]any{"judgment": "newsletter", "question": "Este e-mail é uma newsletter ou promoção?", "item": "INBOX/38", "yes": true, "confidence": 0.97}},
			{"decide", map[string]any{"judgment": "newsletter", "question": "Este e-mail é uma newsletter ou promoção?", "item": "INBOX/36", "yes": true, "confidence": 0.93}},
			{"decide", map[string]any{"judgment": "newsletter", "question": "Este e-mail é uma newsletter ou promoção?", "item": "INBOX/41", "yes": false, "confidence": 0.95}},
			{"gmail_archive", map[string]any{"id": "INBOX/40"}},
			{"gmail_archive", map[string]any{"id": "INBOX/38"}},
			{"gmail_archive", map[string]any{"id": "INBOX/36"}},
			{"telegram_send", map[string]any{"text": "🧹 Arquivei 3 newsletters e promoções: As 10 notícias de IA da semana, Só hoje: 70% OFF em tudo, Histórias escolhidas para você."}},
		}
		summary = "Encontrei 6 e-mails não lidos. Três eram newsletters ou promoções: arquivei (simulado) e te avisei no Telegram. Todo dia às 18h a rotina faz o mesmo."
	case "bills":
		steps = []step{
			{"gmail_search", map[string]any{"query": "fatura OR boleto OR conta OR vencimento", "days": 30}},
			{"telegram_send", map[string]any{"text": "💸 Contas nos próximos 7 dias:\n• Enel — R$ 189,90 — Sua conta de luz de setembro"}},
		}
		summary = "Achei uma conta vencendo nos próximos 7 dias (Enel, R$ 189,90) e te avisei. A rotina olha seus e-mails toda manhã às 8h."
	default:
		steps = []step{
			{"calendar_events", map[string]any{"from": "today", "to": "tomorrow"}},
			{"gmail_search", map[string]any{"query": "is:unread", "days": 1}},
			{"decide", map[string]any{"judgment": "important", "question": "Este e-mail é importante para hoje?", "item": "INBOX/41", "yes": true, "confidence": 0.95}},
			{"decide", map[string]any{"judgment": "important", "question": "Este e-mail é importante para hoje?", "item": "INBOX/39", "yes": true, "confidence": 0.8}},
			{"decide", map[string]any{"judgment": "important", "question": "Este e-mail é importante para hoje?", "item": "INBOX/40", "yes": false, "confidence": 0.9}},
			{"decide", map[string]any{"judgment": "important", "question": "Este e-mail é importante para hoje?", "item": "INBOX/38", "yes": false, "confidence": 0.95}},
			{"telegram_send", map[string]any{"text": "☀️ Bom dia! Hoje:\n• 09:30 Standup do time\n• 14:00 Revisão de contrato com Jurídico\n• 19:30 Jantar com a Carol\n\n📬 Importantes:\n• Ana Souza: Contrato Q4 precisa da sua assinatura hoje\n• Enel: Sua conta de luz de setembro"}},
		}
		summary = "Li sua agenda (3 compromissos) e os e-mails não lidos: dois importantes, dois promocionais ignorados. Mandei o resumo no Telegram. Todo dia às 7h a rotina faz isso sozinha."
	}
	for i, s := range steps {
		if a.Delay > 0 {
			select {
			case <-time.After(a.Delay):
			case <-ctx.Done():
				return llm.Response{}, ctx.Err()
			}
		}
		args := s.args
		if s.tool == "calendar_events" {
			now := time.Now()
			d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			args = map[string]any{"from": d.Format(time.RFC3339), "to": d.AddDate(0, 0, 1).Format(time.RFC3339)}
		}
		if err := call(r.MCPURL, i, s.tool, args); err != nil {
			return llm.Response{}, err
		}
	}
	cost := map[string]float64{"brief": 0.27, "triage": 0.31, "bills": 0.18}[kind]
	return llm.Response{Text: summary, CostUSD: cost}, nil
}

// Kind picks the demo script for a request.
func Kind(request string) string {
	r := strings.ToLower(request)
	switch {
	case strings.Contains(r, "newsletter") || strings.Contains(r, "promo") || strings.Contains(r, "arquiv"):
		return "triage"
	case strings.Contains(r, "conta") || strings.Contains(r, "fatura") || strings.Contains(r, "boleto") || strings.Contains(r, "venc"):
		return "bills"
	}
	return "brief"
}

func call(url string, id int, tool string, args any) error {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Compiler returns the routine for each demo script, and turns rule
// sentences into rules with a few keywords.
type Compiler struct{}

func (Compiler) Generate(_ context.Context, r llm.Request) (llm.Response, error) {
	if i := strings.Index(r.Prompt, "The owner wrote: "); i >= 0 {
		return llm.Response{Structured: demoRule(r.Prompt[i+len("The owner wrote: "):]), CostUSD: 0.002}, nil
	}
	kind := "brief"
	if i := strings.Index(r.Prompt, "The user asked: "); i >= 0 {
		kind = Kind(r.Prompt[i : i+min(400, len(r.Prompt)-i)])
	}
	return llm.Response{Structured: json.RawMessage(routines[kind]), CostUSD: 0.06}, nil
}

func demoRule(text string) json.RawMessage {
	t := strings.ToLower(text)
	then := "ask"
	if strings.Contains(t, "nunca") && !strings.Contains(t, "sem me perguntar") {
		then = "block"
	}
	var caps []string
	switch {
	case strings.Contains(t, "apag") || strings.Contains(t, "lixeira"):
		caps = []string{"gmail.delete", "gmail.trash"}
	case strings.Contains(t, "envi") || strings.Contains(t, "mand"):
		caps = []string{"gmail.send"}
	case strings.Contains(t, "arquiv"):
		caps = []string{"gmail.archive"}
	}
	b, _ := json.Marshal(map[string]any{"summary": text, "then": then, "when": map[string]any{"capabilities": caps}})
	return b
}

var routines = map[string]string{
	"brief": `{"name":"Resumo matinal","description":"Agenda de hoje e e-mails importantes no Telegram, todo dia às 7h.","manifest":{"schedule":"0 7 * * *","capabilities":["calendar.events","gmail.search","notify.send"],"judgments":{"important":"Este e-mail é importante para hoje?"},"locale":"pt-BR","params":[{"name":"destinos","label":"Onde avisar","type":"destinations","default":[]}]},
"code":"async function run() {\n  const today = dates.today();\n  const events = await calendar.events({from: today, to: dates.startOfDay(today, 1)});\n  const mails = await gmail.search({query: \"is:unread\", days: 1});\n  const important = [];\n  for (const m of mails) {\n    if ((await judge.important(m)).p >= 0.5) important.push(m);\n  }\n  let text = \"☀️ Bom dia! Hoje:\\n\";\n  text += events.length ? events.map(e => \"• \" + dates.format(e.start, \"HH:mm\") + \" \" + e.title).join(\"\\n\") : \"• Agenda livre\";\n  if (important.length) {\n    text += \"\\n\\n📬 Importantes:\\n\" + important.map(m => \"• \" + m.from_name + \": \" + m.subject).join(\"\\n\");\n  }\n  await notify.send({text});\n}",
"tests":[{"name":"dia sem e-mails importantes","now":"2026-10-02T07:00:00-03:00","responses":[{"capability":"calendar.events","result":[{"title":"Dentista","start":"2026-10-02T10:00:00-03:00","end":"2026-10-02T11:00:00-03:00"}]},{"capability":"gmail.search","result":[{"id":"x1","from_name":"Loja","subject":"Promoção relâmpago"}]}],"judgments":{"important":{"x1":0.05}},"expect":[{"capability":"notify.send","count":1,"contains":["Dentista"],"not_contains":["Promoção relâmpago"]}]}]}`,
	"triage": `{"name":"Triagem de newsletters","description":"Arquiva newsletters e promoções não lidas às 18h e conta quantas foram.","manifest":{"schedule":"0 18 * * *","capabilities":["gmail.search","gmail.archive","notify.send"],"judgments":{"newsletter":"Este e-mail é uma newsletter ou promoção?"},"locale":"pt-BR","params":[{"name":"destinos","label":"Onde avisar","type":"destinations","default":[]}]},
"code":"async function run() {\n  const mails = await gmail.search({query: \"is:unread\", days: 7});\n  const archived = [];\n  for (const m of mails) {\n    if ((await judge.newsletter(m)).p >= 0.5) {\n      await gmail.archive({id: m.id});\n      archived.push(m.subject);\n    }\n  }\n  if (archived.length) {\n    await notify.send({text: \"🧹 Arquivei \" + archived.length + \" newsletters e promoções: \" + archived.join(\", \") + \".\"});\n  }\n}",
"tests":[{"name":"nada para arquivar","now":"2026-10-02T18:00:00-03:00","responses":[{"capability":"gmail.search","result":[{"id":"y1","subject":"Reunião amanhã"}]}],"judgments":{"newsletter":{"y1":0.04}},"expect":[{"capability":"notify.send","count":0},{"capability":"gmail.archive","count":0}]}]}`,
	"bills": `{"name":"Contas a vencer","description":"Boletos e faturas que vencem nos próximos dias, com valor.","manifest":{"schedule":"0 8 * * *","capabilities":["gmail.search","notify.send"],"locale":"pt-BR","params":[{"name":"dias","label":"Avisar contas que vencem nos próximos (dias)","type":"number","default":7},{"name":"destinos","label":"Onde avisar","type":"destinations","default":[]}]},
"code":"async function run() {\n  const mails = await gmail.search({query: \"fatura OR boleto OR conta OR vencimento\", days: 30});\n  const soon = [];\n  for (const m of mails) {\n    const due = dates.parse(m.snippet);\n    const amount = money.find(m.snippet);\n    if (!due || !amount) continue;\n    const days = dates.diffDays(now(), due);\n    if (days >= 0 && days <= params.dias) soon.push(\"• \" + m.from_name + \" — \" + amount + \" — \" + m.subject);\n  }\n  if (soon.length) await notify.send({text: \"💸 Contas nos próximos \" + params.dias + \" dias:\\n\" + soon.join(\"\\n\")});\n}",
"tests":[]}`,
}

// Now is the demo clock.
func Now() time.Time { return time.Now() }
