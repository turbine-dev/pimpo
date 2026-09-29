package app

import (
	"context"
	"time"

	"github.com/turbine-dev/pimpo/internal/demo"
	"github.com/turbine-dev/pimpo/internal/undo"
)

// EnableDemo swaps every external service for the demo's: an in-memory
// mailbox and calendar, a scripted explorer and ready routines.
func (a *App) EnableDemo(ctx context.Context, stepDelay time.Duration) {
	box := demo.NewMailbox(time.Now)
	a.Router.Add(box)
	a.Router.Add(demo.Calendar{Now: time.Now})
	a.Router.Add(&demo.Telegram{})
	a.DemoJudge = demo.Judge{}
	a.Agent = demo.Agent{Delay: stepDelay}
	a.LLM = demo.Compiler{}
	a.Undo.Mail = func(context.Context) (undo.Mail, error) { return box, nil }
	a.Events.Put(ctx, "demo", "true")
}
