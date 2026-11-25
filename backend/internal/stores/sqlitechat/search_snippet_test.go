package sqlitechat

import (
	"reflect"
	"testing"
	"unicode/utf16"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

const (
	mo = snippetMatchOpen
	mc = snippetMatchClose
)

func TestSearchSnippetStripsMarkdownAndKeepsHighlights(t *testing.T) {
	for _, tc := range []struct {
		name, marked, text string
		spans              []chat.TextSpan
	}{
		{
			name:   "emphasis and list cut mid-construct",
			marked: "… 3. **Run Nginx on a temporary port** and " + mo + "test" + mc + " it.\n4. **Switch …",
			text:   "… 3. Run Nginx on a temporary port and test it. 4. Switch …",
			spans:  []chat.TextSpan{{Start: 39, End: 43}},
		},
		{
			name:   "match inside emphasis, code, and a link",
			marked: "## Title\n- *" + mo + "test" + mc + "* `go " + mo + "test" + mc + "` [the " + mo + "test" + mc + " docs](https://x.y/test)",
			text:   "Title test go test the test docs",
			spans:  []chat.TextSpan{{Start: 6, End: 10}, {Start: 14, End: 18}, {Start: 23, End: 27}},
		},
		{
			name:   "fences, quotes, tables, and html",
			marked: "```bash\nmake " + mo + "test" + mc + "\n```\n> note <b>bold</b>\n| a | b |\n|---|---|",
			text:   "make test note bold a b",
			spans:  []chat.TextSpan{{Start: 5, End: 9}},
		},
		{
			name:   "identifiers keep inner underscores and stars",
			marked: "run_" + mo + "test" + mc + "_suite with 2*3",
			text:   "run_test_suite with 2*3",
			spans:  []chat.TextSpan{{Start: 4, End: 8}},
		},
		{
			name:   "offsets count UTF-16 units",
			marked: "🙂 é " + mo + "test" + mc,
			text:   "🙂 é test",
			spans:  []chat.TextSpan{{Start: 5, End: 9}},
		},
		{
			name:   "unclosed marker ends at the text end",
			marked: "a " + mo + "test",
			text:   "a test",
			spans:  []chat.TextSpan{{Start: 2, End: 6}},
		},
		{
			name:   "no match markers",
			marked: "plain",
			text:   "plain",
			spans:  []chat.TextSpan{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, spans := searchSnippet(tc.marked)
			if text != tc.text || !reflect.DeepEqual(spans, tc.spans) {
				t.Fatalf("got %q %+v, want %q %+v", text, spans, tc.text, tc.spans)
			}
			units := utf16.Encode([]rune(text))
			for _, span := range spans {
				if got := string(utf16.Decode(units[span.Start:span.End])); got != "test" {
					t.Fatalf("span %+v covers %q", span, got)
				}
			}
		})
	}
}

func TestSearchTranscriptReturnsPlainHighlightedSnippet(t *testing.T) {
	s := testStore(t)
	makeChat(t, s, "aaaa")
	appendEvents(t, s, "aaaa",
		chat.Event{Type: "user", TurnID: "one", Text: "How do I migrate?"},
		chat.Event{Type: "assistant_text", TurnID: "one", MessageID: "m", Text: "1. **Run Nginx** and `Test` it.\n2. **Switch** traffic."})
	hits, err := s.SearchTranscript(testContext, "aaaa", "test", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits %+v %v", hits, err)
	}
	want := chat.SearchHit{Text: "1. Run Nginx and Test it. 2. Switch traffic.", Highlights: []chat.TextSpan{{Start: 17, End: 21}}}
	if hits[0].Text != want.Text || !reflect.DeepEqual(hits[0].Highlights, want.Highlights) {
		t.Fatalf("got %q %+v, want %q %+v", hits[0].Text, hits[0].Highlights, want.Text, want.Highlights)
	}
}
