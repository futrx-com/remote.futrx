package s3fs

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// IsMounted reports whether path is currently an s3disk mountpoint.
func IsMounted(path string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		// mountinfo field 5 is the mountpoint, with octal escapes for spaces.
		if unescape(fields[4]) == abs {
			return true, nil
		}
	}
	return false, sc.Err()
}

func unescape(s string) string {
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(s)
}
