package prompt

import (
	"strings"
	"unicode/utf8"
)

const notificationSummaryOpen = "<notification_summary>"
const notificationSummaryClose = "</notification_summary>"
const maxNotificationSummaryBytes = 240

// notificationSummaryFilter removes a private trailer before text reaches the
// event log or live clients. pending holds only a possible marker prefix.
type notificationSummaryFilter struct {
	pending string
	summary string
	inside  bool
	closed  bool
}

func (f *notificationSummaryFilter) text(chunk string) string {
	var visible strings.Builder
	f.pending += chunk
	for f.pending != "" {
		marker := notificationSummaryOpen
		if f.inside {
			marker = notificationSummaryClose
		}
		if at := strings.Index(f.pending, marker); at >= 0 {
			if f.inside {
				f.appendSummary(f.pending[:at])
				f.closed = true
			} else {
				visible.WriteString(f.pending[:at])
			}
			f.pending = f.pending[at+len(marker):]
			f.inside = !f.inside
			continue
		}
		keep := markerSuffixLength(f.pending, marker)
		part := f.pending[:len(f.pending)-keep]
		if f.inside {
			f.appendSummary(part)
		} else {
			visible.WriteString(part)
		}
		f.pending = f.pending[len(f.pending)-keep:]
		break
	}
	return visible.String()
}

func (f *notificationSummaryFilter) appendSummary(part string) {
	if len(f.summary) >= maxNotificationSummaryBytes {
		return
	}
	f.summary += part
	if len(f.summary) > maxNotificationSummaryBytes {
		f.summary = f.summary[:maxNotificationSummaryBytes]
	}
}

func (f *notificationSummaryFilter) finish() (visible, summary string) {
	// A partial opening marker or an unclosed summary is private too.
	if !f.inside && f.pending != "" && !strings.HasPrefix(notificationSummaryOpen, f.pending) {
		visible = f.pending
	}
	if f.closed {
		summary = strings.Join(strings.Fields(f.summary), " ")
		for !utf8.ValidString(summary) {
			summary = summary[:len(summary)-1]
		}
	}
	f.pending = ""
	return visible, summary
}

func markerSuffixLength(text, marker string) int {
	limit := min(len(text), len(marker)-1)
	for n := limit; n > 0; n-- {
		if strings.HasSuffix(text, marker[:n]) {
			return n
		}
	}
	return 0
}
