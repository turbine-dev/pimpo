package company

import (
	"testing"
	"time"

	// The templates' rules name the catalog's capabilities.
	_ "github.com/turbine-dev/pimpo/internal/connector/services"
)

func TestEveryTemplateMakesAWholeCompany(t *testing.T) {
	ts := Templates()
	if len(ts) != 6 || ts[0].ID != "software" || ts[5].ID != "blank" {
		t.Fatalf("templates = %+v", ts)
	}
	for _, tpl := range ts {
		o, err := Import([]byte(tpl.File), "c_"+tpl.ID, "ana", "Ana", time.Now())
		if err != nil {
			t.Errorf("%s: %v", tpl.ID, err)
			continue
		}
		if ceo, _ := o.Member(CEO); ceo.Person != "ana" || ceo.Name != "Ana" {
			t.Errorf("%s: the CEO seat is %+v", tpl.ID, ceo)
		}
		for _, r := range o.AgentRoutines {
			if _, err := ParseSchedule(r.Schedule); err != nil {
				t.Errorf("%s: routine %s: %v", tpl.ID, r.ID, err)
			}
		}
	}
	sw := ts[0]
	if sw.Members != 7 || len(sw.Roles) != 6 {
		t.Fatalf("the software company has %d members in %d roles", sw.Members, len(sw.Roles))
	}
	if _, ok := TemplateFile("../store"); ok {
		t.Fatal("a template outside the templates")
	}
}
