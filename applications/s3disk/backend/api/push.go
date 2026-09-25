package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	appLifecycle "futrx.local/catalog/applications/s3disk/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const (
	// A chat sends a handful of attachments; larger callers should paginate.
	maxPushNames = 32
	// The mount writes back synchronously unless --async-writeback is set.
	pushTimeout = 90 * time.Second
)

// The route accepts names only; both directories belong to this application.
type pushRequest struct {
	Names []string `json:"names"`
}

type pushResult struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Stored  bool   `json:"stored"`
	Removed bool   `json:"removed"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (b *backend) push(r applications.Request) applications.Response {
	var request pushRequest
	if err := json.Unmarshal(r.Body, &request); err != nil {
		return applications.Errorf(http.StatusBadRequest, "Malformed request body")
	}
	if len(request.Names) == 0 {
		return applications.Errorf(http.StatusBadRequest, "No file names were given")
	}
	if len(request.Names) > maxPushNames {
		return applications.Errorf(
			http.StatusBadRequest, "At most %d files can be pushed at once", maxPushNames)
	}

	ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
	defer cancel()
	batch, err := b.attachments.push(ctx, request.Names)
	if errors.Is(err, errNotMounted) {
		return applications.Errorf(http.StatusConflict, "Nothing is mounted at %s", b.target.mountpoint)
	}
	if err != nil {
		return applications.Errorf(http.StatusBadGateway, "%s", err)
	}

	results := make([]pushResult, 0, len(batch.Results))
	issues := 0
	for _, result := range batch.Results {
		if result.Error != "" {
			issues++
		}
		results = append(results, pushResult{
			Name: result.Name, Path: result.Path,
			Stored: result.Stored, Removed: result.Removed,
			Skipped: result.Skipped, Error: result.Error,
		})
	}
	response := map[string]any{
		"mountpoint": b.target.mountpoint,
		"directory":  batch.Directory,
		"stored":     batch.Stored,
		"removed":    batch.Removed,
		"results":    results,
	}
	if err := b.pushes.Completed(appLifecycle.PushOutcome{
		Directory: batch.Directory,
		Requested: len(batch.Results),
		Stored:    batch.Stored,
		Removed:   batch.Removed,
		Issues:    issues,
	}); err != nil {
		response["warning"] = fmt.Sprintf("event not published: %v", err)
	}
	return applications.JSON(http.StatusOK, response)
}
