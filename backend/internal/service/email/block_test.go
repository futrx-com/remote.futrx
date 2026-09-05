package email

import (
	"errors"
	"strings"
	"testing"
)

func TestBlocksEscapeHTML(t *testing.T) {
	payload := `<script>alert('x')</script>`
	blocks := map[string]block{
		"heading": headingBlock{value: payload},
		"text":    textBlock{value: payload},
		"button":  buttonBlock{label: payload, target: "https://example.com/" + payload},
		"list":    listBlock{items: []string{payload}},
		"kv":      kvBlock{pairs: [][2]string{{payload, payload}}},
		"code":    codeBlock{value: payload},
		"note":    noteBlock{value: payload},
	}
	for name, b := range blocks {
		t.Run(name, func(t *testing.T) {
			out := b.html()
			if strings.Contains(out, "<script>") {
				t.Errorf("%s html contains an unescaped <script>: %s", name, out)
			}
			if !strings.Contains(out, "&lt;script&gt;") {
				t.Errorf("%s html does not contain the escaped payload: %s", name, out)
			}
		})
	}
}

func TestBlocksCarryTheSameContentAsText(t *testing.T) {
	cases := []struct {
		name  string
		block block
		want  []string
	}{
		{"heading", headingBlock{value: "Run finished"}, []string{"Run finished"}},
		{"text", textBlock{value: "The run you started has finished."}, []string{"The run you started has finished."}},
		{"button", buttonBlock{label: "Open the run", target: "https://example.com/runs/1"}, []string{"Open the run", "https://example.com/runs/1"}},
		{"list", listBlock{items: []string{"first", "second"}}, []string{"- first", "- second"}},
		{"kv", kvBlock{pairs: [][2]string{{"Project", "remote"}}}, []string{"Project: remote"}},
		{"code", codeBlock{value: "482913"}, []string{"482913"}},
		{"note", noteBlock{value: "This code expires in 10 minutes."}, []string{"This code expires in 10 minutes."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.block.text()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("text() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestDividerHasNoText(t *testing.T) {
	if got := (dividerBlock{}).text(); got != "" {
		t.Errorf("divider text() = %q, want empty so it is skipped in the plain-text part", got)
	}
	if !strings.Contains(dividerBlock{}.html(), "background-color:#e5e7eb") {
		t.Error("divider html() does not draw a rule")
	}
}

func TestSafeURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"https", "https://example.com/reset?token=abc", false},
		{"http", "http://localhost:8080/runs/1", false},
		{"javascript", "javascript:alert(1)", true},
		{"data", "data:text/html,<script>alert(1)</script>", true},
		{"relative", "/runs/1", true},
		{"scheme only", "https://", true},
		{"empty", "   ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safeURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("safeURL(%q) = %q, want an error", tc.in, got)
				}
				if !errors.Is(err, ErrIncompleteMail) {
					t.Errorf("err = %v, want ErrIncompleteMail", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeURL(%q) returned %v, want no error", tc.in, err)
			}
			if got != tc.in {
				t.Errorf("safeURL(%q) = %q, want it returned unchanged", tc.in, got)
			}
		})
	}
}

func TestRenderBlocksSpacesAllButTheLast(t *testing.T) {
	htmlBody, textBody := renderBlocks([]block{
		headingBlock{value: "One"},
		textBlock{value: "Two"},
	})
	if got := strings.Count(htmlBody, blockGap); got != 1 {
		t.Errorf("block gaps = %d, want 1 (every block but the last)", got)
	}
	if want := "One\n\nTwo"; !strings.Contains(textBody, want) {
		t.Errorf("text body = %q, want it to contain %q", textBody, want)
	}
	if !strings.Contains(textBody, textFooter) {
		t.Errorf("text body = %q, want it to carry the footer attribution", textBody)
	}
	if !strings.Contains(htmlBody, "Sent by Remote") {
		t.Error("html body is not wrapped in the branded shell")
	}
}
