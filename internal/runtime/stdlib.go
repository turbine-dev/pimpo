package runtime

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// The routine standard library. goja has no Intl, and hand-written date and
// money handling is where compiled routines broke most, so the common cases
// live here: dates in the owner's zone and language, and money amounts.

var weekdays = map[string][2][]string{
	"pt": {{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}, {"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}},
	"en": {{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}, {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}},
}

var months = map[string][2][]string{
	"pt": {{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}, {"jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"}},
	"en": {{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}, {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}},
}

func lang(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "pt") {
		return "pt"
	}
	return "en"
}

func installStdlib(vm *goja.Runtime, zone *time.Location, locale string, now time.Time) {
	l := lang(locale)
	parse := func(s string) (time.Time, bool) {
		for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.ParseInLocation(f, s, zone); err == nil {
				return t.In(zone), true
			}
		}
		return time.Time{}, false
	}
	must := func(s string) time.Time {
		t, ok := parse(s)
		if !ok {
			panic(vm.NewTypeError(fmt.Sprintf("dates: %q is not an ISO date", s)))
		}
		return t
	}
	iso := func(t time.Time) string { return t.In(zone).Format(time.RFC3339) }
	day := func(t time.Time) time.Time {
		t = t.In(zone)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, zone)
	}

	dates := vm.NewObject()
	dates.Set("zone", func() string { return zone.String() })
	dates.Set("today", func() string { return iso(day(now)) })
	dates.Set("startOfDay", func(s string, offset int) string { return iso(day(must(s)).AddDate(0, 0, offset)) })
	dates.Set("addDays", func(s string, n int) string { return iso(must(s).AddDate(0, 0, n)) })
	dates.Set("addHours", func(s string, n float64) string { return iso(must(s).Add(time.Duration(n * float64(time.Hour)))) })
	dates.Set("diffDays", func(a, b string) int { return int(math.Round(day(must(b)).Sub(day(must(a))).Hours() / 24)) })
	dates.Set("sameDay", func(a, b string) bool { return day(must(a)).Equal(day(must(b))) })
	dates.Set("isBefore", func(a, b string) bool { return must(a).Before(must(b)) })
	dates.Set("weekday", func(s string) int { return int(must(s).Weekday()) })
	dates.Set("format", func(s, pattern string) string { return formatDate(must(s), pattern, l) })
	dates.Set("parse", func(text string) goja.Value {
		if t, ok := parseHuman(text, now.In(zone), zone); ok {
			return vm.ToValue(iso(t))
		}
		return goja.Null()
	})
	vm.Set("dates", dates)

	money := vm.NewObject()
	money.Set("parse", func(text string) goja.Value {
		if v, ok := parseMoney(text); ok {
			return vm.ToValue(v)
		}
		return goja.Null()
	})
	money.Set("find", func(text string) goja.Value {
		if m := moneyPattern.FindString(text); m != "" {
			return vm.ToValue(strings.TrimSpace(m))
		}
		return goja.Null()
	})
	money.Set("format", func(v float64, currency string) string { return formatMoney(v, currency, l) })
	vm.Set("money", money)
}

