package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

const modelResolutionWorkers = 4

type claudeModelCatalog struct {
	Source       agent.CapabilitySource
	DefaultLabel string
	Selections   []claudeModelSelection
}

type claudeModelSelection struct {
	ID          string
	Label       string
	Description string
}

type modelCommandResponse struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
}

func queryModelCatalog(
	ctx context.Context,
	req agent.CapabilityRequest,
) (claudeModelCatalog, bool, error) {
	result, err := runModelCommand(ctx, req, "")
	if err != nil {
		return claudeModelCatalog{}, false, err
	}
	defaultLabel, selectionIDs, err := parseModelCatalogResult(result)
	if err != nil {
		return claudeModelCatalog{}, false, err
	}

	selections := make([]claudeModelSelection, len(selectionIDs))
	resolved := make([]bool, len(selectionIDs))
	jobs := make(chan int)
	var wait sync.WaitGroup
	workerCount := min(modelResolutionWorkers, len(selectionIDs))
	for range workerCount {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				id := selectionIDs[index]
				label, resolveErr := resolveModelLabel(ctx, req, id)
				if resolveErr == nil {
					resolved[index] = true
				} else {
					label = fallbackModelLabel(id)
				}
				selections[index] = claudeModelSelection{
					ID:          id,
					Label:       label,
					Description: "Claude Code selection: " + id,
				}
			}
		}()
	}
	for index := range selectionIDs {
		jobs <- index
	}
	close(jobs)
	wait.Wait()

	labels := make(map[string]string, len(selections))
	fullyResolved := true
	for index, selection := range selections {
		labels[selection.ID] = selection.Label
		fullyResolved = fullyResolved && resolved[index]
	}
	for index := range selections {
		selections[index].Label = contextualModelLabel(selections[index].ID, selections[index].Label, labels)
	}

	return claudeModelCatalog{
		Source:       agent.CapabilitySourceLive,
		DefaultLabel: defaultLabel,
		Selections:   selections,
	}, fullyResolved, nil
}

func runModelCommand(
	ctx context.Context,
	req agent.CapabilityRequest,
	selection string,
) (string, error) {
	args := []string{"-p", "--no-session-persistence", "--output-format", "json"}
	if selection != "" {
		args = append(args, "--model", selection)
	}
	args = append(args, "/model")
	cmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "IS_SANDBOX=1"},
		"claude",
		args...,
	)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var response modelCommandResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("decode claude model command: %w", err)
	}
	if response.IsError {
		return "", errors.New(strings.TrimSpace(response.Result))
	}
	return response.Result, nil
}

func parseModelCatalogResult(result string) (string, []string, error) {
	defaultLabel := currentModelLabel(result)
	marker := "Available:"
	start := strings.Index(result, marker)
	if start < 0 {
		return "", nil, errors.New("claude model command did not list available selections")
	}
	available := result[start+len(marker):]
	if end := strings.Index(available, "or a full model ID"); end >= 0 {
		available = available[:end]
	}
	available = strings.Trim(available, " \t\r\n,.")
	parts := strings.Split(available, ",")
	seen := make(map[string]bool)
	selections := make([]string, 0, len(parts))
	for _, part := range parts {
		id := normalizeModelSelection(part)
		if id == "" || id == "default" || seen[id] {
			continue
		}
		seen[id] = true
		selections = append(selections, id)
	}
	if defaultLabel == "" || len(selections) == 0 {
		return "", nil, errors.New("claude model command returned an incomplete catalog")
	}
	return defaultLabel, selections, nil
}

func resolveModelLabel(ctx context.Context, req agent.CapabilityRequest, selection string) (string, error) {
	result, err := runModelCommand(ctx, req, selection)
	if err != nil {
		return "", err
	}
	label := currentModelLabel(result)
	if label == "" {
		return "", errors.New("claude model command did not resolve the selection")
	}
	return label, nil
}

func currentModelLabel(result string) string {
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Current model:") {
			continue
		}
		label := strings.TrimSpace(strings.TrimPrefix(line, "Current model:"))
		// Newer Claude Code versions wrap model labels in Markdown inline-code
		// backticks. Strip them on both sides of the default marker so either
		// `Opus 5 (default)` or `Opus 5` (default) stays presentation-neutral.
		label = strings.Trim(label, "`")
		label = strings.TrimSuffix(label, " (default)")
		return strings.Trim(strings.TrimSpace(label), "`")
	}
	return ""
}

func normalizeModelSelection(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "[1m]") {
		base := strings.TrimSuffix(value, "[1m]")
		if agent.NormalizeModelID(base) != "" {
			return base + "[1m]"
		}
		return ""
	}
	return agent.NormalizeModelID(value)
}

func contextualModelLabel(selection, resolved string, labels map[string]string) string {
	resolved = strings.TrimSpace(resolved)
	switch selection {
	case "best":
		return "Best · " + resolved
	case "opusplan":
		opus := labels["opus"]
		sonnet := labels["sonnet"]
		if opus != "" && sonnet != "" {
			return opus + " (Plan) · " + sonnet + " (Default)"
		}
	}
	if strings.HasSuffix(selection, "[1m]") && !strings.Contains(strings.ToLower(resolved), "1m") {
		return resolved + " (1M context)"
	}
	return resolved
}

func fallbackModelCatalog() claudeModelCatalog {
	ids := []string{
		"sonnet", "opus", "haiku", "fable", "best",
		"sonnet[1m]", "opus[1m]", "fable[1m]", "opusplan",
	}
	labels := make(map[string]string, len(ids))
	for _, id := range ids {
		labels[id] = fallbackModelLabel(id)
	}
	selections := make([]claudeModelSelection, 0, len(ids))
	for _, id := range ids {
		selections = append(selections, claudeModelSelection{
			ID:          id,
			Label:       contextualModelLabel(id, labels[id], labels),
			Description: "Claude Code selection: " + id,
		})
	}
	return claudeModelCatalog{
		Source:       agent.CapabilitySourceFallback,
		DefaultLabel: "Opus 5 (1M context)",
		Selections:   selections,
	}
}

func fallbackModelLabel(selection string) string {
	switch strings.TrimSuffix(selection, "[1m]") {
	case "best", "fable":
		return "Fable 5"
	case "opus", "opusplan":
		return "Opus 5"
	case "sonnet":
		return "Sonnet 5"
	case "haiku":
		return "Haiku 4.5"
	default:
		return selection
	}
}
