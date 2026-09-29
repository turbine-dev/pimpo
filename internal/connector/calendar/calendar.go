// Package calendar reads calendars from iCalendar feeds, such as the
// private "secret address in iCal format" Google Calendar gives every
// calendar. Read-only, no OAuth.
package calendar

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"

	"github.com/turbine-dev/pimpo/internal/connector"
)

type Event struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Start     string   `json:"start"`
	End       string   `json:"end"`
	AllDay    bool     `json:"all_day"`
	Location  string   `json:"location,omitempty"`
	Attendees []string `json:"attendees"`
	Calendar  string   `json:"calendar"`
}

// Feeds returns the feed URLs to read. It is a function so URLs stay in the
// vault until the moment of the request.
type Feeds func(ctx context.Context) ([]string, error)

type Calendar struct {
	Feeds Feeds
	HTTP  *http.Client
	Zone  *time.Location
	// TTL caches feeds briefly; one routine often asks twice.
	TTL time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at  time.Time
	cal *ics.Calendar
}

func (c *Calendar) Capabilities() []string { return []string{"calendar.events"} }

func (c *Calendar) Call(ctx context.Context, _, _ string, args any) (any, error) {
	var a struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := connector.Args(args, &a); err != nil {
		return nil, err
	}
	zone := c.Zone
	if zone == nil {
		zone = time.Local
	}
	from, err := parseTime(a.From, zone)
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	to, err := parseTime(a.To, zone)
	if err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}
	if !to.After(from) {
		return nil, fmt.Errorf("to must be after from")
	}
	urls, err := c.Feeds(ctx)
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, u := range urls {
		cal, err := c.fetch(ctx, u)
		if err != nil {
			return nil, err
		}
		out = append(out, expand(cal, from, to, zone)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	if out == nil {
		out = []Event{}
	}
	return out, nil
}

// parseTime accepts RFC 3339, a local date and time without an offset
// (read in zone, as people and models often write it), or a plain date
// (midnight in zone).
func parseTime(s string, zone *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, zone); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date or RFC 3339 time", s)
}

func (c *Calendar) fetch(ctx context.Context, url string) (*ics.Calendar, error) {
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]cached{}
	}
	if e, ok := c.cache[url]; ok && time.Since(e.at) < c.TTL {
		c.mu.Unlock()
		return e.cal, nil
	}
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar feed unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("calendar feed answered %d; the link may have been reset", resp.StatusCode)
	}
	cal, err := ics.ParseCalendar(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("calendar feed is not iCalendar: %w", err)
	}
	c.mu.Lock()
	c.cache[url] = cached{time.Now(), cal}
	c.mu.Unlock()
	return cal, nil
}

func expand(cal *ics.Calendar, from, to time.Time, zone *time.Location) []Event {
	name := ""
	for _, p := range cal.CalendarProperties {
		if p.IANAToken == "X-WR-CALNAME" {
			name = p.Value
		}
	}
	var out []Event
	for _, ev := range cal.Events() {
		if s := prop(ev, ics.ComponentPropertyStatus); strings.EqualFold(s, "CANCELLED") {
			continue
		}
		start, allDay, err := when(ev, ics.ComponentPropertyDtStart, zone)
		if err != nil {
			continue
		}
		end, _, err := when(ev, ics.ComponentPropertyDtEnd, zone)
		if err != nil {
			end = start
			if allDay {
				end = start.AddDate(0, 0, 1)
			}
		}
		dur := end.Sub(start)
		base := Event{
			Title:    prop(ev, ics.ComponentPropertySummary),
			AllDay:   allDay,
			Location: prop(ev, ics.ComponentPropertyLocation),
			Calendar: name,
		}
		for _, a := range ev.Attendees() {
			base.Attendees = append(base.Attendees, strings.TrimPrefix(strings.ToLower(a.Email()), "mailto:"))
		}
		if base.Attendees == nil {
			base.Attendees = []string{}
		}
		uid := prop(ev, ics.ComponentPropertyUniqueId)
		for _, s := range occurrences(ev, start, from.Add(-dur), to, zone) {
			if !s.Add(dur).After(from) || !s.Before(to) {
				continue
			}
			e := base
			e.ID = uid + "@" + s.UTC().Format("20060102T150405Z")
			e.Start, e.End = format(s.In(zone), allDay), format(s.Add(dur).In(zone), allDay)
			out = append(out, e)
		}
	}
	return out
}

func occurrences(ev *ics.VEvent, start, from, to time.Time, zone *time.Location) []time.Time {
	rule := prop(ev, ics.ComponentPropertyRrule)
	if rule == "" {
		return []time.Time{start}
	}
	opt, err := rrule.StrToROptionInLocation(rule, start.Location())
	if err != nil {
		return []time.Time{start}
	}
	opt.Dtstart = start
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return []time.Time{start}
	}
	skip := map[int64]bool{}
	for _, p := range ev.GetProperties(ics.ComponentPropertyExdate) {
		for _, v := range strings.Split(p.Value, ",") {
			if t, err := parseICSTime(v, p.ICalParameters, zone); err == nil {
				skip[t.Unix()] = true
			}
		}
	}
	var out []time.Time
	for _, t := range r.Between(from, to, true) {
		if !skip[t.Unix()] {
			out = append(out, t)
		}
	}
	return out
}

func when(ev *ics.VEvent, p ics.ComponentProperty, zone *time.Location) (time.Time, bool, error) {
	prop := ev.GetProperty(p)
	if prop == nil {
		return time.Time{}, false, fmt.Errorf("missing %s", p)
	}
	allDay := len(prop.Value) == 8
	t, err := parseICSTime(prop.Value, prop.ICalParameters, zone)
	return t, allDay, err
}

func parseICSTime(v string, params map[string][]string, zone *time.Location) (time.Time, error) {
	loc := zone
	if tz, ok := params["TZID"]; ok && len(tz) > 0 {
		if l, err := time.LoadLocation(tz[0]); err == nil {
			loc = l
		}
	}
	switch {
	case len(v) == 8:
		return time.ParseInLocation("20060102", v, zone)
	case strings.HasSuffix(v, "Z"):
		return time.Parse("20060102T150405Z", v)
	default:
		return time.ParseInLocation("20060102T150405", v, loc)
	}
}

func format(t time.Time, allDay bool) string {
	if allDay {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

func prop(ev *ics.VEvent, p ics.ComponentProperty) string {
	if v := ev.GetProperty(p); v != nil {
		return v.Value
	}
	return ""
}
