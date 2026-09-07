// Package humanize renders machine values in the form a person reads them in.
package humanize

import "fmt"

// Bytes formats a byte count with a binary unit: "512 B", "8.0 KiB", "2.5 GiB".
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
