package runtime

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Param is a setting of a routine the owner can change without touching
// its code: the city of a weather report, a price limit, a sender.
type Param struct {
	// Name is how the code reads it: params.<name>.
	Name  string `json:"name"`
	Label string `json:"label"`
	// Type is text, number, boolean, date, time, location, select,
	// multiselect, email or destinations.
	Type    string   `json:"type"`
	Default any      `json:"default,omitempty"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// Location is the value of a location parameter.
type Location struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone,omitempty"`
	Country   string  `json:"country,omitempty"`
}

var paramTypes = map[string]bool{"text": true, "number": true, "boolean": true, "date": true, "time": true, "location": true, "select": true, "multiselect": true, "email": true, "destinations": true}

var clock = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (p Param) validate() error {
	if !isIdent(p.Name) {
		return fmt.Errorf("parameter name %q must be a JavaScript identifier", p.Name)
	}
	if !paramTypes[p.Type] {
		return fmt.Errorf("parameter %s: unknown type %q", p.Name, p.Type)
	}
	if (p.Type == "select" || p.Type == "multiselect") && len(p.Options) == 0 {
		return fmt.Errorf("parameter %s: a %s needs options", p.Name, p.Type)
	}
	if p.Default != nil {
		if _, err := p.coerce(p.Default); err != nil {
			return fmt.Errorf("parameter %s: default: %w", p.Name, err)
		}
	}
	return nil
}

// coerce checks a value against the parameter's type and returns it in
// the shape the routine sees.
func (p Param) coerce(v any) (any, error) {
	switch p.Type {
	case "text":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be text")
		}
		return s, nil
	case "number":
		switch n := v.(type) {
		case float64:
			return n, nil
		case int:
			return float64(n), nil
		case string:
			f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(n), ",", "."), 64)
			if err != nil {
				return nil, errors.New("must be a number")
			}
			return f, nil
		}
		return nil, errors.New("must be a number")
	case "boolean":
		b, ok := v.(bool)
		if !ok {
			return nil, errors.New("must be yes or no")
		}
		return b, nil
	case "date":
		s, _ := v.(string)
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, errors.New("must be a date like 2026-10-05")
		}
		return s, nil
	case "time":
		s, _ := v.(string)
		if !clock.MatchString(s) {
			return nil, errors.New("must be a time like 07:30")
		}
		return s, nil
	case "email":
		s, _ := v.(string)
		if a, err := mail.ParseAddress(s); err != nil || a.Address != s {
			return nil, errors.New("must be an email address")
		}
		return s, nil
	case "select":
		s, _ := v.(string)
		for _, o := range p.Options {
			if o == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("must be one of %s", strings.Join(p.Options, ", "))
	case "multiselect", "destinations":
		list, err := strings2(v)
		if err != nil {
			return nil, err
		}
		if p.Type == "multiselect" {
			for _, s := range list {
				found := false
				for _, o := range p.Options {
					found = found || o == s
				}
				if !found {
					return nil, fmt.Errorf("%q is not an option", s)
				}
			}
		}
		return list, nil
	case "location":
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("must be a place")
		}
		lat, ok1 := m["latitude"].(float64)
		lon, ok2 := m["longitude"].(float64)
		name, _ := m["name"].(string)
		if !ok1 || !ok2 || lat < -90 || lat > 90 || lon < -180 || lon > 180 || name == "" {
			return nil, errors.New("must be a place with a name, latitude and longitude")
		}
		out := map[string]any{"name": name, "latitude": lat, "longitude": lon}
		for _, k := range []string{"timezone", "country"} {
			if s, ok := m[k].(string); ok && s != "" {
				out[k] = s
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown type %q", p.Type)
}

func strings2(v any) ([]string, error) {
	switch l := v.(type) {
	case []string:
		return l, nil
	case []any:
		out := make([]string, 0, len(l))
		for _, x := range l {
			s, ok := x.(string)
			if !ok {
				return nil, errors.New("must be a list of choices")
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, errors.New("must be a list of choices")
}

// ResolveParams fills in defaults and checks every value; the result is
// what the routine sees as params. Unknown names are refused.
func (m Manifest) ResolveParams(values map[string]any) (map[string]any, error) {
	out := map[string]any{}
	known := map[string]bool{}
	for _, p := range m.Params {
		known[p.Name] = true
		v, ok := values[p.Name]
		if !ok || v == nil {
			v = p.Default
		}
		if v == nil {
			return nil, fmt.Errorf("%s is not set", labelOf(p))
		}
		c, err := p.coerce(v)
		if err != nil {
			return nil, fmt.Errorf("%s %s", labelOf(p), err)
		}
		out[p.Name] = c
	}
	for k := range values {
		if !known[k] {
			return nil, fmt.Errorf("the routine has no setting %q", k)
		}
	}
	return out, nil
}

func labelOf(p Param) string {
	if p.Label != "" {
		return p.Label
	}
	return p.Name
}

// Destinations returns the value of the first destinations parameter.
func (m Manifest) Destinations(params map[string]any) []string {
	for _, p := range m.Params {
		if p.Type == "destinations" {
			l, _ := params[p.Name].([]string)
			return l
		}
	}
	return nil
}
