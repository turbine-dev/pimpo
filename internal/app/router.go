package app

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/denerFernandes/pimpo/internal/llm"
)

// Automatic model choice: before a chat request, Pimpo weighs how
// demanding it is and sends clear cases elsewhere: a quick question to the
// light model, a heavy one to the strong model. Anything in between, or
// any doubt, stays on the model the owner set for tasks. Jev weighs the
// request when it is set up; otherwise a few plain rules do.

// Auto is the chat's model choice meaning "let Pimpo choose".
const Auto = "auto"

// routed is the model a request goes to, how hard it thinks, and why.
type routed struct {
	Model string `json:"model"`
	// Tier is simple, normal or hard when the request was weighed.
	Tier string `json:"tier,omitempty"`
	// By is jev, rules, fixed or default: how the model was chosen.
	By string `json:"by"`
	// Effort is the level of thinking, "" for the model's own default.
	Effort string `json:"effort,omitempty"`
	// EffortBy is fixed when the owner set the level, auto when it
	// followed the weighing, default when it is the job's default.
	EffortBy string `json:"effort_by,omitempty"`
}

// tierChooser weighs a request; tests replace it.
var tierChooser = func(a *App) (chooser, bool) { return meaningJudge(a) }

// routeModel picks the model and the effort for a chat request. fixed and
// effort are what the owner chose for the conversation, "" or Auto for
// Pimpo's choice. The same weighing drives both: a quick request goes to
// the light model thinking little, a heavy one to the strong model
// thinking hard, and anything else stays on the tasks model at its usual
// level.
func (a *App) routeModel(ctx context.Context, request, conversation, fixed, effort string) routed {
	s := a.Settings(ctx)
	fixedModel := fixed != "" && fixed != Auto
	fixedEffort := effort != "" && effort != Auto
	tier, by := "", ""
	if !s.AutoOff && !(fixedModel && fixedEffort) {
		tier, by = a.weigh(ctx, request, conversation)
	}
	room := a.roomToSpend(ctx)
	var r routed
	switch {
	case fixedModel:
		r = routed{Model: fixed, Tier: tier, By: "fixed"}
	case s.AutoOff:
		r = routed{Model: s.ExploreModel, By: "default"}
	default:
		light, strong := a.autoModels(ctx)
		switch {
		case tier == "simple" && light != "":
			r = routed{Model: light, Tier: tier, By: by}
		case tier == "hard" && strong != "" && room:
			r = routed{Model: strong, Tier: tier, By: by}
		default:
			r = routed{Model: s.ExploreModel, Tier: "normal", By: by}
			if tier != "" {
				r.Tier = tier
			}
		}
	}
	switch {
	case fixedEffort:
		r.Effort, r.EffortBy = effort, "fixed"
	case tier == "simple":
		r.Effort, r.EffortBy = "low", "auto"
	case tier == "hard" && room:
		r.Effort, r.EffortBy = "high", "auto"
	default:
		r.Effort = s.Efforts["explore"]
		if r.Effort != "" {
			r.EffortBy = "default"
		}
	}
	return r
}

// weigh says how demanding a request is. A level counts only when it is
// clearly more likely than the others; doubt is "normal".
func (a *App) weigh(ctx context.Context, request, conversation string) (string, string) {
	if j, ok := tierChooser(a); ok {
		if len(conversation) > 1200 {
			conversation = conversation[len(conversation)-1200:]
		}
		state := map[string]string{"request": request, "conversation_so_far": conversation}
		probs, err := j.Choose(ctx, "How demanding is `request` for a personal AI agent that reads the owner's email and calendar, searches the web and acts on connected services? Judge the work it needs, not the topic.", state, map[string]string{
			"simple": "A quick answer or a single lookup: a fact, a time, a conversion, one item from the calendar or email, a short reply. No planning and at most one or two tool uses.",
			"normal": "A few steps or a moderate amount of reading and writing: summarise several items, compare a few options, draft a message, look something up on the web and report it.",
			"hard":   "Many steps or careful judgment: plan and act across several services, write a long or delicate text, analyse a lot of material, or set up something that will run by itself every time.",
		})
		if err == nil {
			for _, tier := range []string{"simple", "hard"} {
				if probs[tier] >= 0.6 {
					return tier, "jev"
				}
			}
			return "normal", "jev"
		}
	}
	return rulesTier(request), "rules"
}

var (
	heavyWords  = regexp.MustCompile(`(?i)\b(analis|compar|planej|pesquis|relat[oó]ri|estrat[eé]g|analy[sz]|research|report|strateg|plan (a|my|the)|every (day|week|month)|todo (dia|m[eê]s)|toda semana|sempre que|whenever)`)
	actionWords = regexp.MustCompile(`(?i)\b(envi|mand|arquiv|apag|escrev|respond|crie|marque|agende|send|archive|delete|write|reply|create|schedule|book)`)
)

