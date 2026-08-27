package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func queryAppServerCapabilities(
	ctx context.Context,
	req agent.CapabilityRequest,
) (modelListResponse, collaborationModeListResponse, error) {
	cmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "CODEX_HOME=/root/.codex", "OPENAI_API_KEY="},
		"codex",
		"app-server",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return modelListResponse{}, collaborationModeListResponse{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return modelListResponse{}, collaborationModeListResponse{}, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return modelListResponse{}, collaborationModeListResponse{}, err
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	return readAppServerCapabilities(stdin, stdout)
}

func readAppServerCapabilities(
	stdin io.Writer,
	stdout io.Reader,
) (modelListResponse, collaborationModeListResponse, error) {
	writeRPC := func(message any) error {
		return json.NewEncoder(stdin).Encode(message)
	}
	if err := writeRPC(map[string]any{
		"method": "initialize",
		"id":     1,
		"params": map[string]any{
			"clientInfo": map[string]string{
				"name":    "remote-futrx",
				"title":   "Remote",
				"version": "1",
			},
			"capabilities": map[string]bool{"experimentalApi": true},
		},
	}); err != nil {
		return modelListResponse{}, collaborationModeListResponse{}, err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	initialized := false
	var modelResult modelListResponse
	var modeResult collaborationModeListResponse
	modelRequestID := 2
	const modeRequestID = 3
	modelsDone := false
	modesDone := false
	for scanner.Scan() {
		var response rpcResponse
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil || response.ID == 0 {
			continue
		}
		if response.ID == 1 {
			if response.Error != nil {
				return modelListResponse{}, collaborationModeListResponse{}, fmt.Errorf("initialize: %s", response.Error.Message)
			}
			if initialized {
				continue
			}
			initialized = true
			if err := writeRPC(map[string]any{
				"method": "model/list",
				"id":     modelRequestID,
				"params": map[string]any{"includeHidden": false, "limit": 100},
			}); err != nil {
				return modelListResponse{}, collaborationModeListResponse{}, err
			}
			if err := writeRPC(map[string]any{
				"method": "collaborationMode/list",
				"id":     modeRequestID,
				"params": map[string]any{},
			}); err != nil {
				return modelListResponse{}, collaborationModeListResponse{}, err
			}
		} else if response.ID == modelRequestID {
			if response.Error != nil {
				return modelListResponse{}, collaborationModeListResponse{}, fmt.Errorf("model/list: %s", response.Error.Message)
			}
			var page modelListResponse
			if err := json.Unmarshal(response.Result, &page); err != nil {
				return modelListResponse{}, collaborationModeListResponse{}, fmt.Errorf("decode model/list: %w", err)
			}
			modelResult.Data = append(modelResult.Data, page.Data...)
			if page.NextCursor != "" {
				modelRequestID++
				if modelRequestID == modeRequestID {
					modelRequestID++
				}
				if err := writeRPC(map[string]any{
					"method": "model/list",
					"id":     modelRequestID,
					"params": map[string]any{
						"includeHidden": false,
						"limit":         100,
						"cursor":        page.NextCursor,
					},
				}); err != nil {
					return modelListResponse{}, collaborationModeListResponse{}, err
				}
			} else {
				modelsDone = true
			}
		} else if response.ID == modeRequestID {
			modesDone = true
			if response.Error == nil {
				_ = json.Unmarshal(response.Result, &modeResult)
			}
		}
		if modelsDone && modesDone {
			if len(modelResult.Data) == 0 {
				return modelListResponse{}, collaborationModeListResponse{}, errors.New("model/list returned no models")
			}
			return modelResult, modeResult, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return modelListResponse{}, collaborationModeListResponse{}, err
	}
	return modelListResponse{}, collaborationModeListResponse{}, errors.New("codex app-server closed before returning capabilities")
}
