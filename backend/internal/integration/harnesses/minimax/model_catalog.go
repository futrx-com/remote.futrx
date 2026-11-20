package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const maxModelCatalogResponseBytes = 1 << 20

var (
	ErrMiniMaxModelDiscoveryUnavailable = errors.New("MiniMax model discovery is temporarily unavailable")
	ErrMiniMaxModelUnavailable          = errors.New("selected MiniMax model is no longer available")
)

type modelCatalogSource interface {
	Models(context.Context, string) ([]string, error)
}

type modelCatalogClient struct {
	client   apiKeyValidationClient
	endpoint string
}

func newModelCatalogClient() *modelCatalogClient {
	return &modelCatalogClient{
		client: &http.Client{
			Timeout: configconstants.MiniMaxAPIValidationTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		endpoint: configconstants.MiniMaxModelsURL,
	}
}

func (c *modelCatalogClient) Models(ctx context.Context, key string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, ErrMiniMaxModelDiscoveryUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)

	response, err := c.client.Do(request)
	if err != nil {
		return nil, ErrMiniMaxModelDiscoveryUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: %w", ErrMiniMaxModelDiscoveryUnavailable, agentauth.ErrAPIKeyRejected)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w (HTTP %d)", ErrMiniMaxModelDiscoveryUnavailable, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxModelCatalogResponseBytes+1))
	if err != nil || len(body) > maxModelCatalogResponseBytes {
		return nil, ErrMiniMaxModelDiscoveryUnavailable
	}
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, ErrMiniMaxModelDiscoveryUnavailable
	}

	models := make([]string, 0, len(catalog.Data))
	seen := make(map[string]struct{}, len(catalog.Data))
	for _, item := range catalog.Data {
		id := agent.NormalizeModelID(item.ID)
		if !isMiniMaxLanguageModel(id) {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	if len(models) == 0 {
		return nil, ErrMiniMaxModelDiscoveryUnavailable
	}
	return models, nil
}

// MiniMax uses separate APIs for speech, image, video, and music generation.
// The coding agent can only execute language models from the MiniMax-M family.
// Matching the family keeps future M-series releases discoverable without a
// release-by-release model allowlist.
func isMiniMaxLanguageModel(id string) bool {
	return strings.HasPrefix(strings.ToLower(id), "minimax-m")
}

func resolveMiniMaxModel(models []string, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		if len(models) == 0 {
			return "", ErrMiniMaxModelDiscoveryUnavailable
		}
		return models[0], nil
	}
	requested = agent.NormalizeModelID(requested)
	if requested == "" {
		return "", ErrMiniMaxModelUnavailable
	}
	for _, model := range models {
		if model == requested {
			return model, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrMiniMaxModelUnavailable, requested)
}
