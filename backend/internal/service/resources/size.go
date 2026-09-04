package resources

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	kib = uint64(1024)
	mib = kib * 1024
	gib = mib * 1024
	tib = gib * 1024
)

// binarySuffixes is ordered longest-first so "MiB" never matches as "B".
var binarySuffixes = []struct {
	suffix string
	unit   uint64
}{
	{"TiB", tib},
	{"GiB", gib},
	{"MiB", mib},
	{"KiB", kib},
	{"TB", 1000 * 1000 * 1000 * 1000},
	{"GB", 1000 * 1000 * 1000},
	{"MB", 1000 * 1000},
	{"kB", 1000},
	{"B", 1},
}

// ParseSize converts an LXD byte-size literal to bytes. LXD accepts both the
// binary ("GiB") and decimal ("GB") families, so both are honored here; a bare
// number is bytes. An empty string is a valid zero so callers can treat
// "unset" and "no limit" identically.
func ParseSize(value string) (uint64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	for _, candidate := range binarySuffixes {
		if !strings.HasSuffix(trimmed, candidate.suffix) {
			continue
		}
		digits := strings.TrimSpace(strings.TrimSuffix(trimmed, candidate.suffix))
		amount, err := strconv.ParseFloat(digits, 64)
		if err != nil || amount < 0 || math.IsInf(amount, 0) {
			return 0, fmt.Errorf("%w: %q", ErrInvalidSettings, value)
		}
		return uint64(amount * float64(candidate.unit)), nil
	}
	amount, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidSettings, value)
	}
	return amount, nil
}

// FormatSize renders bytes back into the largest binary unit that divides them
// exactly, so a derived default reads as "3GiB" rather than "3072MiB". Values
// that divide no unit cleanly fall back to whole MiB.
func FormatSize(bytes uint64) string {
	if bytes == 0 {
		return ""
	}
	for _, candidate := range []struct {
		unit   uint64
		suffix string
	}{
		{tib, "TiB"},
		{gib, "GiB"},
		{mib, "MiB"},
		{kib, "KiB"},
	} {
		if bytes >= candidate.unit && bytes%candidate.unit == 0 {
			return strconv.FormatUint(bytes/candidate.unit, 10) + candidate.suffix
		}
	}
	if bytes >= mib {
		return strconv.FormatUint(bytes/mib, 10) + "MiB"
	}
	return strconv.FormatUint(bytes, 10) + "B"
}

// roundDownTo floors bytes onto a multiple of step, never below step itself.
func roundDownTo(bytes, step uint64) uint64 {
	if step == 0 || bytes < step {
		return bytes
	}
	return bytes - bytes%step
}
