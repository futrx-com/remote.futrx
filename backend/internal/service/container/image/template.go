package image

// Template images are the optional FAST PATH for project templates. The
// default (slow) path launches a project from the shared base image and runs
// the template's provision script inside the new container, which costs the
// user several minutes on first start. Publishing a dedicated image once —
// ideally on a beefier machine — moves that cost to build time.
//
// The build is deliberately layered: the builder container starts from the
// already-published base image, so a template image is base + one script and
// never repeats the base recipe.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

const (
	// templateImageBuilderName is the disposable builder used for template
	// images. Kept stable so a retry cleans up a leftover from a failed run.
	templateImageBuilderName = "futrx-remote-template-builder"

	templateImageBuildStageCount = 4
)

// TemplateSpec describes one layered template image build.
type TemplateSpec struct {
	// Name is the template name, used for progress text and the description.
	Name string
	// Alias is the image alias to publish, e.g. futrx-remote-wordpress-base.
	Alias string
	// BaseAlias is the image the builder container starts from. Empty means
	// the shared base image alias.
	BaseAlias string
	// Program is the complete provision program (harness included) to run
	// inside the builder.
	Program string
	// Description is the published image's description.
	Description string
}

// BuildTemplate publishes a dedicated image for one project template by
// launching the base image, running the template's provision program, and
// publishing the result. It uses the same staged progress reporting and the
// same overridable timeouts as the base-image build.
func (b *Builder) BuildTemplate(ctx context.Context, spec TemplateSpec) error {
	if !b.runtime.Available() {
		return errors.New("lxc CLI not found on PATH - install LXD on the host first")
	}
	if spec.Name == "" || spec.Alias == "" {
		return errors.New("template image build needs a template name and an alias")
	}
	if spec.Program == "" {
		return fmt.Errorf("template %q has no provisioning to bake into an image", spec.Name)
	}
	base := spec.BaseAlias
	if base == "" {
		base = Alias
	}
	description := spec.Description
	if description == "" {
		description = "futrx remote " + spec.Name + " template: " + base + " + " + spec.Name + " stack"
	}

	// Clean up any leftover builder from a previous interrupted run.
	cleanCtx, cleanCancel := context.WithTimeout(ctx, deleteTimeout)
	_, _ = b.runtime.DeleteContainer(cleanCtx, templateImageBuilderName)
	cleanCancel()

	bctx, bcancel := context.WithTimeout(ctx, b.buildTimeout)
	defer bcancel()

	// Cleanup outlives a canceled caller so an interrupted build does not
	// strand its disposable container.
	defer func() {
		dctx, dcancel := context.WithTimeout(context.Background(), deleteTimeout)
		defer dcancel()
		_, _ = b.runtime.DeleteContainer(dctx, templateImageBuilderName)
	}()

	out, err := b.runBuildStage(
		1, templateImageBuildStageCount,
		"Starting the builder from "+base,
		func() (string, error) {
			return b.runtime.LaunchContainer(bctx, base, templateImageBuilderName)
		},
	)
	if err != nil {
		return fmt.Errorf("launch builder from %s: %w; output: %s", base, err, out)
	}

	// Give cloud-init / systemd-resolved a moment so apt-get can reach the
	// network on the first try.
	select {
	case <-time.After(b.networkWarmup):
	case <-bctx.Done():
		return bctx.Err()
	}

	// Fail fast on a container that can only reach IPv6 (see egress.go).
	if _, err := b.runtime.ExecuteScript(bctx, templateImageBuilderName, ipv4EgressProbe); err != nil {
		return errors.New(ipv4EgressHint)
	}

	out, err = b.runBuildStage(
		2, templateImageBuildStageCount,
		"Provisioning the "+spec.Name+" stack",
		func() (string, error) {
			return b.runtime.ExecuteScript(bctx, templateImageBuilderName, spec.Program)
		},
	)
	if err != nil {
		return fmt.Errorf("provision %s: %w; output: %s", spec.Name, err, output.TruncateTail(out, 2000))
	}

	out, err = b.runBuildStage(
		3, templateImageBuildStageCount,
		"Finalizing the builder container",
		func() (string, error) {
			return b.runtime.StopContainer(bctx, templateImageBuilderName)
		},
	)
	if err != nil {
		return fmt.Errorf("stop builder: %w; output: %s", err, out)
	}

	pctx, pcancel := context.WithTimeout(ctx, b.publishTimeout)
	defer pcancel()
	out, err = b.runBuildStage(
		4, templateImageBuildStageCount,
		"Publishing "+spec.Alias,
		func() (string, error) {
			return b.runtime.PublishImage(pctx, templateImageBuilderName, spec.Alias, description)
		},
	)
	if err != nil {
		return fmt.Errorf("publish: %w; output: %s", err, out)
	}
	return nil
}
