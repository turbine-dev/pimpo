package company

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// shop is a small company: Ana is its person, Clara answers customers and
// reports to Bia, the manager, who reports to Ana.
func shop(t *testing.T, s *Store) Org {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Create(ctx, Company{ID: "c1", Person: "ana", Name: "Lume Moda", Industry: "online shop"}, "Ana"); err != nil {
		t.Fatal(err)
	}
	s.SaveDepartment(ctx, "c1", Department{ID: "vendas", Name: "Vendas"})
	s.SaveRole(ctx, "c1", Role{ID: "gerente", Title: "Gerente"})
	s.SaveRole(ctx, "c1", Role{ID: "atendente", Title: "Atendente", Function: "Answer customers", AccountKinds: []string{"whatsapp", "email"}})
	if _, err := s.SaveMember(ctx, "c1", Member{ID: "bia", Kind: Agent, Role: "gerente", Department: "vendas", ReportsTo: CEO, Name: "Bia"}); err != nil {
		t.Fatal(err)
	}
	o, err := s.SaveMember(ctx, "c1", Member{ID: "clara", Kind: Agent, Role: "atendente", Department: "vendas", ReportsTo: "bia", Name: "Clara"})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestACompanyStartsWithItsPersonAtTheTop(t *testing.T) {
	s := newStore(t)
	o, err := s.Create(context.Background(), Company{ID: "c1", Person: "ana", Name: "Lume"}, "Ana")
	if err != nil {
		t.Fatal(err)
	}
	ceo, ok := o.Member(CEO)
	if !ok || ceo.Kind != Person || ceo.Person != "ana" || ceo.Name != "Ana" || ceo.ReportsTo != "" {
		t.Fatalf("CEO seat = %+v", ceo)
	}
	got, _ := s.Org(context.Background(), "c1")
	if len(got.Members) != 1 || got.Name != "Lume" {
		t.Fatalf("stored = %+v", got)
	}
}

func TestTheTreeStaysWhole(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	for _, c := range []struct {
		name string
		m    Member
		want string
	}{
		{"no boss", Member{ID: "x", Kind: Agent, Role: "atendente", Name: "X"}, "report to"},
		{"unknown boss", Member{ID: "x", Kind: Agent, Role: "atendente", Name: "X", ReportsTo: "nobody"}, "not in the company"},
		{"unknown role", Member{ID: "x", Kind: Agent, Role: "chef", Name: "X", ReportsTo: CEO}, "role"},
		{"unknown department", Member{ID: "x", Kind: Agent, Role: "atendente", Name: "X", ReportsTo: CEO, Department: "rh"}, "department"},
		{"bad id", Member{ID: "X Y", Kind: Agent, Role: "atendente", Name: "X", ReportsTo: CEO}, "unique id"},
		{"a loop", Member{ID: "bia", Kind: Agent, Role: "gerente", Name: "Bia", ReportsTo: "clara"}, "loop"},
		{"a person without one", Member{ID: "x", Kind: Person, Name: "X", ReportsTo: CEO}, "needs one"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := s.SaveMember(ctx, "c1", c.m)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
	o, _ := s.Org(ctx, "c1")
	if m, _ := o.Member("bia"); m.ReportsTo != CEO {
		t.Fatalf("a refused change was written: %+v", m)
	}
}

func TestTheCEOSeatKeepsItsPersonAndPlace(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	o, err := s.SaveMember(ctx, "c1", Member{ID: CEO, Kind: Agent, Person: "mallory", ReportsTo: "bia", Name: "Ana Lima"})
	if err != nil {
		t.Fatal(err)
	}
	if ceo, _ := o.Member(CEO); ceo.Person != "ana" || ceo.Kind != Person || ceo.ReportsTo != "" || ceo.Name != "Ana Lima" {
		t.Fatalf("CEO = %+v", ceo)
	}
	if _, err := s.DeleteMember(ctx, "c1", CEO); err == nil {
		t.Fatal("the CEO seat was deleted")
	}
	o, _ = s.Update(ctx, Company{ID: "c1", Person: "mallory", Name: "Lume 2"})
	if o.Person != "ana" || o.Name != "Lume 2" {
		t.Fatalf("company = %+v", o.Company)
	}
}

func TestLettingSomeoneGoMovesTheirReportsUp(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	o, err := s.DeleteMember(context.Background(), "c1", "bia")
	if err != nil {
		t.Fatal(err)
	}
	if clara, _ := o.Member("clara"); clara.ReportsTo != CEO {
		t.Fatalf("Clara reports to %q", clara.ReportsTo)
	}
	if _, ok := o.Member("bia"); ok {
		t.Fatal("Bia is still there")
	}
}

func TestRolesInUseStayAndDepartmentsGoQuietly(t *testing.T) {
	s := newStore(t)
	shop(t, s)
	ctx := context.Background()
	if _, err := s.DeleteRole(ctx, "c1", "atendente"); err == nil || !strings.Contains(err.Error(), "Clara") {
		t.Fatalf("deleting a role in use: %v", err)
	}
	o, err := s.DeleteDepartment(ctx, "c1", "vendas")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range o.Members {
		if m.Department != "" {
			t.Fatalf("%s is still in the deleted department", m.Name)
		}
	}
	if _, err := s.DeleteDepartment(ctx, "c1", "vendas"); err != ErrNotFound {
		t.Fatalf("deleting it again: %v", err)
	}
}

func TestChainAndReports(t *testing.T) {
	o := shop(t, newStore(t))
	chain := o.Chain("clara")
	if len(chain) != 2 || chain[0].ID != "bia" || chain[1].ID != CEO {
		t.Fatalf("chain = %+v", chain)
	}
	if r := o.Reports("bia"); len(r) != 1 || r[0].ID != "clara" {
		t.Fatalf("reports = %+v", r)
	}
}

func TestGrants(t *testing.T) {
	c := Company{Person: "ana", Partners: []Partner{{Person: "rui", Grant: View}, {Person: "leo", Grant: Approve}}}
	for _, x := range []struct {
		person, need string
		want         bool
	}{
		{"ana", Configure, true}, {"rui", View, true}, {"rui", Approve, false}, {"leo", Approve, true}, {"leo", Configure, false}, {"eve", View, false},
	} {
		if got := Allows(c.Grant(x.person), x.need); got != x.want {
			t.Errorf("%s %s = %v", x.person, x.need, got)
		}
	}
	o := Org{Company: Company{Name: "x", Person: "ana", Partners: []Partner{{Person: "rui", Grant: "owner"}}}, Members: []Member{{ID: CEO, Kind: Person, Person: "ana", Name: "Ana"}}}
	if err := o.Check(); err == nil {
		t.Fatal("an unknown grant passed")
	}
}

func TestACompanyFileTravelsWithoutItsPeople(t *testing.T) {
	s := newStore(t)
	o := shop(t, s)
	o.Memory.Task.Auto = true
	b, err := o.Export()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "ana\n") || strings.Contains(string(b), "person:") {
		t.Fatalf("the file names a person:\n%s", b)
	}
	got, err := Import(b, "c2", "rui", "Rui", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Person != "rui" || got.Name != "Lume Moda" || len(got.Roles) != 2 || len(got.Departments) != 1 || len(got.Members) != 3 {
		t.Fatalf("imported = %+v", got)
	}
	if !got.Memory.Task.Auto {
		t.Fatalf("the memory's rules were lost: %+v", got.Memory)
	}
	if ceo, _ := got.Member(CEO); ceo.Person != "rui" || ceo.Name != "Rui" {
		t.Fatalf("CEO = %+v", ceo)
	}
	if r, _ := got.Role("atendente"); len(r.AccountKinds) != 2 || r.Function != "Answer customers" {
		t.Fatalf("role = %+v", r)
	}
	if err := s.Replace(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"format: 2\nname: x\nmembers: []", "name: x", "format: 1\nname: x\nmembers:\n  - {id: a, name: A, role: none, reports_to: ceo}"} {
		if _, err := Import([]byte(bad), "c3", "rui", "Rui", time.Now()); err == nil {
			t.Errorf("imported %q", bad)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Ana Lima": "ana-lima", "  Atendente — Instagram ": "atendente-instagram", "Gestão de tráfego": "gestao-de-trafego", "!!!": ""} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHours(t *testing.T) {
	utc := time.UTC
	at := func(day time.Weekday, hm string) time.Time {
		var h, m int
		fmt.Sscanf(hm, "%d:%d", &h, &m)
		// 2026-01-04 is a Sunday.
		return time.Date(2026, 1, 4+int(day), h, m, 0, 0, utc)
	}
	office := Hours{Days: []int{1, 2, 3, 4, 5}, From: "09:00", To: "18:00"}
	night := Hours{Days: []int{5}, From: "22:00", To: "06:00"}
	for _, c := range []struct {
		h    Hours
		t    time.Time
		want bool
	}{
		{office, at(time.Monday, "09:00"), true}, {office, at(time.Monday, "18:00"), false}, {office, at(time.Saturday, "10:00"), false},
		{night, at(time.Friday, "23:00"), true}, {night, at(time.Saturday, "05:59"), true}, {night, at(time.Saturday, "23:00"), false},
		{Hours{}, at(time.Sunday, "03:00"), true},
	} {
		if got := c.h.Open(c.t, utc); got != c.want {
			t.Errorf("%+v at %v = %v", c.h, c.t, got)
		}
	}
	for _, bad := range []Hours{{Days: []int{7}, From: "09:00", To: "18:00"}, {Days: []int{1}, From: "9h", To: "18:00"}, {Days: []int{1}, From: "09:00", To: "09:00"}} {
		if bad.check() == nil {
			t.Errorf("%+v passed", bad)
		}
	}
}

func TestWhoMayWork(t *testing.T) {
	s := newStore(t)
	o := shop(t, s)
	if ok, _ := o.Working("clara"); !ok {
		t.Fatal("Clara may not work")
	}
	o.Departments[0].Paused = true
	if ok, why := o.Working("clara"); ok || !strings.Contains(why, "Vendas") {
		t.Fatalf("a paused department: %v %q", ok, why)
	}
	o.Departments[0].Paused, o.Paused = false, true
	if ok, why := o.Working("bia"); ok || !strings.Contains(why, "company") {
		t.Fatalf("a paused company: %v %q", ok, why)
	}
	if err := o.CheckWork(CEO); err == nil {
		t.Fatal("the CEO seat was given work")
	}
}
