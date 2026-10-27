// Package runtimeassets publishes provider-selected runtime assets inside
// project containers.
package runtimeassets

import (
	"context"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

// Adapter publishes the selected provider's non-secret runtime configuration.
// Content is declared by the provider profile and verified on every
// preparation because both the asset and marker live in a root-writable
// provider home.
type Adapter struct {
	runner    command.Runner
	publisher *assets.Publisher
}

// NewAdapter returns a runtime-asset adapter backed by shared container
// dependencies.
func NewAdapter(runner command.Runner, publisher *assets.Publisher) *Adapter {
	return &Adapter{runner: runner, publisher: publisher}
}

// Ensure publishes every selected asset to the project container.
func (a *Adapter) Ensure(
	ctx context.Context,
	containerName string,
	assetsToPublish []provisioning.RuntimeAsset,
) error {
	if len(assetsToPublish) == 0 {
		return nil
	}
	if !a.runner.Available() {
		return command.ErrUnavailable
	}

	dctx, cancel := context.WithTimeout(ctx, configconstants.RuntimeAssetEnsureTimeout)
	defer cancel()
	created := make(map[string]struct{}, len(assetsToPublish))
	for _, asset := range assetsToPublish {
		asset = asset.Resolved()
		if _, exists := created[asset.Directory]; !exists {
			out, err := a.runner.Run(dctx, "exec", containerName, "--",
				"install", "-d", "-m", asset.DirectoryMode, asset.Directory)
			if err != nil {
				return fmt.Errorf("mkdir %s: %w; output: %s", asset.Directory, err, out)
			}
			created[asset.Directory] = struct{}{}
		}

		if err := a.publisher.PushVerified(
			ctx,
			containerName,
			asset.Content,
			asset.HashPath,
			asset.Mode,
			asset.Path,
		); err != nil {
			return err
		}
	}
	return nil
}
