package kimicodex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in because CI only needs Go, not a globally installed Codex. This runs
// the actual pinned harness against a local Responses fixture, never Kimi.
func TestInstalledCodexWithKimiCatalogAndResponses(t *testing.T) {
	binary := os.Getenv("KIMI_CODEX_TEST_BINARY")
	if binary == "" {
		t.Skip("set KIMI_CODEX_TEST_BINARY to test the installed Codex harness")
	}
	models, err := parseModels([]byte(catalogFixture))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := buildRuntimeModelCatalog(models)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, catalog, 0600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	var sawToolOutput atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Model     string `json:"model"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
		}
		if request.Model != "k3-256k" || request.Reasoning.Effort != "max" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("Codex did not send the selected Kimi model, effort, and credential")
		}
		index := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(event any) { data, _ := json.Marshal(event); fmt.Fprintf(w, "data: %s\n\n", data) }
		response := map[string]any{"id": fmt.Sprintf("resp_%d", index), "object": "response", "status": "in_progress", "output": []any{}}
		write(map[string]any{"type": "response.created", "response": response})
		var item map[string]any
		if index == 1 {
			item = map[string]any{"id": "fc_1", "type": "function_call", "name": "shell_command", "call_id": "call_1", "arguments": `{"command":"printf kimi-tool-ok","timeout_ms":1000}`}
		} else {
			if strings.Contains(string(body), "kimi-tool-ok") && strings.Contains(string(body), "function_call_output") {
				sawToolOutput.Store(true)
			}
			item = map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "Kimi mock integration passed", "annotations": []any{}}}}
		}
		write(map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
		write(map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item})
		response["status"] = "completed"
		response["output"] = []any{item}
		response["usage"] = map[string]any{"input_tokens": 20, "output_tokens": 10, "total_tokens": 30}
		write(map[string]any{"type": "response.completed", "response": response})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"exec", "--json", "--ephemeral", "--skip-git-repo-check", "--sandbox", "danger-full-access"}
	for _, config := range kimiCodexConfigArgs("k3-256k", 262144) {
		args = append(args, config)
	}
	args = append(args, "-c", "model_catalog_json="+strconv.Quote(path), "-c", "model_providers.kimi-codex.base_url="+strconv.Quote(server.URL), "-c", `model_reasoning_effort="max"`, "-c", `web_search="disabled"`, "Run the fixture tool then report its result.")
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "CODEX_HOME=" + dir, "KIMI_API_KEY=fixture-key"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Codex failed: %v\n%s", err, out)
	}
	if requests.Load() != 2 || !sawToolOutput.Load() || !strings.Contains(string(out), "Kimi mock integration passed") {
		t.Fatalf("requests=%d toolOutput=%v\n%s", requests.Load(), sawToolOutput.Load(), out)
	}
}
