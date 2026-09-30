// Package people is the household: who may talk to Pimpo, in what role,
// and who answers for whom. The owner is implicit and always exists.
package people

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/turbine-dev/pimpo/internal/i18n"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
)

type Role string

const (
	Owner  Role = "owner"
	Member Role = "member"
	// Guest can ask, but everything a guest's request would change waits
	// for the responsible person.
	Guest Role = "guest"
)

// OwnerID names the owner. Facts, credentials and runs from before people
// existed belong to the owner, so the empty id means the owner too.
const OwnerID = "owner"

// Household marks memory everyone in the house may read.
const Household = "casa"

type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role Role   `json:"role"`
	// Chat is the person's Telegram chat, once paired.
	Chat int64 `json:"chat,omitempty"`
	// WhatsApp is the person's WhatsApp id (phone digits), once paired.
	WhatsApp string `json:"whatsapp,omitempty"`
	// Responsible answers approvals for this person's requests.
	Responsible string `json:"responsible,omitempty"`
	// Invite is the code they send to the bot to pair; cleared on use,
	// and it works until InviteUntil.
	Invite      string    `json:"invite,omitempty"`
	InviteUntil time.Time `json:"invite_until,omitzero"`
	Created     time.Time `json:"created"`
}

const key = "people"

var ErrUnknown = errors.New("no such person")

type Directory struct {
	Events *event.Store
	// OwnerChat is the owner's Telegram chat; the owner is not stored here.
	OwnerChat func(ctx context.Context) (int64, error)
	// OwnerWhatsApp is the owner's WhatsApp id.
	OwnerWhatsApp func(ctx context.Context) string

	mu sync.Mutex
}

func Norm(id string) string {
	if id == "" {
		return OwnerID
	}
	return id
}

func (d *Directory) load(ctx context.Context) ([]Person, error) {
	raw, err := d.Events.Get(ctx, key)
	if err != nil || raw == "" {
		return nil, err
	}
	var list []Person
	err = json.Unmarshal([]byte(raw), &list)
	return list, err
}

func (d *Directory) save(ctx context.Context, list []Person) error {
	b, _ := json.Marshal(list)
	return d.Events.Put(ctx, key, string(b))
}

// List returns everyone, the owner first.
func (d *Directory) List(ctx context.Context) ([]Person, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return nil, err
	}
	owner := Person{ID: OwnerID, Name: i18n.T(ctx, "people.you"), Role: Owner}
	if d.OwnerChat != nil {
		owner.Chat, _ = d.OwnerChat(ctx)
	}
	if d.OwnerWhatsApp != nil {
		owner.WhatsApp = d.OwnerWhatsApp(ctx)
	}
	return append([]Person{owner}, list...), nil
}

func (d *Directory) Get(ctx context.Context, id string) (Person, error) {
	all, err := d.List(ctx)
	if err != nil {
		return Person{}, err
	}
	id = Norm(id)
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return Person{}, ErrUnknown
}

// ByChat finds who a Telegram chat belongs to.
func (d *Directory) ByChat(ctx context.Context, chat int64) (Person, bool) {
	all, _ := d.List(ctx)
	for _, p := range all {
		if chat != 0 && p.Chat == chat {
			return p, true
		}
	}
	return Person{}, false
}

// ByWhatsApp finds who a WhatsApp id belongs to.
func (d *Directory) ByWhatsApp(ctx context.Context, id string) (Person, bool) {
	all, _ := d.List(ctx)
	for _, p := range all {
		if id != "" && p.WhatsApp == id {
			return p, true
		}
	}
	return Person{}, false
}

// PairWhatsApp links the WhatsApp id that sent an invite code to its person.
func (d *Directory) PairWhatsApp(ctx context.Context, invite, id string) (Person, error) {
	if other, taken := d.ByWhatsApp(ctx, id); taken {
		return other, errors.New("this number already belongs to someone")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return Person{}, err
	}
	for i, p := range list {
		if inviteMatches(p, invite, time.Now()) {
			list[i].WhatsApp, list[i].Invite = id, ""
			return list[i], d.save(ctx, list)
		}
	}
	return Person{}, ErrUnknown
}