// rulesTier is the fallback weighing: short plain questions are simple,
// long or planning requests are hard, the rest normal.
func rulesTier(request string) string {
	r := strings.TrimSpace(request)
	n := len([]rune(r))
	switch {
	case n > 500 || heavyWords.MatchString(r):
		return "hard"
	case n <= 90 && strings.HasSuffix(r, "?") && !actionWords.MatchString(r) && strings.Count(r, "?") == 1:
		return "simple"
	}
	return "normal"
}

// autoModels are the light and strong models: the owner's picks, or the
// best guesses among what is available (Claude Code's Haiku and Opus, else
// the cheapest and dearest priced models, local ones counting as light).
func (a *App) autoModels(ctx context.Context) (light, strong string) {
	s := a.Settings(ctx)
	light, strong = s.AutoLight, s.AutoStrong
	isClaude := slices.Contains([]string{"sonnet", "opus", "haiku"}, s.ExploreModel)
	if isClaude && claudeInstalled() {
		if light == "" {
			light = "haiku"
		}
		if strong == "" {
			strong = "opus"
		}
	}
	if _, name, ok := strings.Cut(s.ExploreModel, ":"); ok && name != "" && (light == "" || strong == "") {
		var cheap, dear *ModelOption
		for i, m := range s.Models {
			// opencode models carry no price of their own; opencode reports it.
			if llm.IsOpencode(m.ID) {
				continue
			}
			p := m.PriceIn + m.PriceOut
			if cheap == nil || p < cheap.PriceIn+cheap.PriceOut {
				cheap = &s.Models[i]
			}
			if dear == nil || p > dear.PriceIn+dear.PriceOut {
				dear = &s.Models[i]
			}
		}
		if light == "" && cheap != nil {
			light = cheap.ID
		}
		if strong == "" && dear != nil && dear.PriceIn+dear.PriceOut > 0 {
			strong = dear.ID
		}
	}
	if light == s.ExploreModel {
		light = ""
	}
	if strong == s.ExploreModel {
		strong = ""
	}
	return light, strong
}

// roomToSpend keeps the strong model for days with budget left.
func (a *App) roomToSpend(ctx context.Context) bool {
	limit := a.Budget.Limit(ctx)
	if limit <= 0 {
		return true
	}
	spent, _ := a.Budget.Today(ctx)
	return spent < limit*0.8
}

// Chat and exploration keys for the chosen and the used model.
func chatModelKey(chat string) string       { return "chat.model." + chat }
func chatEffortKey(chat string) string      { return "chat.effort." + chat }
func explorationModelKey(exp string) string { return "exploration.model." + exp }

func (a *App) chatModel(ctx context.Context, chat string) string {
	m, _ := a.Events.Get(ctx, chatModelKey(chat))
	return m
}

func (a *App) setChatModel(ctx context.Context, chat, model string) {
	if model == Auto {
		model = ""
	}
	a.Events.Put(ctx, chatModelKey(chat), model)
}

func (a *App) chatEffort(ctx context.Context, chat string) string {
	e, _ := a.Events.Get(ctx, chatEffortKey(chat))
	return e
}

func (a *App) setChatEffort(ctx context.Context, chat, effort string) {
	if effort == Auto {
		effort = ""
	}
	a.Events.Put(ctx, chatEffortKey(chat), effort)
}

func (a *App) noteRouted(ctx context.Context, exp string, r routed) {
	b, _ := json.Marshal(r)
	a.Events.Put(ctx, explorationModelKey(exp), string(b))
	a.Events.Append(ctx, "model.routed", "system", map[string]any{"exploration": exp, "model": r.Model, "tier": r.Tier, "by": r.By, "effort": r.Effort})
}

func (a *App) routedOf(ctx context.Context, exp string) *routed {
	raw, _ := a.Events.Get(ctx, explorationModelKey(exp))
	var r routed
	if raw == "" || json.Unmarshal([]byte(raw), &r) != nil {
		return nil
	}
	return &r
}

// usableModel says whether a model can be chosen: Claude Code, Codex, or
// one of the owner's priced models.
func (a *App) usableModel(ctx context.Context, model string) bool {
	if model == "" || model == Auto || slices.Contains([]string{"sonnet", "opus", "haiku"}, model) || isCodex(model) {
		return true
	}
	if p, _, ok := strings.Cut(model, ":"); ok && (slices.Contains(llm.Providers, p) || llm.IsOpencode(model)) {
		for _, m := range a.Settings(ctx).Models {
			if m.ID == model {
				return true
			}
		}
	}
	return false
}

// usableEffort says whether an effort can be chosen for a chat.
func usableEffort(e string) bool { return e == Auto || llm.ValidEffort(e) }
