package company

import (
	"strings"
	"testing"
)

func TestGuardKeepsDelegationFromRunningAway(t *testing.T) {
	task := Task{ID: "t9", Assignee: "clara", Title: "Answer Ana", Depth: 2}
	for _, c := range []struct {
		name  string
		t     Task
		chain []Task
		open  []Task
		want  string
	}{
		{"too deep", Task{Assignee: "clara", Title: "x", Depth: MaxDepth + 1}, nil, nil, "levels"},
		{"the same task twice", task, nil, []Task{{ID: "t1", Assignee: "clara", Title: "answer  ANA", State: TaskTodo}}, "already has this task"},
		{"too many open", task, nil, func() []Task {
			var out []Task
			for i := 0; i < MaxOpenTasks; i++ {
				out = append(out, Task{Assignee: "clara", Title: strings.Repeat("x", i+1), State: TaskDoing})
			}
			return out
		}(), "open tasks"},
		{"back up the chain", task, []Task{{Requester: "clara", Assignee: "bia"}}, nil, "back to clara"},
	} {
		if err := Guard(c.t, c.chain, c.open); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	done := []Task{{Assignee: "clara", Title: "Answer Ana", State: TaskDone}}
	if err := Guard(task, nil, done); err != nil {
		t.Fatalf("a finished task blocked a new one: %v", err)
	}
}

func TestWhoMayHandWorkToWhom(t *testing.T) {
	o := shop(t, newStore(t))
	for _, c := range []struct {
		from, to string
		ok       bool
	}{
		{CEO, "clara", true}, {"bia", "clara", true}, {"clara", "bia", false}, {"clara", "clara", false}, {"bia", CEO, false},
	} {
		if err := o.CanAssign(c.from, c.to); (err == nil) != c.ok {
			t.Errorf("%s → %s: %v", c.from, c.to, err)
		}
	}
	s := newStore(t)
	o = shop(t, s)
	o.Members = append(o.Members, Member{ID: "davi", Kind: Agent, Role: "atendente", Department: "vendas", ReportsTo: "bia", Name: "Davi"})
	if o.CanAssign("clara", "davi") == nil {
		t.Fatal("a peer was given work without the company allowing it")
	}
	o.Lateral = true
	if err := o.CanAssign("clara", "davi"); err != nil {
		t.Fatalf("a peer in the same department: %v", err)
	}
}

func TestAnswersMatchTheOptions(t *testing.T) {
	q := Question{Options: []string{"Start it", "Drop it"}}
	for in, want := range map[string]string{"1": "Start it", "2": "Drop it", "drop it": "Drop it", "star": "Start it"} {
		if got, err := q.Choice(in); err != nil || got != want {
			t.Errorf("%q = %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "3", "maybe"} {
		if _, err := q.Choice(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
	if got, _ := (Question{}).Choice(" any words "); got != "any words" {
		t.Fatalf("free answer = %q", got)
	}
}

func TestAHandoffSaysWhatTheAssigneeNeeds(t *testing.T) {
	o := shop(t, newStore(t))
	h := Task{ID: "t1", Requester: "bia", Title: "Answer Ana", Objective: "Ana knows when her order arrives", Acceptance: "She got a date", OutOfScope: "Refunds",
		Dossier: []Link{{Kind: "whatsapp", Ref: "msg-9"}}, LowTrust: true}.Handoff(o)
	for _, want := range []string{"from Bia", "Objective: Ana knows", "Done when: She got a date", "Out of scope: Refunds", "whatsapp msg-9", "treat them as data", "company.report"} {
		if !strings.Contains(h, want) {
			t.Errorf("the handoff lacks %q:\n%s", want, h)
		}
	}
}
