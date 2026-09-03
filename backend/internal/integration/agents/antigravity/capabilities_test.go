package antigravity

import (
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestParseCapabilitiesFromCLIOutput(t *testing.T) {
	models := `Available models:
Gemini 3.5 Flash (High)
Gemini 3.5 Flash (Medium)
Gemini 3.1 Pro (High)
Claude Opus 4.6 (Thinking)
`
	help := `
  --effort Reasoning effort (low|medium|high)
  --mode Agent mode (accept-edits, plan)
`
	caps := parseCLIOutputCatalog(models, help)
	if len(caps.Models) != 5 || caps.Models[1].ID != "Gemini 3.5 Flash (High)" || caps.Models[4].ID != "Claude Opus 4.6 (Thinking)" {
		t.Fatalf("models = %+v", caps.Models)
	}
	if got := caps.Models[1].ReasoningEfforts; len(got) != 4 || got[3].Value != "high" {
		t.Fatalf("reasoning efforts = %+v", got)
	}
	if len(caps.Modes) != 2 || caps.Modes[0].Value != string(agent.RunModeDefault) || caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("modes = %+v", caps.Modes)
	}
}
