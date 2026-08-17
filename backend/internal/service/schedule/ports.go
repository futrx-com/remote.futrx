package schedule

import (
	"context"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type Repository interface {
	List(ctx context.Context) ([]Task, error)
	Create(ctx context.Context, task Task) (Task, error)
	Get(ctx context.Context, id ID) (Task, error)
	Update(ctx context.Context, id ID, fn func(*Task) error) (Task, error)
	Delete(ctx context.Context, id ID) error
}

type ChatLookup interface {
	Get(ctx context.Context, id servicechat.ID) (servicechat.Meta, error)
}

type ProjectAccess interface {
	HasAccess(ctx context.Context, id serviceproject.ID, email string) (bool, error)
}

type IdentityDirectory interface {
	IsRegistered(ctx context.Context, email string) (bool, error)
	IsAdmin(ctx context.Context, email string) (bool, error)
}

// RunResult is delivered exactly once by a RunHandle. Output is inspected for
// the completion marker as a fallback when the executor cannot classify it.
type RunResult struct {
	Output       string
	TaskComplete bool
	Err          error
}

// RunHandle represents an accepted, asynchronously executing agent prompt.
type RunHandle interface {
	Done() <-chan RunResult
}

// Executor must return only after the run has either been accepted or
// rejected. Accepted work completes asynchronously through RunHandle.
type Executor interface {
	StartScheduledPrompt(
		ctx context.Context,
		task Task,
		prompt string,
	) (RunHandle, error)
}

type CronParser interface {
	Next(expression string, after time.Time, location *time.Location) (time.Time, error)
}

// RunObserver receives the outcome of every scheduled run for out-of-band
// reporting such as outbound notifications. Implementations must not block the
// scheduler.
type RunObserver interface {
	ScheduledRunFinished(ctx context.Context, task Task, result RunResult)
}
