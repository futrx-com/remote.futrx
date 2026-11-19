package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const maxAPIKeyValidationResponseBytes = 1 << 20

var (
	ErrAPIKeyValidationUnavailable = errors.New("MiniMax Token Plan key validation is temporarily unavailable")
	ErrTokenPlanKeyRequired        = fmt.Errorf(
		"MiniMax requires a Token Plan subscription key with the %q prefix; pay-as-you-go API keys are not supported: %w",
		configconstants.MiniMaxTokenPlanKeyPrefix,
		agentauth.ErrAPIKeyRejected,
	)
)

type apiKeyValidationClient interface {
	Do(*http.Request) (*http.Response, error)
}

type apiKeyValidator struct {
	client   apiKeyValidationClient
	endpoint string
}

func newAPIKeyValidator() *apiKeyValidator {
	return &apiKeyValidator{
		client: &http.Client{
			Timeout: configconstants.MiniMaxAPIValidationTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		endpoint: configconstants.MiniMaxTokenPlanValidationURL,
	}
}

func (*apiKeyValidator) ValidateAPIKeyFormat(key string) error {
	if !strings.HasPrefix(key, configconstants.MiniMaxTokenPlanKeyPrefix) {
		return ErrTokenPlanKeyRequired
	}
	return nil
}

func (v *apiKeyValidator) ValidateAPIKey(ctx context.Context, key string) error {
	_, err := v.tokenPlan(ctx, key)
	return err
}

// tokenPlan returns the same authenticated response used to validate a key.
// The usage reader can inspect its quota rows without a second API request.
func (v *apiKeyValidator) tokenPlan(ctx context.Context, key string) ([]json.RawMessage, error) {
	if err := v.ValidateAPIKeyFormat(key); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.endpoint, nil)
	if err != nil {
		return nil, ErrAPIKeyValidationUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)

	response, err := v.client.Do(request)
	if err != nil {
		return nil, ErrAPIKeyValidationUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, agentauth.ErrAPIKeyRejected
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w (HTTP %d)", ErrAPIKeyValidationUnavailable, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxAPIKeyValidationResponseBytes+1))
	if err != nil || len(body) > maxAPIKeyValidationResponseBytes {
		return nil, ErrAPIKeyValidationUnavailable
	}
	var tokenPlan struct {
		BaseResponse *struct {
			StatusCode int `json:"status_code"`
		} `json:"base_resp"`
		ModelRemains *[]json.RawMessage `json:"model_remains"`
	}
	if err := json.Unmarshal(body, &tokenPlan); err != nil || tokenPlan.BaseResponse == nil {
		return nil, ErrAPIKeyValidationUnavailable
	}
	switch tokenPlan.BaseResponse.StatusCode {
	case 0:
		if tokenPlan.ModelRemains == nil {
			return nil, ErrAPIKeyValidationUnavailable
		}
		return *tokenPlan.ModelRemains, nil
	case 1004, 1008, 2049:
		return nil, agentauth.ErrAPIKeyRejected
	default:
		return nil, ErrAPIKeyValidationUnavailable
	}
}

var _ agentauth.APIKeyValidator = (*apiKeyValidator)(nil)
var _ agentauth.APIKeyFormatValidator = (*apiKeyValidator)(nil)
