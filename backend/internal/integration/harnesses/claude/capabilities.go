package claude

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

const fastServiceTier = "fast"

const ultracodeEffort = "ultracode"

var claudeModelVersionPattern = regexp.MustCompile(`(?i)\b(fable|opus|sonnet|haiku)\s+(\d+)(?:\.(\d+))?`)

type effortDiscoveryResult struct {
	options []harness.CapabilityOption
	err     error
}

func (p *Provider) Capabilities(ctx context.Context, req harness.CapabilityRequest) (harness.Capabilities, error) {
	// Effort and model commands are independent local CLI probes. Run them in
	// parallel so detecting Ultracode does not add another startup delay to the
	// first uncached catalog request.
	effortDone := make(chan effortDiscoveryResult, 1)
	go func() {
		options, err := queryEffortOptions(ctx, req)
		effortDone <- effortDiscoveryResult{options: options, err: err}
	}()
	catalog, fullyResolved, catalogErr := queryModelCatalog(ctx, req)
	effortResult := <-effortDone
	reasoning, effortErr := effortResult.options, effortResult.err
	if effortErr != nil || len(reasoning) == 0 {
		reasoning = queryHelpEffortOptions(ctx, req)
	}
	if catalogErr != nil {
		caps := buildCapabilities(fallbackModelCatalog(), reasoning)
		caps.Warning = "Claude model catalog could not be read from the CLI"
		return caps, fmt.Errorf("claude capability discovery: %w", catalogErr)
	}

	caps := buildCapabilities(catalog, reasoning)
	if effortErr != nil || len(reasoning) == 0 {
		caps.Warning = "Claude effort levels could not be read from the CLI"
	}
	if !fullyResolved {
		caps.Warning = "Some Claude model versions could not be resolved by the CLI"
	}
	return caps, nil
}

func queryHelpEffortOptions(ctx context.Context, req harness.CapabilityRequest) []harness.CapabilityOption {
	helpCmd := harnessruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "IS_SANDBOX=1"},
		"claude",
		"--help",
	)
	helpOutput, _ := helpCmd.CombinedOutput()
	return parseHelpEfforts(string(helpOutput))
}

func fallbackCapabilities() harness.Capabilities {
	return buildCapabilities(fallbackModelCatalog(), fallbackReasoningOptions())
}

func buildCapabilities(catalog claudeModelCatalog, reasoning []harness.CapabilityOption) harness.Capabilities {
	if len(reasoning) == 0 {
		reasoning = fallbackReasoningOptions()
	}
	models := make([]harness.ModelCapability, 0, len(catalog.Selections)+1)
	models = append(models, harness.ModelCapability{
		ID:               "",
		Label:            autoModelLabel(catalog.DefaultLabel),
		Description:      "Use the model selected by Claude Code for this account",
		ReasoningEfforts: reasoningOptionsForModel(reasoning, catalog.DefaultLabel),
		ServiceTiers:     fastModeOptions(),
	})
	for _, selection := range catalog.Selections {
		model := harness.ModelCapability{
			ID:               selection.ID,
			Label:            selection.Label,
			Description:      selection.Description,
			ReasoningEfforts: reasoningOptionsForModel(reasoning, selection.Label),
		}
		if supportsFastMode(selection.ID) {
			model.ServiceTiers = fastModeOptions()
		}
		models = append(models, model)
	}
	return harness.Capabilities{
		Provider:    harness.ProviderClaude,
		Label:       "Claude",
		Source:      catalog.Source,
		Models:      models,
		Modes:       harness.ProviderModes(true),
		DefaultMode: harness.RunModeDefault,
	}
}

// reasoningOptionsForModel turns Claude Code's provider-wide /effort choices
// (or the --help fallback) into per-model choices. Ultracode is a session
// setting layered on xhigh rather than an API effort level, so it is offered
// only when the resolved model supports xhigh.
func reasoningOptionsForModel(
	options []harness.CapabilityOption,
	modelLabel string,
) []harness.CapabilityOption {
	supportsEffort, supportsXHigh, knownModel := claudeModelEffortSupport(modelLabel)
	if knownModel && !supportsEffort {
		return []harness.CapabilityOption{}
	}

	result := make([]harness.CapabilityOption, 0, len(options)+1)
	hasXHigh := false
	hasUltracode := false
	for _, option := range options {
		switch option.Value {
		case ultracodeEffort:
			hasUltracode = true
			// Re-add it below only for a compatible model. This also prevents a
			// future provider-wide response from applying it to every model.
			continue
		case "xhigh":
			if knownModel && !supportsXHigh {
				continue
			}
			hasXHigh = true
		}
		result = append(result, option)
	}
	if supportsXHigh && hasXHigh && hasUltracode {
		result = append(result, harness.CapabilityOption{
			Value:       ultracodeEffort,
			Label:       "Ultracode",
			Description: "XHigh effort with automatic dynamic-workflow orchestration (session only)",
		})
	}
	return result
}

func claudeModelEffortSupport(label string) (supportsEffort, supportsXHigh, known bool) {
	matches := claudeModelVersionPattern.FindAllStringSubmatch(label, -1)
	for _, match := range matches {
		major, majorErr := strconv.Atoi(match[2])
		minor := 0
		var minorErr error
		if match[3] != "" {
			minor, minorErr = strconv.Atoi(match[3])
		}
		if majorErr != nil || minorErr != nil {
			continue
		}

		modelEffort, modelXHigh := modelVersionEffortSupport(
			strings.ToLower(match[1]),
			major,
			minor,
		)
		if !known {
			supportsEffort = modelEffort
			supportsXHigh = modelXHigh
			known = true
			continue
		}
		// Composite selections such as opusplan must be compatible across
		// every model Claude Code may use for that selection.
		supportsEffort = supportsEffort && modelEffort
		supportsXHigh = supportsXHigh && modelXHigh
	}
	return supportsEffort, supportsXHigh, known
}

func modelVersionEffortSupport(family string, major, minor int) (bool, bool) {
	switch family {
	case "fable":
		return major >= 5, major >= 5
	case "opus":
		if major >= 5 {
			return true, true
		}
		if major == 4 && minor >= 7 {
			return true, true
		}
		return major == 4 && minor == 6, false
	case "sonnet":
		if major >= 5 {
			return true, true
		}
		return major == 4 && minor == 6, false
	case "haiku":
		return false, false
	default:
		return false, false
	}
}

func fallbackReasoningOptions() []harness.CapabilityOption {
	reasoning := []harness.CapabilityOption{harness.AutoOption()}
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		reasoning = append(reasoning, harness.CapabilityOption{Value: effort, Label: optionLabel(effort)})
	}
	return reasoning
}

func fastModeOptions() []harness.CapabilityOption {
	return []harness.CapabilityOption{
		harness.AutoOption(),
		{
			Value:       fastServiceTier,
			Label:       "Fast",
			Description: "Use Claude Fast mode for lower latency at a higher token cost",
		},
	}
}

func supportsFastMode(model string) bool {
	model = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(model)), "[1m]")
	return model == "opus"
}

func autoModelLabel(resolved string) string {
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return "Auto"
	}
	return "Auto · " + resolved
}
