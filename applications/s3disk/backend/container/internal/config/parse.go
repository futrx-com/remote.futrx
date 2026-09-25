package config

import (
	"fmt"
	"os/user"
	"strconv"
	"strings"
)

// How a person's spelling becomes a configured value, and back: the mount
// takes its bucket, sizes, modes and owners as text, from a flag, an fstab
// line or an environment variable, and the flag table renders its defaults
// through the same forms so a default is written once as a number.

// ParseS3URL accepts "s3://bucket/prefix", "bucket:prefix" or "bucket/prefix"
// and returns the bucket plus a normalised prefix ("" or "dir/subdir/").
func ParseS3URL(s string) (bucket, prefix string, err error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return "", "", fmt.Errorf("empty bucket specification")
	}
	if after, ok := strings.CutPrefix(raw, "s3://"); ok {
		raw = after
	} else if scheme, _, ok := strings.Cut(raw, "://"); ok {
		// Pasting the provider's endpoint here is the easy mistake to make, and
		// the old parser accepted it: "https://s3.example.com" split on the
		// first colon and became the bucket "https". That produced a mount that
		// failed later against the wrong service, with nothing pointing back at
		// the real cause.
		return "", "", fmt.Errorf(
			"%q is a %s:// URL, not a bucket: give the bucket as s3://bucket/prefix "+
				"(or just bucket/prefix), and pass the provider's endpoint separately "+
				"with --endpoint", s, scheme)
	}
	if raw == "" {
		return "", "", fmt.Errorf("empty bucket specification")
	}
	// "bucket:prefix" is the s3fs/goofys spelling.
	if i := strings.IndexAny(raw, ":/"); i >= 0 {
		bucket, prefix = raw[:i], raw[i+1:]
	} else {
		bucket = raw
	}
	if bucket == "" {
		return "", "", fmt.Errorf("missing bucket name in %q", s)
	}
	if strings.ContainsAny(bucket, "/ ") {
		return "", "", fmt.Errorf("invalid bucket name %q", bucket)
	}
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return bucket, prefix, nil
}

// ParseSize understands plain byte counts and K/M/G/T suffixes, in any of the
// spellings people actually type: "512M", "8G", "8Gi", "8GiB", "1024B".
func ParseSize(text string) (int64, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	// Trim the unit tail first ("GiB" -> "G"), so the multiplier letter is last.
	if n := len(s); n > 0 && (s[n-1] == 'b' || s[n-1] == 'B') {
		s = s[:n-1]
	}
	if n := len(s); n > 0 && (s[n-1] == 'i' || s[n-1] == 'I') {
		s = s[:n-1]
	}
	mult := int64(1)
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'k', 'K':
			mult, s = 1<<10, s[:n-1]
		case 'm', 'M':
			mult, s = 1<<20, s[:n-1]
		case 'g', 'G':
			mult, s = 1<<30, s[:n-1]
		case 't', 'T':
			mult, s = 1<<40, s[:n-1]
		}
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", text)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid size %q: must not be negative", text)
	}
	return int64(value * float64(mult)), nil
}

// FormatSize renders a byte count the way ParseSize reads one, using the
// largest unit that divides it exactly: 8<<30 is "8G". It exists so a default
// is written once, as a number, and the command line shows the same value
// rather than a second copy of it spelled as a string.
func FormatSize(n int64) string {
	for _, unit := range []struct {
		suffix string
		scale  int64
	}{{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}} {
		if n >= unit.scale && n%unit.scale == 0 {
			return strconv.FormatInt(n/unit.scale, 10) + unit.suffix
		}
	}
	return strconv.FormatInt(n, 10)
}

// ParseMode parses an octal permission string such as "0755".
func ParseMode(s string) (uint32, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q (expected octal, e.g. 0755)", s)
	}
	return uint32(n) & 07777, nil
}

// FormatMode renders permission bits the way ParseMode reads them.
func FormatMode(mode uint32) string { return fmt.Sprintf("%04o", mode&07777) }

// ParseOwner resolves a user or group name (or numeric id) to an id.
func ParseOwner(s string, group bool) (uint32, error) {
	if n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32); err == nil {
		return uint32(n), nil
	}
	if group {
		g, err := user.LookupGroup(s)
		if err != nil {
			return 0, err
		}
		n, _ := strconv.ParseUint(g.Gid, 10, 32)
		return uint32(n), nil
	}
	u, err := user.Lookup(s)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.ParseUint(u.Uid, 10, 32)
	return uint32(n), nil
}
