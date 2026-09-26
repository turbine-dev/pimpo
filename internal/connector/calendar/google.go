package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/denerFernandes/pimpo/internal/connector"
)

// Google reads calendars through the Google Calendar API with an OAuth token.
type Google struct {
	Token func(ctx context.Context) (string, error)
	Base  string
	HTTP  *http.Client
	Zone  *time.Location
}

func (g *Google) Capabilities() []string { return []string{"calendar.events"} }

func (g *Google) get(ctx context.Context, path string, q url.Values, out any) error {
	tok, err := g.Token(ctx)
	if err != nil {
		return err
	}
	base := g.Base
	if base == "" {
		base = "https://www.googleapis.com/calendar/v3"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	client := g.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Google Calendar unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Google Calendar answered %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (g *Google) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var a struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	zone := g.Zone
	if zone == nil {
		zone = time.Local
	}
	from, err := parseTime(a.From, zone)
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	to, err := parseTime(a.To, zone)
	if err != nil || !to.After(from) {
		return nil, fmt.Errorf("to must be a time after from")
	}
	var list struct {
		Items []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
			Primary bool   `json:"primary"`
		} `json:"items"`
	}
	if err := g.get(ctx, "/users/me/calendarList", url.Values{"minAccessRole": {"reader"}}, &list); err != nil {
		return nil, err
	}
	out := []Event{}
	for _, cal := range list.Items {
		var evs struct {
			Items []struct {
				ID        string                          `json:"id"`
				Summary   string                          `json:"summary"`
				Location  string                          `json:"location"`
				Status    string                          `json:"status"`
				Start     struct{ DateTime, Date string } `json:"start"`
				End       struct{ DateTime, Date string } `json:"end"`
				Attendees []struct {
					Email string `json:"email"`
				} `json:"attendees"`
			} `json:"items"`
		}
		q := url.Values{"singleEvents": {"true"}, "orderBy": {"startTime"}, "timeMin": {from.Format(time.RFC3339)}, "timeMax": {to.Format(time.RFC3339)}, "maxResults": {"250"}}
		if err := g.get(ctx, "/calendars/"+url.PathEscape(cal.ID)+"/events", q, &evs); err != nil {
			return nil, err
		}
		for _, e := range evs.Items {
			if e.Status == "cancelled" {
				continue
			}
			ev := Event{ID: e.ID, Title: e.Summary, Location: e.Location, Calendar: cal.Summary, Attendees: []string{}}
			if e.Start.DateTime != "" {
				s, _ := time.Parse(time.RFC3339, e.Start.DateTime)
				en, _ := time.Parse(time.RFC3339, e.End.DateTime)
				ev.Start, ev.End = s.In(zone).Format(time.RFC3339), en.In(zone).Format(time.RFC3339)
			} else {
				ev.AllDay, ev.Start, ev.End = true, e.Start.Date, e.End.Date
			}
			for _, at := range e.Attendees {
				ev.Attendees = append(ev.Attendees, strings.ToLower(at.Email))
			}
			out = append(out, ev)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}
