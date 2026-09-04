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
	if err := v.ValidateAPIKeyFormat(key); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.endpoint, nil)
	if err != nil {
		return ErrAPIKeyValidationUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)

	response, err := v.client.Do(request)
	if err != nil {
		return ErrAPIKeyValidationUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return agentauth.ErrAPIKeyRejected
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w (HTTP %d)", ErrAPIKeyValidationUnavailable, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxAPIKeyValidationResponseBytes+1))
	if err != nil || len(body) > maxAPIKeyValidationResponseBytes {
		return ErrAPIKeyValidationUnavailable
	}
	var tokenPlan struct {
		BaseResponse *struct {
			StatusCode int `json:"status_code"`
		} `json:"base_resp"`
		ModelRemains *[]json.RawMessage `json:"model_remains"`
	}
	if err := json.Unmarshal(body, &tokenPlan); err != nil || tokenPlan.BaseResponse == nil {
		return ErrAPIKeyValidationUnavailable
	}
	switch tokenPlan.BaseResponse.StatusCode {
	case 0:
		if tokenPlan.ModelRemains == nil {
			return ErrAPIKeyValidationUnavailable
		}
		return nil
	case 1004, 1008, 2049:
		return agentauth.ErrAPIKeyRejected
	default:
		return ErrAPIKeyValidationUnavailable
	}
}

var _ agentauth.APIKeyValidator = (*apiKeyValidator)(nil)
var _ agentauth.APIKeyFormatValidator = (*apiKeyValidator)(nil)
