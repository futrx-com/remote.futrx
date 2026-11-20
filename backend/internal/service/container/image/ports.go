package image

import (
	"context"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

// Runtime translates the image workflow's semantic operations to a container
// runtime. Command arguments and transport details remain behind this port.
type Runtime interface {
	Available() bool
	DeleteContainer(ctx context.Context, containerName string) (string, error)
	LaunchContainer(ctx context.Context, sourceImage, containerName string) (string, error)
	ExecuteScript(ctx context.Context, containerName, script string) (string, error)
	StopContainer(ctx context.Context, containerName string) (string, error)
	PublishImage(ctx context.Context, containerName, alias, description string) (string, error)
}

// ProfileSource supplies immutable snapshots for recipe generation.
type ProfileSource interface {
	Snapshot() []provisioning.Profile
}

type ProgressState uint8

const (
	ProgressStarted ProgressState = iota
	ProgressRunning
	ProgressSucceeded
	ProgressFailed
)

// Progress describes one observable transition in the base-image workflow.
// Delivery adapters decide how, or whether, to present it to a user.
type Progress struct {
	Stage       int
	StageCount  int
	Description string
	State       ProgressState
	Elapsed     time.Duration
}

type ProgressReporter interface {
	ReportImageBuildProgress(Progress)
}