// Role of a person; unknown ids get the most restricted role.
func (d *Directory) Role(ctx context.Context, id string) Role {
	p, err := d.Get(ctx, id)
	if err != nil {
		return Guest
	}
	return p.Role
}

// Responsible returns who approves for this person: the responsible the
// owner gave them (a parent for a child), else a member answers for
// themselves and a guest's requests go to the owner. The owner answers
// for themselves.
func (d *Directory) Responsible(ctx context.Context, id string) Person {
	p, err := d.Get(ctx, id)
	if err == nil && p.Responsible != "" && p.Responsible != p.ID {
		if r, err := d.Get(ctx, p.Responsible); err == nil && r.Role != Guest {
			return r
		}
	}
	if err == nil && p.Role == Member {
		return p
	}
	owner, _ := d.Get(ctx, OwnerID)
	return owner
}

func slug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "pessoa"
	}
	return b.String()
}

// Add invites someone. They pair by sending "/start <invite>" to the bot.
func (d *Directory) Add(ctx context.Context, name string, role Role, responsible string) (Person, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Person{}, errors.New("give the person a name")
	}
	if role != Member && role != Guest {
		return Person{}, errors.New("role must be member or guest")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return Person{}, err
	}
	id := slug(name)
	for n := 2; ; n++ {
		taken := id == OwnerID || id == Household
		for _, p := range list {
			taken = taken || p.ID == id
		}
		if !taken {
			break
		}
		id = slug(name) + strconv.Itoa(n)
	}
	p := Person{ID: id, Name: name, Role: role, Responsible: Norm(responsible), Invite: NewCode(), InviteUntil: time.Now().Add(InviteLife), Created: time.Now()}
	list = append(list, p)
	return p, d.save(ctx, list)
}

// Pair links the chat that sent an invite code to its person.
func (d *Directory) Pair(ctx context.Context, invite string, chat int64) (Person, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return Person{}, err
	}
	if d.OwnerChat != nil {
		if owner, _ := d.OwnerChat(ctx); owner != 0 && owner == chat {
			return Person{}, errors.New("this chat already belongs to someone")
		}
	}
	for i, p := range list {
		if inviteMatches(p, invite, time.Now()) {
			for _, q := range list {
				if q.Chat == chat {
					return Person{}, errors.New("this chat already belongs to someone")
				}
			}
			list[i].Chat, list[i].Invite = chat, ""
			return list[i], d.save(ctx, list)
		}
	}
	return Person{}, ErrUnknown
}

// RenewInvites gives everyone still unpaired whose invite ran out a new
// one, so the People page never shows a code that no longer works.
func (d *Directory) RenewInvites(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return err
	}
	changed := false
	for i, p := range list {
		if p.Chat == 0 && p.WhatsApp == "" && !time.Now().Before(p.InviteUntil) {
			list[i].Invite, list[i].InviteUntil = NewCode(), time.Now().Add(InviteLife)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return d.save(ctx, list)
}

// Update changes a person's role or responsible.
func (d *Directory) Update(ctx context.Context, id string, role Role, responsible string) (Person, error) {
	if role != Member && role != Guest {
		return Person{}, errors.New("role must be member or guest")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return Person{}, err
	}
	for i, p := range list {
		if p.ID == id {
			list[i].Role, list[i].Responsible = role, Norm(responsible)
			return list[i], d.save(ctx, list)
		}
	}
	return Person{}, ErrUnknown
}

// Remove forgets a person; their chat can no longer talk to Pimpo.
func (d *Directory) Remove(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	list, err := d.load(ctx)
	if err != nil {
		return err
	}
	for i, p := range list {
		if p.ID == id {
			return d.save(ctx, append(list[:i], list[i+1:]...))
		}
	}
	return ErrUnknown
}

type ctxKey struct{}

// With marks ctx as acting for a person.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, Norm(id))
}

// From returns who ctx acts for; the owner when nobody was set.
func From(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey{}).(string); ok {
		return id
	}
	return OwnerID
}

// Visible reports whether a fact kept for owner may be read by reader.
func Visible(owner, reader string) bool {
	return Norm(owner) == Norm(reader) || owner == Household
}
