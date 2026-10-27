package kimicodex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const maxModelCatalogResponseBytes = 1 << 20

var (
	ErrKimiCodexModelDiscoveryUnavailable = errors.New("Kimi Code model discovery is temporarily unavailable")
	ErrKimiCodexModelUnavailable          = errors.New("selected Kimi Code model is unavailable; refresh models and check your Kimi plan")
)

type modelInfo struct {
	ID            string
	Label         string
	ContextWindow int
	ImageInput    bool
	Efforts       []agent.ReasoningEffort
	DefaultEffort agent.ReasoningEffort
}

func (m modelInfo) reasoningEffort(requested agent.ReasoningEffort) agent.ReasoningEffort {
	if slices.Contains(m.Efforts, requested) {
		return requested
	}
	// Never disable thinking: K3 can silently route to a different model.
	return m.DefaultEffort
}

type modelCatalogSource interface {
	Models(context.Context, string) ([]modelInfo, error)
}

type modelCatalogClient struct {
	client   *http.Client
	endpoint string
}

func newModelCatalogClient() *modelCatalogClient {
	return &modelCatalogClient{
		client: &http.Client{
			Timeout: configconstants.KimiCodexAPIValidationTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		endpoint: configconstants.KimiCodexModelsURL,
	}
}

func (c *modelCatalogClient) ValidateAPIKey(ctx context.Context, key string) error {
	_, err := c.Models(ctx, key)
	return err
}

func (c *modelCatalogClient) Models(ctx context.Context, key string) ([]modelInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, ErrKimiCodexModelDiscoveryUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("User-Agent", "remote.futrx")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, ErrKimiCodexModelDiscoveryUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: %w", ErrKimiCodexModelDiscoveryUnavailable, agentauth.ErrAPIKeyRejected)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w (HTTP %d)", ErrKimiCodexModelDiscoveryUnavailable, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxModelCatalogResponseBytes+1))
	if err != nil || len(body) > maxModelCatalogResponseBytes {
		return nil, ErrKimiCodexModelDiscoveryUnavailable
	}
	return parseModels(body)
}

// Kimi's /models supplies account-specific context_length and think_efforts.
// Older responses may omit metadata; use a conservative 256K context then.
func parseModels(body []byte) ([]modelInfo, error) {
	var catalog struct {
		Data []struct {
			ID              string `json:"id"`
			DisplayName     string `json:"display_name"`
			ContextLength   int    `json:"context_length"`
			SupportsImageIn *bool  `json:"supports_image_in"`
			SupportsToolUse *bool  `json:"supports_tool_use"`
			ThinkEfforts    *struct {
				Support bool                    `json:"support"`
				Valid   []agent.ReasoningEffort `json:"valid_efforts"`
				Default agent.ReasoningEffort   `json:"default_effort"`
			} `json:"think_efforts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, ErrKimiCodexModelDiscoveryUnavailable
	}
	var models []modelInfo
	seen := make(map[string]bool)
	for _, item := range catalog.Data {
		id := agent.NormalizeModelID(item.ID)
		if !isCodingModel(id) || seen[id] || (item.SupportsToolUse != nil && !*item.SupportsToolUse) {
			continue
		}
		seen[id] = true
		model := modelInfo{ID: id, Label: item.DisplayName, ContextWindow: item.ContextLength,
			ImageInput: item.SupportsImageIn == nil || *item.SupportsImageIn,
			Efforts:    []agent.ReasoningEffort{"low", "high", "max"}, DefaultEffort: "high"}
		if model.Label == "" {
			model.Label = id
		}
		if model.ContextWindow <= 0 {
			model.ContextWindow = 262_144
		}
		if id == "k3-256k" && model.ContextWindow > 262_144 {
			model.ContextWindow = 262_144
		}
		if id == "kimi-for-coding" {
			model.DefaultEffort = "max"
		}
		if id == "kimi-for-coding-highspeed" {
			model.Efforts = []agent.ReasoningEffort{"high"}
		}
		if metadata := item.ThinkEfforts; metadata != nil && metadata.Support {
			var efforts []agent.ReasoningEffort
			for _, effort := range metadata.Valid {
				if slices.Contains([]agent.ReasoningEffort{"low", "high", "max"}, effort) && !slices.Contains(efforts, effort) {
					efforts = append(efforts, effort)
				}
			}
			if len(efforts) > 0 {
				model.Efforts = efforts
				model.DefaultEffort = efforts[0]
				if slices.Contains(efforts, metadata.Default) {
					model.DefaultEffort = metadata.Default
				}
			}
		}
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, ErrKimiCodexModelDiscoveryUnavailable
	}
	// Prefer the explicitly requested K3 generation with its economical 256K
	// variant. Never select kimi-for-coding while presenting it as K3.
	for _, preferred := range []string{"k3-256k", "k3"} {
		if index := slices.IndexFunc(models, func(m modelInfo) bool { return m.ID == preferred }); index >= 0 {
			models[0], models[index] = models[index], models[0]
			break
		}
	}
	return models, nil
}

func isCodingModel(id string) bool {
	return id == "k3" || strings.HasPrefix(id, "k3-") || strings.HasPrefix(id, "k3.") ||
		id == "kimi-for-coding" || strings.HasPrefix(id, "kimi-for-coding-")
}

func resolveModel(models []modelInfo, requested string) (modelInfo, error) {
	if strings.TrimSpace(requested) == "" && len(models) > 0 {
		return models[0], nil
	}
	requested = agent.NormalizeModelID(requested)
	for _, model := range models {
		if model.ID == requested {
			return model, nil
		}
	}
	return modelInfo{}, ErrKimiCodexModelUnavailable
}

var _ agentauth.APIKeyValidator = (*modelCatalogClient)(nil)
