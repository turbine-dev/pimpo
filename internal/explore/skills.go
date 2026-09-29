package explore

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/turbine-dev/pimpo/internal/host"
	"github.com/turbine-dev/pimpo/internal/mcp"
)

// Skill is a SKILL.md skill the agent may load when a task matches it.
// Loading it narrows the rest of the exploration to the capabilities the
// owner granted the skill.
type Skill struct {
	ID, Name, Description string
	Capabilities          []string
	// Instructions reads the skill's text, framed as a third party's.
	Instructions func() (string, error)
}

func skillsPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nSkills the owner installed (written by third parties). When the task matches one, call use_skill with its id before doing it; its text then guides you, and from then on you can only use the capabilities the owner granted that skill:\n")
	for _, s := range skills {
		b.WriteString("- " + s.ID + ": " + s.Name)
		if s.Description != "" {
			b.WriteString(" — " + s.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func skillTools(h *host.Host, skills []Skill) []mcp.Tool {
	if len(skills) == 0 {
		return nil
	}
	byID := map[string]Skill{}
	var ids []string
	for _, s := range skills {
		byID[s.ID] = s
		ids = append(ids, s.ID)
	}
	sort.Strings(ids)
	schema, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": map[string]any{"type": "string", "enum": ids}}})
	return []mcp.Tool{{
		Name:        "use_skill",
		Description: "Loads an installed skill's instructions for this task. After it, only the capabilities the owner granted the skill can be used for the rest of the task.",
		InputSchema: schema,
		Handle: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				ID string `json:"id"`
			}
			json.Unmarshal(raw, &in)
			s, ok := byID[in.ID]
			if !ok {
				return nil, errors.New("no installed skill " + in.ID)
			}
			text, err := s.Instructions()
			if err != nil {
				return nil, err
			}
			h.Narrow(s.Capabilities, "the skill "+s.Name)
			h.Env.Events.Append(ctx, "skill.used", h.Source, map[string]any{"skill": s.ID, "capabilities": s.Capabilities})
			return map[string]any{"instructions": text, "capabilities": s.Capabilities}, nil
		},
	}}
}
