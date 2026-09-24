package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/denerFernandes/vigia/internal/capability"
	"github.com/denerFernandes/vigia/internal/connector"
)

// Critical domains move things in the physical world that matter for
// safety: they only go through ha.critical, which always asks.
var critical = map[string]bool{"lock": true, "alarm_control_panel": true, "cover": true, "valve": true, "siren": true}

func init() {
	register(Kind{
		ID: "homeassistant", Title: "Home Assistant", Description: "Estados da casa e ações em dispositivos.",
		Help: "Em Home Assistant › Perfil › Segurança, crie um token de longa duração. Fechaduras, alarmes, portões e válvulas sempre pedem sua aprovação.",
		Fields: []Field{
			{Name: "url", Label: "Endereço do Home Assistant", Placeholder: "http://homeassistant.local:8123"},
			{Name: "token", Label: "Token de longa duração", Secret: true},
		},
		Specs: []capability.Spec{
			{Name: "ha.states", Risk: capability.Read, Signature: "ha.states({domain})", Returns: "[{entity_id, state, name, changed}] domain filters, e.g. light, sensor",
				Schema: obj(`"domain":{"type":"string"}`)},
			{Name: "ha.call", Risk: capability.Reversible, Signature: "ha.call({domain, service, entity_id, data})", Returns: "{ok}; lights, switches, climate, media; not locks, alarms, covers or valves",
				Schema: obj(`"domain":{"type":"string"},"service":{"type":"string"},"entity_id":{"type":"string"},"data":{"type":"object"}`, "domain", "service", "entity_id")},
			{Name: "ha.critical", Risk: capability.Irreversible, Signature: "ha.critical({domain, service, entity_id})", Returns: "{ok}; locks, alarms, covers, valves and sirens; always needs approval",
				Schema: obj(`"domain":{"type":"string"},"service":{"type":"string"},"entity_id":{"type":"string"}`, "domain", "service", "entity_id")},
		},
		Call: callHA,
		Probe: func(ctx context.Context, cfg Config) error {
			_, err := callHA(ctx, cfg, "ha.states", "", map[string]any{"domain": "sun"})
			return err
		},
	})
}

var ident = regexp.MustCompile(`^[a-z0-9_]+$`)

func callHA(ctx context.Context, cfg Config, name, _ string, args any) (any, error) {
	v, err := need(ctx, cfg, "Home Assistant", "url", "token")
	if err != nil {
		return nil, err
	}
	api := strings.TrimRight(v[0], "/") + "/api"
	h := map[string]string{"Authorization": "Bearer " + v[1]}
	var a struct {
		Domain   string         `json:"domain"`
		Service  string         `json:"service"`
		EntityID string         `json:"entity_id"`
		Data     map[string]any `json:"data"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	switch name {
	case "ha.states":
		var raw []struct {
			EntityID    string         `json:"entity_id"`
			State       string         `json:"state"`
			LastChanged string         `json:"last_changed"`
			Attributes  map[string]any `json:"attributes"`
		}
		if err := doJSON(ctx, "GET", api+"/states", h, nil, &raw); err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, s := range raw {
			if a.Domain != "" && !strings.HasPrefix(s.EntityID, a.Domain+".") {
				continue
			}
			name, _ := s.Attributes["friendly_name"].(string)
			out = append(out, map[string]any{"entity_id": s.EntityID, "state": s.State, "name": name, "changed": s.LastChanged})
		}
		return out, nil
	case "ha.call", "ha.critical":
		if !ident.MatchString(a.Domain) || !ident.MatchString(a.Service) || !strings.HasPrefix(a.EntityID, a.Domain+".") {
			return nil, errors.New("domain, service and an entity_id of that domain are required")
		}
		if critical[a.Domain] != (name == "ha.critical") {
			if name == "ha.call" {
				return nil, fmt.Errorf("%s devices go through ha.critical, which always asks first", a.Domain)
			}
			return nil, fmt.Errorf("%s is not a critical domain; use ha.call", a.Domain)
		}
		body := map[string]any{"entity_id": a.EntityID}
		if name == "ha.call" {
			for k, v := range a.Data {
				if k != "entity_id" {
					body[k] = v
				}
			}
		}
		if err := doJSON(ctx, "POST", api+"/services/"+url.PathEscape(a.Domain)+"/"+url.PathEscape(a.Service), h, body, nil); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	}
	return nil, fmt.Errorf("unknown capability %s", name)
}
