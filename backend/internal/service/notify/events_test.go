package notify

import (
	"errors"
	"strings"
	"testing"
)

func TestNeedsAttentionTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{name: "claude question tool", toolName: "AskUserQuestion", want: true},
		{name: "snake case spelling", toolName: "ask_user_question", want: true},
		{name: "plan approval", toolName: "ExitPlanMode", want: true},
		{name: "kebab case permission request", toolName: "request-permission", want: true},
		{name: "namespaced mcp tool", toolName: "mcp/remote/exit_plan_mode", want: true},
		{name: "surrounding whitespace", toolName: "  ExitPlanMode  ", want: true},
		{name: "ordinary file edit", toolName: "Edit", want: false},
		{name: "shell tool", toolName: "Bash", want: false},
		{name: "empty name", toolName: "", want: false},
		{name: "similar but unrelated", toolName: "AskAI", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NeedsAttentionTool(test.toolName); got != test.want {
				t.Fatalf("NeedsAttentionTool(%q) = %t, want %t", test.toolName, got, test.want)
			}
		})
	}
}

func TestRunKind(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		cancelled  bool
		wantKind   Kind
		wantStatus string
	}{
		{name: "clean run finishes", wantKind: KindRunFinished, wantStatus: StatusFinished},
		{name: "error fails", err: errors.New("boom"), wantKind: KindRunFailed, wantStatus: StatusFailed},
		{name: "cancellation reports as failed", cancelled: true, wantKind: KindRunFailed, wantStatus: StatusCancelled},
		{
			name:       "cancellation wins over the error",
			err:        errors.New("context canceled"),
			cancelled:  true,
			wantKind:   KindRunFailed,
			wantStatus: StatusCancelled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, status := RunKind(test.err, test.cancelled)
			if kind != test.wantKind || status != test.wantStatus {
				t.Fatalf("RunKind = (%q, %q), want (%q, %q)", kind, status, test.wantKind, test.wantStatus)
			}
		})
	}
}

func TestChatURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		chatID  string
		want    string
	}{
		{name: "chat deep link", baseURL: "https://remote.example.com", chatID: "abc123", want: "https://remote.example.com/?chat=abc123"},
		{name: "trailing slash is normalized", baseURL: "https://remote.example.com/", chatID: "abc123", want: "https://remote.example.com/?chat=abc123"},
		{name: "no chat falls back to the root", baseURL: "https://remote.example.com", chatID: "", want: "https://remote.example.com/"},
		{name: "no base URL yields no link", baseURL: "", chatID: "abc123", want: ""},
		{name: "chat ids are escaped", baseURL: "https://remote.example.com", chatID: "a b&c", want: "https://remote.example.com/?chat=a+b%26c"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ChatURL(test.baseURL, test.chatID); got != test.want {
				t.Fatalf("ChatURL(%q, %q) = %q, want %q", test.baseURL, test.chatID, got, test.want)
			}
		})
	}
}

func TestSummaryCollapsesBlankLinesAndTruncates(t *testing.T) {
	got := Summary("  first line \n\n\n   \nsecond line\n\n")
	if got != "first line\nsecond line" {
		t.Fatalf("Summary = %q", got)
	}

	long := Summary(strings.Repeat("x", summaryLimit+50))
	if runes := []rune(long); len(runes) != summaryLimit {
		t.Fatalf("truncated summary length = %d, want %d", len(runes), summaryLimit)
	}
	if !strings.HasSuffix(long, "…") {
		t.Fatalf("truncated summary should end with an ellipsis: %q", long[len(long)-5:])
	}
}

func TestTelegramMessageEscapesAgentOutput(t *testing.T) {
	message := TelegramMessage(Event{
		Event:       KindRunFinished,
		Status:      StatusFinished,
		ProjectName: "Ops & Tools",
		ChatTitle:   "<script>alert(1)</script>",
		Provider:    "claude",
		Summary:     "Wrote <b>index.html</b> & ran the tests",
		URL:         "https://remote.example.com/?chat=abc123",
	})

	for _, unwanted := range []string{"<script>", "<b>index.html</b>", "Ops & Tools"} {
		if strings.Contains(message, unwanted) {
			t.Fatalf("message leaked unescaped %q:\n%s", unwanted, message)
		}
	}
	for _, wanted := range []string{
		"&lt;script&gt;",
		"Ops &amp; Tools",
		"<b>Agent:</b> claude",
		`<a href="https://remote.example.com/?chat=abc123">Open in Remote</a>`,
	} {
		if !strings.Contains(message, wanted) {
			t.Fatalf("message missing %q:\n%s", wanted, message)
		}
	}
}
