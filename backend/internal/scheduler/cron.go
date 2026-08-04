package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// field is one 5-field cron field. It stores the set of matching values.
type field struct {
	values map[int]bool
}

// ParsedCron is a validated 5-field cron expression.
type ParsedCron [5]field

// ParseCron validates a 5-field cron expression:
// minute(0-59) hour(0-23) day-of-month(1-31) month(1-12) day-of-week(0-6, 0=Sunday).
// Each field supports "*", "*/step", ranges "a-b", single values, and comma lists.
func ParseCron(expr string) (ParsedCron, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return ParsedCron{}, fmt.Errorf("cron must have 5 fields (minute hour day month weekday)")
	}
	specs := []struct{ min, max int }{
		{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7},
	}
	var out ParsedCron
	for i, part := range parts {
		f, err := parseField(part, specs[i].min, specs[i].max)
		if err != nil {
			return ParsedCron{}, fmt.Errorf("cron field %d: %w", i+1, err)
		}
		out[i] = f
	}
	// Vixie cron treats 7 as Sunday too.
	if out[4].values[7] {
		out[4].values[0] = true
		delete(out[4].values, 7)
	}
	return out, nil
}

func parseField(part string, min, max int) (field, error) {
	f := field{values: map[int]bool{}}
	for _, item := range strings.Split(part, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return f, fmt.Errorf("empty item")
		}
		step := 1
		base := item
		if idx := strings.IndexByte(item, '/'); idx >= 0 {
			base = item[:idx]
			s, err := strconv.Atoi(item[idx+1:])
			if err != nil || s < 1 {
				return f, fmt.Errorf("invalid step %q", item[idx+1:])
			}
			step = s
		}
		lo, hi := min, max
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			idx := strings.IndexByte(base, '-')
			v, err := strconv.Atoi(base[:idx])
			if err != nil {
				return f, fmt.Errorf("invalid value %q", base[:idx])
			}
			w, err := strconv.Atoi(base[idx+1:])
			if err != nil {
				return f, fmt.Errorf("invalid value %q", base[idx+1:])
			}
			lo, hi = v, w
		default:
			v, err := strconv.Atoi(base)
			if err != nil {
				return f, fmt.Errorf("invalid value %q", base)
			}
			lo, hi = v, v
		}
		if lo > hi || lo < min || hi > max {
			return f, fmt.Errorf("range %d-%d is outside %d-%d", lo, hi, min, max)
		}
		for v := lo; v <= hi; v += step {
			if v >= min && v <= max {
				f.values[v] = true
			}
		}
	}
	if len(f.values) == 0 {
		return f, fmt.Errorf("no valid values")
	}
	return f, nil
}

func (f field) matches(v int) bool {
	return f.values[v]
}

// Next returns the next activation time strictly after `after`, or an error when
// no match is found within five years.
func Next(expr string, after time.Time) (time.Time, error) {
	parsed, err := ParseCron(expr)
	if err != nil {
		return time.Time{}, err
	}
	dowFull := len(parsed[4].values) >= 7
	domFull := len(parsed[2].values) >= 31
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.AddDate(5, 0, 0)
	for t.Before(limit) {
		if parsed[0].matches(t.Minute()) &&
			parsed[1].matches(t.Hour()) &&
			parsed[3].matches(int(t.Month())) {
			domOK := parsed[2].matches(t.Day())
			dowOK := parsed[4].matches(int(t.Weekday()))
			guard := domOK && dowOK
			if !dowFull && !domFull {
				// Both day fields restricted: run when either matches.
				guard = domOK || dowOK
			}
			if guard {
				return t, nil
			}
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching time in the next 5 years for %q", expr)
}