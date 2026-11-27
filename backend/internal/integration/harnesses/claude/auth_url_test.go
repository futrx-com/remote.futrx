package claude

import (
	"slices"
	"strings"
	"testing"
)

func TestClaudeAuthURLStopsAtTerminalControls(t *testing.T) {
	const url = "https://claude.com/cai/oauth/authorize?code=true&client_id=abc&state=xyz"
	tests := map[string]string{
		"osc 8 hyperlink":    "If the browser didn't open, visit: \x1b]8;;" + url + "\x07" + url + "\x1b]8;;\x07\n",
		"osc 8 terminator":   "visit: " + url + "\x1b]8;;\x07\n",
		"plain url":          "visit: " + url + "\n",
		"url at end of read": "visit: " + url,
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			got := claudeAuthURLRE.FindString(output)
			if got != url {
				t.Fatalf("matched %q, want %q", got, url)
			}
			if strings.ContainsAny(got, "\x1b\x07") {
				t.Fatalf("match contains terminal control bytes: %q", got)
			}
		})
	}
}

func TestCodeAuthEnvRequestsFullSignInLink(t *testing.T) {
	auth := &Auth{}
	env := auth.codeAuthEnv([]string{"PATH=/usr/bin", "CLAUDE_CODE_FULL_SIGN_IN_LINK=0"})
	if !slices.Contains(env, "PATH=/usr/bin") {
		t.Fatalf("env dropped PATH: %v", env)
	}
	count := 0
	for _, value := range env {
		if strings.HasPrefix(value, "CLAUDE_CODE_FULL_SIGN_IN_LINK=") {
			count++
			if value != "CLAUDE_CODE_FULL_SIGN_IN_LINK=1" {
				t.Fatalf("env has %q, want CLAUDE_CODE_FULL_SIGN_IN_LINK=1", value)
			}
		}
	}
	if count != 1 {
		t.Fatalf("CLAUDE_CODE_FULL_SIGN_IN_LINK set %d times in %v", count, env)
	}
}
