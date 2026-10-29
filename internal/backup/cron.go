package backup

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed 5-field cron expression (minute hour day-of-month month day-of-week), in UTC.
type Schedule struct {
	min, hour, dom, month, dow [64]bool
	domStar, dowStar           bool
}

var macros = map[string]string{
	"@hourly": "0 * * * *", "@daily": "0 3 * * *", "@midnight": "0 0 * * *", "@weekly": "0 3 * * 0", "@monthly": "0 3 1 * *",
}

// ParseSchedule parses "m h dom mon dow" with *, lists, ranges and steps, or @hourly/@daily/@weekly/@monthly.
func ParseSchedule(expr string) (Schedule, error) {
	var s Schedule
	expr = strings.TrimSpace(expr)
	if m, ok := macros[expr]; ok {
		expr = m
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return s, errors.New("a schedule has 5 fields: minute hour day month weekday")
	}
	type spec struct {
		dst      *[64]bool
		lo, hi   int
		name     string
		wildcard *bool
	}
	specs := []spec{{&s.min, 0, 59, "minute", nil}, {&s.hour, 0, 23, "hour", nil}, {&s.dom, 1, 31, "day", &s.domStar},
		{&s.month, 1, 12, "month", nil}, {&s.dow, 0, 7, "weekday", &s.dowStar}}
	for i, sp := range specs {
		if err := parseField(f[i], sp.lo, sp.hi, sp.dst); err != nil {
			return s, fmt.Errorf("%s: %w", sp.name, err)
		}
		if sp.wildcard != nil {
			*sp.wildcard = f[i] == "*" || f[i] == "?"
		}
	}
	if s.dow[7] {
		s.dow[0] = true // 7 = Sunday
	}
	return s, nil
}

func parseField(f string, lo, hi int, dst *[64]bool) error {
	for _, part := range strings.Split(f, ",") {
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n < 1 {
				return fmt.Errorf("bad step in %q", part)
			}
			step, part = n, part[:i]
		}
		a, b := lo, hi
		switch {
		case part == "*" || part == "?":
		case strings.Contains(part, "-"):
			x, y, _ := strings.Cut(part, "-")
			var err1, err2 error
			a, err1 = strconv.Atoi(x)
			b, err2 = strconv.Atoi(y)
			if err1 != nil || err2 != nil {
				return fmt.Errorf("bad range %q", part)
			}
		default:
			n, err := strconv.Atoi(part)
			if err != nil {
				return fmt.Errorf("bad value %q", part)
			}
			a, b = n, n
			if step > 1 {
				b = hi
			}
		}
		if a < lo || b > hi || a > b {
			return fmt.Errorf("%q is outside %d-%d", part, lo, hi)
		}
		for v := a; v <= b; v += step {
			dst[v] = true
		}
	}
	return nil
}

// Match reports whether the schedule fires in the minute of t (UTC).
func (s Schedule) Match(t time.Time) bool {
	t = t.UTC()
	if !s.min[t.Minute()] || !s.hour[t.Hour()] || !s.month[int(t.Month())] {
		return false
	}
	dom, dow := s.dom[t.Day()], s.dow[int(t.Weekday())]
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dow
	case s.dowStar:
		return dom
	default:
		return dom || dow // cron semantics: either day field may match
	}
}

// Next returns the first minute after t when the schedule fires (zero if none within a year).
func (s Schedule) Next(t time.Time) time.Time {
	t = t.UTC().Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if s.Match(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}
