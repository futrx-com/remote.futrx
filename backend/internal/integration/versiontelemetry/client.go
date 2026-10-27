// Package versiontelemetry sends the application's minimal version report to
// remote.futrx.com.
package versiontelemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	serviceversiontelemetry "github.com/futrx-com/remote.futrx.com/internal/service/versiontelemetry"
)

const (
	endpointURL    = "https://remote.futrx.com/api/telemetry/version"
	requestTimeout = 3 * time.Second
)

var errRedirectRejected = errors.New("version telemetry redirect rejected")

var _ serviceversiontelemetry.Reporter = (*Client)(nil)

type Client struct {
	httpClient *http.Client
	endpoint   string
}

func New() *Client {
	return newClient(endpointURL, newHTTPClient(requestTimeout))
}

func newClient(endpoint string, httpClient *http.Client) *Client {
	return &Client{httpClient: httpClient, endpoint: endpoint}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirectRejected
		},
	}
}

func (c *Client) ReportVersion(
	ctx context.Context,
	report serviceversiontelemetry.Report,
) error {
	payload, err := json.Marshal(struct {
		InstallationID string `json:"installationId"`
		Version        string `json:"version"`
	}{InstallationID: report.InstallationID, Version: report.Version})
	if err != nil {
		return fmt.Errorf("encode version telemetry: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create version telemetry request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header["User-Agent"] = nil

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send version telemetry: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("send version telemetry: HTTP %d", response.StatusCode)
	}
	return nil
}
