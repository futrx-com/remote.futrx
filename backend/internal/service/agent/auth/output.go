package auth

import (
	"regexp"
	"strings"
)

// completionOutputLimit bounds the CLI output handed to completion resolvers.
// It is long enough to carry the CLI's final status line into an error
// message without echoing its whole banner.
const completionOutputLimit = 300

var terminalControlRE = regexp.MustCompile(
	// OSC sequences (hyperlinks, titles) end with BEL or ST.
	`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)` +
		// CSI sequences (colors, cursor movement).
		`|\x1b\[[0-9;?]*[ -/]*[@-~]` +
		// Remaining two-byte escapes.
		`|\x1b[@-Z\\-_]`,
)

// OutputTail returns the last limit bytes of CLI output with terminal control
// sequences removed and whitespace collapsed, so it can be quoted in a
// one-line error message.
func OutputTail(output string, limit int) string {
	output = terminalControlRE.ReplaceAllString(output, "")
	output = strings.Join(strings.Fields(output), " ")
	if limit <= 0 || len(output) <= limit {
		return output
	}
	tail := output[len(output)-limit:]
	// Do not start in the middle of a multi-byte rune.
	for len(tail) > 0 && tail[0]&0xC0 == 0x80 {
		tail = tail[1:]
	}
	return "..." + tail
}
