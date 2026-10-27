package kimi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Opt-in compatibility check against the installed, pinned CLI. The CLI gets
// a fresh home, no inherited credentials, and a loopback mock provider.
func TestInstalledCLIRedirects(t *testing.T) {
	if os.Getenv("REMOTE_KIMI_SMOKE") != "1" {
		t.Skip("set REMOTE_KIMI_SMOKE=1 to exercise the installed Kimi CLI")
	}
	binary, err := exec.LookPath("kimi")
	if err != nil {
		t.Fatal(err)
	}
	realPath := os.Getenv("PATH")
	for _, validRedirect := range []bool{true, false} {
		name := "307 without Location"
		if validRedirect {
			name = "307 followed successfully"
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				if r.Method != http.MethodPost {
					t.Errorf("method = %s", r.Method)
				}
				if r.URL.Path == "/v1/chat/completions" {
					if validRedirect {
						w.Header().Set("Location", "/redirected/chat/completions")
					}
					w.WriteHeader(http.StatusTemporaryRedirect)
					return
				}
				if r.URL.Path != "/redirected/chat/completions" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"id\":\"smoke\",\"object\":\"chat.completion.chunk\",\"model\":\"smoke-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"SMOKE_OK\"},\"finish_reason\":null}]}\n\n"+
					"data: {\"id\":\"smoke\",\"object\":\"chat.completion.chunk\",\"model\":\"smoke-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			home := t.TempDir()
			if err := os.Mkdir(filepath.Join(home, "kimi"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "kimi", "config.toml"), []byte("telemetry = false\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("REMOTE_KIMI_SMOKE_BINARY", binary)
			t.Setenv("REMOTE_KIMI_SMOKE_PATH", realPath)
			t.Setenv("REMOTE_KIMI_SMOKE_HOME", home)
			t.Setenv("REMOTE_KIMI_SMOKE_URL", server.URL+"/v1")
			fakeCLI(t, `exec /usr/bin/env -i \
PATH="$REMOTE_KIMI_SMOKE_PATH" HOME="$REMOTE_KIMI_SMOKE_HOME" \
KIMI_CODE_HOME="$REMOTE_KIMI_SMOKE_HOME/kimi" NO_COLOR=1 \
KIMI_MODEL_NAME=smoke-model KIMI_MODEL_API_KEY=dummy-local-key \
KIMI_MODEL_BASE_URL="$REMOTE_KIMI_SMOKE_URL" KIMI_MODEL_PROVIDER_TYPE=openai \
KIMI_MODEL_CAPABILITIES=text_in KIMI_CODE_EXPERIMENTAL_SECONDARY_MODEL=0 \
"$REMOTE_KIMI_SMOKE_BINARY" "$@"`)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var completed bool
			var text, diagnostic string
			err := (&Provider{}).Run(ctx, agent.RunRequest{Cwd: home, Prompt: "Reply SMOKE_OK without tools."}, func(ev agent.Event) {
				switch ev.Type {
				case agent.EventAssistantTextDelta:
					text += ev.Text
				case agent.EventRunCompleted:
					completed = true
				case agent.EventRunFailed:
					diagnostic = ev.Message
				}
			})
			if validRedirect {
				if err != nil || !completed || text != "SMOKE_OK" || requests.Load() != 2 {
					t.Fatalf("err=%v completed=%t text=%q requests=%d diagnostic=%q", err, completed, text, requests.Load(), diagnostic)
				}
			} else if !errors.Is(err, agent.ErrRunFailed) || completed || !strings.Contains(diagnostic, "307") {
				t.Fatalf("err=%v completed=%t diagnostic=%q", err, completed, diagnostic)
			}
		})
	}
}