// formatDate understands EEEE (weekday), EEE, dd, d, MMMM, MMM, MM, yyyy,
// HH, mm. Text in single quotes is copied as is: "d 'de' MMMM".
func formatDate(t time.Time, pattern, l string) string {
	repl := []struct{ tok, val string }{
		{"EEEE", weekdays[l][0][t.Weekday()]},
		{"EEE", weekdays[l][1][t.Weekday()]},
		{"MMMM", months[l][0][t.Month()-1]},
		{"MMM", months[l][1][t.Month()-1]},
		{"yyyy", strconv.Itoa(t.Year())},
		{"MM", fmt.Sprintf("%02d", int(t.Month()))},
		{"dd", fmt.Sprintf("%02d", t.Day())},
		{"HH", fmt.Sprintf("%02d", t.Hour())},
		{"mm", fmt.Sprintf("%02d", t.Minute())},
		{"d", strconv.Itoa(t.Day())},
	}
	var b strings.Builder
	for i := 0; i < len(pattern); {
		if pattern[i] == '\'' {
			end := strings.IndexByte(pattern[i+1:], '\'')
			if end < 0 {
				end = len(pattern) - i - 1
			}
			b.WriteString(pattern[i+1 : i+1+end])
			i += end + 2
			continue
		}
		matched := false
		for _, r := range repl {
			if strings.HasPrefix(pattern[i:], r.tok) {
				b.WriteString(r.val)
				i += len(r.tok)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(pattern[i])
			i++
		}
	}
	return b.String()
}

var (
	numericDate = regexp.MustCompile(`\b(\d{1,2})[/.-](\d{1,2})(?:[/.-](\d{2,4}))?\b`)
	isoDate     = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	wordDate    = regexp.MustCompile(`(?i)\b(\d{1,2})(?:º|st|nd|rd|th)?\s+(?:de\s+)?([a-zçã]{3,})\.?(?:\s+(?:de\s+)?(\d{4}))?`)
	wordDateEN  = regexp.MustCompile(`(?i)\b([a-z]{3,})\.?\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?`)
)

// parseHuman finds the first date in free text: 2026-09-26, 26/09/2026,
// 26/09 (this year), "26 de setembro", "Sep 26, 2026".
func parseHuman(text string, now time.Time, zone *time.Location) (time.Time, bool) {
	if m := isoDate.FindStringSubmatch(text); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return mkDate(y, mo, d, zone)
	}
	if m := numericDate.FindStringSubmatch(text); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y := now.Year()
		if m[3] != "" {
			y, _ = strconv.Atoi(m[3])
			if y < 100 {
				y += 2000
			}
		}
		return mkDate(y, mo, d, zone)
	}
	if m := wordDate.FindStringSubmatch(text); m != nil {
		if mo := monthNumber(m[2]); mo > 0 {
			d, _ := strconv.Atoi(m[1])
			y := now.Year()
			if m[3] != "" {
				y, _ = strconv.Atoi(m[3])
			}
			return mkDate(y, mo, d, zone)
		}
	}
	if m := wordDateEN.FindStringSubmatch(text); m != nil {
		if mo := monthNumber(m[1]); mo > 0 {
			d, _ := strconv.Atoi(m[2])
			y := now.Year()
			if m[3] != "" {
				y, _ = strconv.Atoi(m[3])
			}
			return mkDate(y, mo, d, zone)
		}
	}
	return time.Time{}, false
}

func mkDate(y, mo, d int, zone *time.Location) (time.Time, bool) {
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, zone)
	if t.Day() != d {
		return time.Time{}, false
	}
	return t, true
}

func monthNumber(word string) int {
	w := strings.ToLower(strings.TrimSuffix(word, "."))
	for _, l := range []string{"pt", "en"} {
		for i, full := range months[l][0] {
			short := months[l][1][i]
			if w == strings.ToLower(full) || w == strings.ToLower(short) || (len(w) >= 3 && strings.HasPrefix(strings.ToLower(full), w)) {
				return i + 1
			}
		}
	}
	return 0
}

var moneyPattern = regexp.MustCompile(`(?:R\$|US\$|\$|€|£)\s?-?\d{1,3}(?:[.,\s]\d{3})*(?:[.,]\d{1,2})?|-?\d{1,3}(?:[.,]\d{3})*[.,]\d{2}\s?(?:reais|BRL|USD|EUR)`)

// parseMoney reads amounts in Brazilian (1.482,35) or US (1,482.35) style.
func parseMoney(text string) (float64, bool) {
	m := moneyPattern.FindString(text)
	if m == "" {
		m = text
	}
	digits := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' {
			return r
		}
		return -1
	}, m)
	if digits == "" {
		return 0, false
	}
	lastDot, lastComma := strings.LastIndex(digits, "."), strings.LastIndex(digits, ",")
	dec := -1
	switch {
	case lastComma > lastDot && len(digits)-lastComma-1 <= 2:
		dec = lastComma
	case lastDot > lastComma && len(digits)-lastDot-1 <= 2:
		dec = lastDot
	}
	var intPart, frac string
	if dec >= 0 {
		intPart, frac = digits[:dec], digits[dec+1:]
	} else {
		intPart = digits
	}
	intPart = strings.NewReplacer(".", "", ",", "").Replace(intPart)
	v, err := strconv.ParseFloat(intPart+"."+frac+"0", 64)
	if err != nil {
		return 0, false
	}
	return math.Round(v*100) / 100, true
}

func formatMoney(v float64, currency, l string) string {
	symbol := map[string]string{"BRL": "R$ ", "USD": "US$ ", "EUR": "€ ", "GBP": "£ "}[strings.ToUpper(currency)]
	if symbol == "" {
		symbol = "R$ "
	}
	neg := v < 0
	s := strconv.FormatFloat(math.Abs(v), 'f', 2, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	thousands, decimal := ".", ","
	if l == "en" || strings.ToUpper(currency) == "USD" {
		thousands, decimal = ",", "."
		if strings.ToUpper(currency) == "USD" {
			symbol = "$"
		}
	}
	var groups []string
	for len(intPart) > 3 {
		groups = append([]string{intPart[len(intPart)-3:]}, groups...)
		intPart = intPart[:len(intPart)-3]
	}
	groups = append([]string{intPart}, groups...)
	out := symbol + strings.Join(groups, thousands) + decimal + frac
	if neg {
		out = "-" + out
	}
	return out
}
