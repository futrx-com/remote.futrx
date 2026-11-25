package sqlitechat

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// FTS5 wraps each matched phrase in these private-use runes, which survive
// Markdown stripping and are then turned into highlight offsets.
const (
	snippetMatchOpen  = ""
	snippetMatchClose = ""
)

var (
	snippetImage      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	snippetLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	snippetHTMLTag    = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9-]*(?:\s[^<>]*)?/?>`)
	snippetFence      = regexp.MustCompile("```[A-Za-z0-9_+-]*")
	snippetLineMarker = regexp.MustCompile(`(?m)^[ \t]*(?:#{1,6}[ \t]+|>[ \t]?|[-*+][ \t]+(?:\[[ xX]\][ \t]+)?|\|)`)
	snippetRule       = regexp.MustCompile(`(?m)^[ \t]*(?:[-*_][ \t]*){3,}$|\|?[ \t]*:?-{3,}:?[ \t]*\|?`)
	snippetDelimiters = strings.NewReplacer("**", "", "__", "", "~~", "", "`", "", "|", " ")
)

// searchSnippet turns an FTS5 snippet into display text: Markdown syntax is
// removed, whitespace collapsed, and the match markers become offsets.
func searchSnippet(marked string) (string, []chat.TextSpan) {
	text := snippetImage.ReplaceAllString(marked, "$1")
	text = snippetLink.ReplaceAllString(text, "$1")
	text = snippetHTMLTag.ReplaceAllString(text, "")
	text = snippetFence.ReplaceAllString(text, " ")
	text = snippetRule.ReplaceAllString(text, " ")
	text = snippetLineMarker.ReplaceAllString(text, "")
	text = snippetDelimiters.Replace(text)
	text = stripEmphasisMarks(text)
	text = strings.Join(strings.Fields(text), " ")
	return extractSnippetSpans(text)
}

// stripEmphasisMarks drops single `*`/`_` that open or close emphasis while
// keeping ones inside words, such as snake_case identifiers or 2*3.
func stripEmphasisMarks(text string) string {
	runes := []rune(text)
	// Match markers are invisible here, so `run_<match>test` stays one word.
	isWord := func(i, step int) bool {
		for ; i >= 0 && i < len(runes); i += step {
			if r := string(runes[i]); r != snippetMatchOpen && r != snippetMatchClose {
				return unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])
			}
		}
		return false
	}
	var b strings.Builder
	for i, r := range runes {
		if (r == '*' || r == '_') && !(isWord(i-1, -1) && isWord(i+1, 1)) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func extractSnippetSpans(marked string) (string, []chat.TextSpan) {
	var b strings.Builder
	spans := []chat.TextSpan{}
	offset, open := 0, -1
	for _, r := range marked {
		switch string(r) {
		case snippetMatchOpen:
			if open < 0 {
				open = offset
			}
		case snippetMatchClose:
			if open >= 0 && offset > open {
				spans = append(spans, chat.TextSpan{Start: open, End: offset})
			}
			open = -1
		default:
			b.WriteRune(r)
			offset += utf16.RuneLen(r)
		}
	}
	if open >= 0 && offset > open {
		spans = append(spans, chat.TextSpan{Start: open, End: offset})
	}
	return b.String(), spans
}
