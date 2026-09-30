package people

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbine-dev/pimpo/internal/event"
)

func dir(t *testing.T) *Directory {
	ev, err := event.Open(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	return &Directory{Events: ev, OwnerChat: func(context.Context) (int64, error) { return 1, nil }}
}

func TestInviteAndPair(t *testing.T) {
	ctx := context.Background()
	d := dir(t)
	ana, err := d.Add(ctx, "Ana", Member, "")
	if err != nil || ana.ID != "ana" || ana.Invite == "" || ana.Responsible != OwnerID {
		t.Fatalf("%+v %v", ana, err)
	}
	ana2, _ := d.Add(ctx, "Ana", Guest, "ana")
	if ana2.ID != "ana2" {
		t.Fatalf("duplicate name got id %s", ana2.ID)
	}
	if _, err := d.Pair(ctx, "wrong", 5); err == nil {
		t.Fatal("paired with a wrong code")
	}
	if _, err := d.Pair(ctx, ana.Invite, 1); err == nil {
		t.Fatal("paired the owner's chat to someone else")
	}
	p, err := d.Pair(ctx, ana.Invite, 5)
	if err != nil || p.Chat != 5 {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := d.Pair(ctx, ana.Invite, 6); err == nil {
		t.Fatal("an invite worked twice")
	}
	if who, ok := d.ByChat(ctx, 5); !ok || who.ID != "ana" {
		t.Fatalf("by chat %+v", who)
	}
	if who, _ := d.ByChat(ctx, 1); who.ID != OwnerID {
		t.Fatalf("owner chat %+v", who)
	}
	if r := d.Responsible(ctx, "ana2"); r.ID != "ana" {
		t.Fatalf("responsible for ana2: %s", r.ID)
	}
	d.Update(ctx, "ana", Guest, "")
	if r := d.Responsible(ctx, "ana2"); r.ID != OwnerID {
		t.Fatalf("a guest answered for someone: %s", r.ID)
	}
	if d.Role(ctx, "ghost") != Guest {
		t.Fatal("unknown people must get the smallest role")
	}
	d.Remove(ctx, "ana")
	if _, ok := d.ByChat(ctx, 5); ok {
		t.Fatal("a removed person can still talk")
	}
}

func TestContextAndVisibility(t *testing.T) {
	ctx := context.Background()
	if From(ctx) != OwnerID || From(With(ctx, "ana")) != "ana" || From(With(ctx, "")) != OwnerID {
		t.Fatal("context person")
	}
	if !Visible("", OwnerID) || !Visible(Household, "ana") || Visible("", "ana") || Visible("bia", "ana") {
		t.Fatal("visibility")
	}
}

// Invites are long, work once and run out; the People page then gets a
// fresh one.
func TestInvitesExpireAndRenew(t *testing.T) {
	ctx := context.Background()
	d := dir(t)
	ana, _ := d.Add(ctx, "Ana", Member, "")
	bia, _ := d.Add(ctx, "Bia", Member, "")
	if len(ana.Invite) < 10 || ana.InviteUntil.Before(time.Now().Add(InviteLife-time.Minute)) {
		t.Fatalf("invite %q until %v", ana.Invite, ana.InviteUntil)
	}
	list, _ := d.load(ctx)
	for i := range list {
		list[i].InviteUntil = time.Now().Add(-time.Minute)
	}
	d.save(ctx, list)
	if _, err := d.Pair(ctx, ana.Invite, 5); err == nil {
		t.Fatal("paired with an expired invite")
	}
	if _, err := d.PairWhatsApp(ctx, bia.Invite, "5511"); err == nil {
		t.Fatal("paired WhatsApp with an expired invite")
	}
	if err := d.RenewInvites(ctx); err != nil {
		t.Fatal(err)
	}
	fresh, _ := d.Get(ctx, "ana")
	if fresh.Invite == ana.Invite || !time.Now().Before(fresh.InviteUntil) {
		t.Fatalf("not renewed: %+v", fresh)
	}
	if _, err := d.Pair(ctx, " "+strings.ToLower(fresh.Invite)+" ", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PairWhatsApp(ctx, fresh.Invite, "5511"); err == nil {
		t.Fatal("an invite worked twice")
	}
}
