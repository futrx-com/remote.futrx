package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type cronSet struct {
	values   []bool
	wildcard bool
}

type parsedCron struct {
	minute     cronSet
	hour       cronSet
	dayOfMonth cronSet
	month      cronSet
	dayOfWeek  cronSet
}

// nextCron implements the traditional
// minute hour day-of-month month day-of-week form. Numeric lists, ranges and
// steps are supported. Day 0 and day 7 both mean Sunday.
func nextCron(expression string, after time.Time, location *time.Location) (time.Time, error) {
	parsed, err := parseCron(expression)
	if err != nil {
		return time.Time{}, err
	}

	candidate := after.In(location).Truncate(time.Minute).Add(time.Minute)
	// Eight years covers every Gregorian leap-day arrangement while keeping a
	// malformed/impossible expression bounded.
	limit := candidate.AddDate(8, 0, 0)
	for candidate.Before(limit) {
		if parsed.matches(candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("%w: expression has no occurrence in eight years", ErrInvalidCron)
}

func parseCron(expression string) (parsedCron, error) {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return parsedCron{}, fmt.Errorf("%w: expected 5 fields", ErrInvalidCron)
	}
	minute, err := parseCronSet(fields[0], 0, 59)
	if err != nil {
		return parsedCron{}, cronFieldError("minute", err)
	}
	hour, err := parseCronSet(fields[1], 0, 23)
	if err != nil {
		return parsedCron{}, cronFieldError("hour", err)
	}
	dayOfMonth, err := parseCronSet(fields[2], 1, 31)
	if err != nil {
		return parsedCron{}, cronFieldError("day of month", err)
	}
	month, err := parseCronSet(fields[3], 1, 12)
	if err != nil {
		return parsedCron{}, cronFieldError("month", err)
	}
	dayOfWeek, err := parseCronSet(fields[4], 0, 7)
	if err != nil {
		return parsedCron{}, cronFieldError("day of week", err)
	}
	// Internally Sunday is time.Sunday (0). Fold cron's alternate 7 into it.
	if dayOfWeek.values[7] {
		dayOfWeek.values[0] = true
	}
	dayOfWeek.values = dayOfWeek.values[:7]
	return parsedCron{
		minute:     minute,
		hour:       hour,
		dayOfMonth: dayOfMonth,
		month:      month,
		dayOfWeek:  dayOfWeek,
	}, nil
}

func cronFieldError(field string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrInvalidCron, field, err)
}

func parseCronSet(spec string, min, max int) (cronSet, error) {
	out := cronSet{values: make([]bool, max+1), wildcard: spec == "*"}
	if spec == "" {
		return cronSet{}, fmt.Errorf("empty field")
	}
	for _, item := range strings.Split(spec, ",") {
		if item == "" {
			return cronSet{}, fmt.Errorf("empty list item")
		}
		base, step, err := splitCronStep(item)
		if err != nil {
			return cronSet{}, err
		}
		start, end, err := cronRange(base, min, max)
		if err != nil {
			return cronSet{}, err
		}
		for value := start; value <= end; value += step {
			out.values[value] = true
		}
	}
	return out, nil
}

func splitCronStep(item string) (base string, step int, err error) {
	parts := strings.Split(item, "/")
	if len(parts) > 2 {
		return "", 0, fmt.Errorf("too many slashes in %q", item)
	}
	base = parts[0]
	step = 1
	if len(parts) == 2 {
		if base == "" || parts[1] == "" {
			return "", 0, fmt.Errorf("invalid step %q", item)
		}
		step, err = strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return "", 0, fmt.Errorf("invalid step %q", parts[1])
		}
	}
	return base, step, nil
}

func cronRange(base string, min, max int) (int, int, error) {
	if base == "*" {
		return min, max, nil
	}
	parts := strings.Split(base, "-")
	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid range %q", base)
	}
	start, err := parseCronNumber(parts[0], min, max)
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 1 {
		return start, start, nil
	}
	end, err := parseCronNumber(parts[1], min, max)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("descending range %q", base)
	}
	return start, end, nil
}

func parseCronNumber(raw string, min, max int) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%q is outside %d-%d", raw, min, max)
	}
	return value, nil
}

func (c parsedCron) matches(t time.Time) bool {
	if !c.minute.values[t.Minute()] ||
		!c.hour.values[t.Hour()] ||
		!c.month.values[int(t.Month())] {
		return false
	}
	dom := c.dayOfMonth.values[t.Day()]
	dow := c.dayOfWeek.values[int(t.Weekday())]
	switch {
	case c.dayOfMonth.wildcard && c.dayOfWeek.wildcard:
		return true
	case c.dayOfMonth.wildcard:
		return dow
	case c.dayOfWeek.wildcard:
		return dom
	default:
		// Traditional cron treats DOM and DOW as alternatives when both are
		// constrained.
		return dom || dow
	}
}
