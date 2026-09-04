package schedule

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const (
	maxPromptBytes   = 32 << 10
	defaultBusyRetry = 15 * time.Second
)

type Option func(*Service)

func WithCronParser(parser CronParser) Option {
	return func(s *Service) {
		if parser != nil {
			s.cron = parser
		}
	}
}

// WithNow is primarily useful for deterministic tests.
func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithBusyRetry(delay time.Duration) Option {
	return func(s *Service) {
		if delay > 0 {
			s.busyRetry = delay
		}
	}
}

// WithMinInterval sets the floor between two starts of the same cron task.
// Definitions that would fire faster are rejected, and the claim path defers
// occurrences that arrive early (e.g. after the floor was raised). Zero
// disables the floor.
func WithMinInterval(interval time.Duration) Option {
	return func(s *Service) {
		if interval >= 0 {
			s.minInterval = interval
		}
	}
}

// WithMaxConcurrentRuns caps how many scheduled runs may hold claims at once
// across all chats, bounding simultaneous container boots. Overflow follows
// each task's overlap policy (queue or skip). Zero disables the cap. Forced
// RunNow bypasses the cap but still counts toward it.
func WithMaxConcurrentRuns(limit int) Option {
	return func(s *Service) {
		if limit >= 0 {
			s.maxConcurrent = limit
		}
	}
}

// WithMaxTasksPerProject caps standing (non-terminal) tasks per project.
// Zero disables the quota.
func WithMaxTasksPerProject(limit int) Option {
	return func(s *Service) {
		if limit >= 0 {
			s.maxTasksPerProject = limit
		}
	}
}

// WithRunObserver installs an observer of scheduled run outcomes.
func WithRunObserver(observer RunObserver) Option {
	return func(s *Service) {
		s.observer = observer
	}
}

type Service struct {
	repo       Repository
	chats      ChatLookup
	access     ProjectAccess
	identities IdentityDirectory
	executor   Executor
	cron       CronParser
	observer   RunObserver
	now        func() time.Time
	busyRetry  time.Duration

	minInterval        time.Duration
	maxConcurrent      int
	maxTasksPerProject int

	wake chan struct{}

	lifecycleMu sync.Mutex
	baseCtx     context.Context
	baseCancel  context.CancelFunc
	runCtx      context.Context
	cancel      context.CancelFunc
	started     bool
	wg          sync.WaitGroup

	// claimMu makes chat-level overlap checks and claims indivisible within
	// this scheduler instance.
	claimMu sync.Mutex
}

func New(
	repo Repository,
	chats ChatLookup,
	access ProjectAccess,
	identities IdentityDirectory,
	executor Executor,
	options ...Option,
) *Service {
	baseCtx, baseCancel := context.WithCancel(context.Background())
	s := &Service{
		repo:       repo,
		chats:      chats,
		access:     access,
		identities: identities,
		executor:   executor,
		cron:       FiveFieldCronParser{},
		now:        time.Now,
		busyRetry:  defaultBusyRetry,
		wake:       make(chan struct{}, 1),
		baseCtx:    baseCtx,
		baseCancel: baseCancel,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// Start begins the single timer/wake scheduler loop. It is idempotent. The
// supplied context owns the loop and all prompts started by it.
func (s *Service) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started {
		return nil
	}
	if s.repo == nil {
		return errors.New("schedule repository is not configured")
	}
	if err := s.recoverClaims(ctx); err != nil {
		return fmt.Errorf("recover scheduled task claims: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.runCtx = runCtx
	s.cancel = cancel
	s.started = true
	s.wg.Add(1)
	go s.loop(runCtx)
	return nil
}

func (s *Service) Close() {
	s.lifecycleMu.Lock()
	cancel := s.cancel
	baseCancel := s.baseCancel
	s.cancel = nil
	s.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if baseCancel != nil {
		baseCancel()
	}
	s.wg.Wait()
}

func (s *Service) List(ctx context.Context, callerEmail string, isAdmin bool) ([]Task, error) {
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return tasks, nil
	}
	callerEmail = normalizeEmail(callerEmail)
	filtered := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.OwnerEmail != callerEmail {
			continue
		}
		if err := s.authorizeCaller(ctx, task, callerEmail, false); err != nil {
			if isPermanentAuthorizationError(err) {
				continue
			}
			return nil, err
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

func (s *Service) Get(
	ctx context.Context,
	id ID,
	callerEmail string,
	isAdmin bool,
) (Task, error) {
	if !ValidID(id) {
		return Task{}, ErrInvalidID
	}
	task, err := s.repo.Get(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if !isAdmin && task.OwnerEmail != normalizeEmail(callerEmail) {
		return Task{}, ErrAccessDenied
	}
	if err := s.authorizeCaller(ctx, task, normalizeEmail(callerEmail), isAdmin); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Service) Create(
	ctx context.Context,
	input CreateInput,
	ownerEmail string,
	isAdmin bool,
) (Task, error) {
	now := s.now()
	task := Task{
		ID:         newID(),
		Name:       strings.TrimSpace(input.Name),
		OwnerEmail: normalizeEmail(ownerEmail),
		ProjectID:  input.ProjectID,
		ChatID:     input.ChatID,
		Prompt:     strings.TrimSpace(input.Prompt),
		Kind:       input.Kind,
		At:         input.At,
		Cron:       strings.TrimSpace(input.Cron),
		Timezone:   strings.TrimSpace(input.Timezone),
		Enabled:    true,
		Status:     StatusActive,
		MaxRuns:    input.MaxRuns,
		Overlap:    input.Overlap,
		CreatedAt:  now.UnixMilli(),
		UpdatedAt:  now.UnixMilli(),
	}
	normalizeTaskDefaults(&task)
	if err := s.validateDefinition(task, now, true); err != nil {
		return Task{}, err
	}
	if err := s.validateCreationAccess(ctx, task, isAdmin); err != nil {
		return Task{}, err
	}
	if err := s.enforceProjectQuota(ctx, task.ProjectID); err != nil {
		return Task{}, err
	}
	if input.CreatedByAgent {
		// Agent-minted tasks are parked until a user arms them from the
		// drawer. This is the enforced arm step: a confused or injected
		// turn can define work but cannot put it on the clock.
		task.CreatedByAgent = true
		task.Enabled = false
		task.Status = StatusPaused
	} else {
		next, err := s.nextOccurrence(task, now)
		if err != nil {
			return Task{}, err
		}
		task.NextRunAt = next.UnixMilli()
	}
	created, err := s.repo.Create(ctx, task)
	if err == nil {
		s.notify()
	}
	return created, err
}

// enforceProjectQuota bounds standing task definitions per project. Terminal
// tasks (completed, exhausted, error) are history and do not consume quota.
func (s *Service) enforceProjectQuota(ctx context.Context, projectID serviceproject.ID) error {
	if s.maxTasksPerProject <= 0 {
		return nil
	}
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	standing := 0
	for _, task := range tasks {
		if task.ProjectID != projectID {
			continue
		}
		switch task.Status {
		case StatusCompleted, StatusExhausted, StatusError:
			continue
		}
		standing++
	}
	if standing >= s.maxTasksPerProject {
		return fmt.Errorf("%w (limit %d)", ErrProjectQuota, s.maxTasksPerProject)
	}
	return nil
}

func (s *Service) Update(
	ctx context.Context,
	id ID,
	input UpdateInput,
	callerEmail string,
	isAdmin bool,
) (Task, error) {
	if _, err := s.Get(ctx, id, callerEmail, isAdmin); err != nil {
		return Task{}, err
	}
	now := s.now()
	updated, err := s.repo.Update(ctx, id, func(task *Task) error {
		scheduleChanged := applyUpdate(task, input)
		normalizeTaskDefaults(task)
		requiresFuture := scheduleChanged || (input.Enabled != nil && *input.Enabled)
		if err := s.validateDefinition(*task, now, requiresFuture); err != nil {
			return err
		}

		if input.Enabled != nil {
			task.Enabled = *input.Enabled
			if !task.Enabled {
				task.Status = StatusPaused
				task.PendingRun = false
				task.PendingRunForced = false
				task.PendingSince = 0
				task.RetryAt = 0
			} else {
				task.Status = StatusActive
			}
		}
		if task.MaxRuns > 0 && task.RunCount >= task.MaxRuns {
			task.Enabled = false
			task.Status = StatusExhausted
			task.PendingRun = false
			task.PendingRunForced = false
			task.RetryAt = 0
		}
		if task.Enabled && (scheduleChanged || (input.Enabled != nil && *input.Enabled)) {
			next, err := s.nextOccurrence(*task, now)
			if err != nil {
				return err
			}
			task.NextRunAt = next.UnixMilli()
			task.PendingRun = false
			task.PendingRunForced = false
			task.PendingSince = 0
			task.RetryAt = 0
		}
		task.UpdatedAt = now.UnixMilli()
		return nil
	})
	if err == nil {
		s.notify()
	}
	return updated, err
}

func (s *Service) Delete(
	ctx context.Context,
	id ID,
	callerEmail string,
	isAdmin bool,
) error {
	if _, err := s.Get(ctx, id, callerEmail, isAdmin); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.notify()
	return nil
}

// RunNow immediately claims one occurrence without moving the task's normal
// cron/once deadline. A queue_one overlap becomes one follow-up run.
func (s *Service) RunNow(
	ctx context.Context,
	id ID,
	callerEmail string,
	isAdmin bool,
) (Task, error) {
	task, err := s.Get(ctx, id, callerEmail, isAdmin)
	if err != nil {
		return Task{}, err
	}
	if err := s.authorizeRun(ctx, task); err != nil {
		if isPermanentAuthorizationError(err) {
			_ = s.pauseForAuthorization(ctx, task.ID, err)
		}
		return Task{}, err
	}
	claimed, ok, err := s.claim(ctx, id, s.now(), true)
	if err != nil {
		return Task{}, err
	}
	if !ok {
		// A queue_one overlap is an accepted request even though no new
		// executor is started yet.
		return s.repo.Get(ctx, id)
	}
	if err := s.dispatch(ctx, claimed); err != nil {
		return claimed, err
	}
	// Dispatch can turn an executor-level busy rejection back into a durable
	// queue. Return the current state so callers do not see the stale claim.
	return s.repo.Get(ctx, id)
}

// Complete disables a task while preserving its definition and run history.
func (s *Service) Complete(
	ctx context.Context,
	id ID,
	callerEmail string,
	isAdmin bool,
) (Task, error) {
	if _, err := s.Get(ctx, id, callerEmail, isAdmin); err != nil {
		return Task{}, err
	}
	return s.complete(ctx, id)
}

// CompleteClaim is for a narrowly scoped complete-current-schedule tool. The
// active claim prevents one scheduled prompt from completing another task.
func (s *Service) CompleteClaim(
	ctx context.Context,
	id ID,
	activeRunID string,
) (Task, error) {
	if strings.TrimSpace(activeRunID) == "" {
		return Task{}, ErrAccessDenied
	}
	nowMS := s.now().UnixMilli()
	task, err := s.repo.Update(ctx, id, func(task *Task) error {
		if task.ActiveRunID != activeRunID {
			return ErrAccessDenied
		}
		markCompleted(task, nowMS)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return task, err
}

func (s *Service) complete(ctx context.Context, id ID) (Task, error) {
	nowMS := s.now().UnixMilli()
	task, err := s.repo.Update(ctx, id, func(task *Task) error {
		markCompleted(task, nowMS)
		return nil
	})
	if err == nil {
		s.notify()
	}
	return task, err
}

func markCompleted(task *Task, nowMS int64) {
	task.Enabled = false
	task.Status = StatusCompleted
	task.NextRunAt = 0
	task.PendingRun = false
	task.PendingRunForced = false
	task.PendingSince = 0
	task.RetryAt = 0
	task.UpdatedAt = nowMS
}

// RunDue claims and dispatches every due occurrence. It is exported so a
// caller can request an immediate reconciliation without creating another
// scheduler loop.
func (s *Service) RunDue(ctx context.Context) error {
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	sort.SliceStable(tasks, func(i, j int) bool {
		return taskDueAt(tasks[i]) < taskDueAt(tasks[j])
	})

	var runErrors []error
	for _, snapshot := range tasks {
		if !isDue(snapshot, now.UnixMilli()) {
			continue
		}
		if err := s.authorizeRun(ctx, snapshot); err != nil {
			if isPermanentAuthorizationError(err) {
				if pauseErr := s.pauseForAuthorization(ctx, snapshot.ID, err); pauseErr != nil {
					runErrors = append(runErrors, pauseErr)
				}
			} else if retryErr := s.deferAfterError(ctx, snapshot.ID, err); retryErr != nil {
				runErrors = append(runErrors, retryErr)
			}
			runErrors = append(runErrors, err)
			continue
		}

		claimed, ok, err := s.claim(ctx, snapshot.ID, now, false)
		if err != nil {
			runErrors = append(runErrors, err)
			continue
		}
		if !ok {
			continue
		}
		if err := s.dispatch(ctx, claimed); err != nil {
			runErrors = append(runErrors, err)
		}
	}
	return errors.Join(runErrors...)
}

func (s *Service) claim(
	ctx context.Context,
	id ID,
	now time.Time,
	force bool,
) (Task, bool, error) {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()

	task, err := s.repo.Get(ctx, id)
	if err != nil {
		return Task{}, false, err
	}
	nowMS := now.UnixMilli()
	force = force || task.PendingRunForced
	if !force && (!task.Enabled || !isDue(task, nowMS)) {
		return Task{}, false, nil
	}

	if !force && task.MaxRuns > 0 && task.RunCount >= task.MaxRuns {
		_, err := s.repo.Update(ctx, id, func(current *Task) error {
			current.Enabled = false
			current.Status = StatusExhausted
			current.NextRunAt = 0
			current.PendingRun = false
			current.PendingRunForced = false
			current.RetryAt = 0
			current.UpdatedAt = nowMS
			return nil
		})
		return Task{}, false, err
	}

	// Fire-time interval floor: regardless of what the cron expression claims,
	// two scheduled starts of one task never come closer than the floor. The
	// occurrence is delayed, not consumed. Forced runs are a human decision
	// and bypass the floor.
	if !force && s.minInterval > 0 && task.LastRunAt > 0 {
		earliest := task.LastRunAt + s.minInterval.Milliseconds()
		if nowMS < earliest {
			_, err := s.repo.Update(ctx, id, func(current *Task) error {
				if current.PendingRun {
					if current.RetryAt < earliest {
						current.RetryAt = earliest
					}
				} else if current.NextRunAt > 0 && current.NextRunAt < earliest {
					current.NextRunAt = earliest
				}
				current.UpdatedAt = nowMS
				return nil
			})
			return Task{}, false, err
		}
	}

	all, err := s.repo.List(ctx)
	if err != nil {
		return Task{}, false, err
	}
	chatBusy := task.ActiveRunID != ""
	for _, other := range all {
		if other.ID != task.ID && other.ChatID == task.ChatID && other.ActiveRunID != "" {
			chatBusy = true
			break
		}
	}
	if chatBusy {
		updated, updateErr := s.consumeOverlap(
			ctx, task, now, force,
			"chat already has a scheduled run in progress",
		)
		return updated, false, updateErr
	}

	// Global dispatch cap: bound simultaneous scheduled runs (and the
	// container boots they imply) across all chats. Saturation follows the
	// task's overlap policy, so queue_one tasks fire as slots free up.
	if !force && s.maxConcurrent > 0 {
		active := 0
		for _, other := range all {
			if other.ActiveRunID != "" {
				active++
			}
		}
		if active >= s.maxConcurrent {
			updated, updateErr := s.consumeOverlap(
				ctx, task, now, force,
				"scheduler is at its concurrent run limit",
			)
			return updated, false, updateErr
		}
	}

	queued := task.PendingRun
	runID := newRunID()
	updated, err := s.repo.Update(ctx, id, func(current *Task) error {
		if (!force && (!current.Enabled || !isDue(*current, nowMS))) ||
			current.ActiveRunID != "" {
			return nil
		}
		current.PendingRun = false
		current.PendingRunForced = false
		current.PendingSince = 0
		current.RetryAt = 0
		current.ActiveRunID = runID
		current.ActiveRunStarted = nowMS
		current.ActiveRunForced = force
		current.RunCount++
		current.LastRunAt = nowMS
		current.LastRunEnd = 0
		current.LastStatus = RunStatusRunning
		current.LastError = ""
		if current.Enabled {
			current.Status = StatusRunning
		}
		advanceScheduledDeadline := !queued && !force
		if queued && current.Enabled &&
			current.NextRunAt > 0 && current.NextRunAt <= nowMS {
			// Executor-level busy rejections retain one queued occurrence.
			// If additional cron deadlines passed while the chat was busy,
			// coalesce all of them into the run being accepted now.
			advanceScheduledDeadline = true
		}
		if advanceScheduledDeadline {
			next, nextErr := s.nextAfterClaim(*current, now)
			if nextErr != nil {
				return nextErr
			}
			current.NextRunAt = next
		}
		current.UpdatedAt = nowMS
		return nil
	})
	if err != nil {
		return Task{}, false, err
	}
	if updated.ActiveRunID != runID {
		return Task{}, false, nil
	}
	return updated, true, nil
}

func (s *Service) consumeOverlap(
	ctx context.Context,
	task Task,
	now time.Time,
	force bool,
	reason string,
) (Task, error) {
	nowMS := now.UnixMilli()
	return s.repo.Update(ctx, task.ID, func(current *Task) error {
		if !force && (!current.Enabled || !isDue(*current, nowMS)) {
			return nil
		}
		if !force {
			next, err := s.nextAfterClaim(*current, now)
			if err != nil {
				return err
			}
			current.NextRunAt = next
		}
		current.UpdatedAt = nowMS
		if current.Overlap == OverlapQueueOne {
			current.PendingRun = true
			current.PendingRunForced = current.PendingRunForced || force
			if current.PendingSince == 0 {
				current.PendingSince = nowMS
			}
			current.LastStatus = RunStatusQueued
			return nil
		}

		current.RunCount++
		current.LastRunAt = nowMS
		current.LastRunEnd = nowMS
		current.LastStatus = RunStatusSkipped
		current.LastError = reason
		if current.Kind == KindOnce ||
			(current.MaxRuns > 0 && current.RunCount >= current.MaxRuns) {
			current.Enabled = false
			current.Status = StatusExhausted
			current.NextRunAt = 0
		} else if current.ActiveRunID == "" {
			current.Status = StatusActive
		}
		return nil
	})
}

func (s *Service) dispatch(_ context.Context, task Task) error {
	ctx := s.executionContext()
	if s.executor == nil {
		err := errors.New("scheduled prompt executor is not configured")
		_ = s.finish(ctx, task, RunResult{Err: err})
		return err
	}
	handle, err := s.executor.StartScheduledPrompt(ctx, task, ScheduledPrompt(task))
	if err != nil {
		if errors.Is(err, ErrExecutorBusy) && task.Overlap == OverlapQueueOne {
			if requeueErr := s.requeueRejected(ctx, task, err); requeueErr != nil {
				return errors.Join(err, requeueErr)
			}
			// Queue-one accepted the occurrence. Do not report a failure to a
			// RunNow caller or make the scheduler back off as though work were
			// lost.
			return nil
		} else {
			_ = s.finish(ctx, task, RunResult{Err: err})
		}
		return err
	}
	if handle == nil || handle.Done() == nil {
		err := errors.New("scheduled prompt executor returned an invalid run handle")
		_ = s.finish(ctx, task, RunResult{Err: err})
		return err
	}

	done := handle.Done()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case result, ok := <-done:
			if !ok {
				result.Err = errors.New("scheduled prompt completion channel closed without a result")
			}
			if err := s.finish(context.Background(), task, result); err != nil &&
				!errors.Is(err, ErrNotFound) {
				log.Printf("schedules: finish %s: %v", task.ID, err)
			}
		case <-ctx.Done():
			// Leave the durable claim in place. Start() reconciles it as an
			// abandoned run on the next process start.
		}
	}()
	return nil
}

func (s *Service) executionContext() context.Context {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.runCtx != nil {
		return s.runCtx
	}
	return s.baseCtx
}

func (s *Service) finish(ctx context.Context, claimed Task, result RunResult) error {
	nowMS := s.now().UnixMilli()
	settled := false
	updated, err := s.repo.Update(ctx, claimed.ID, func(task *Task) error {
		if task.ActiveRunID != claimed.ActiveRunID {
			return nil
		}
		settled = true
		task.ActiveRunID = ""
		task.ActiveRunStarted = 0
		task.ActiveRunForced = false
		task.LastRunEnd = nowMS
		task.UpdatedAt = nowMS
		if result.Err != nil {
			task.LastStatus = RunStatusFailed
			task.LastError = result.Err.Error()
		} else {
			task.LastStatus = RunStatusSucceeded
			task.LastError = ""
		}

		complete := result.TaskComplete || HasCompletionMarker(result.Output)
		switch {
		case complete:
			task.Enabled = false
			task.Status = StatusCompleted
			task.NextRunAt = 0
			task.PendingRun = false
			task.PendingRunForced = false
			task.PendingSince = 0
			task.RetryAt = 0
		case !task.Enabled && task.Status == StatusCompleted:
			// Complete/CompleteClaim can be called before the executor sends
			// its final result. Preserve that explicit terminal decision.
			task.NextRunAt = 0
			task.PendingRun = false
			task.PendingRunForced = false
			task.PendingSince = 0
			task.RetryAt = 0
		case !task.Enabled:
			// A forced RunNow on a paused/completed task leaves the schedule's
			// enabled state and terminal/paused status unchanged.
			if task.Status == StatusRunning {
				task.Status = StatusPaused
			}
		case task.Kind == KindOnce:
			task.Enabled = false
			task.NextRunAt = 0
			task.PendingRun = false
			task.PendingRunForced = false
			task.PendingSince = 0
			task.RetryAt = 0
			if result.Err != nil {
				task.Status = StatusError
			} else {
				task.Status = StatusCompleted
			}
		case task.MaxRuns > 0 && task.RunCount >= task.MaxRuns:
			task.Enabled = false
			task.Status = StatusExhausted
			task.NextRunAt = 0
			task.PendingRun = false
			task.PendingRunForced = false
			task.PendingSince = 0
			task.RetryAt = 0
		default:
			task.Status = StatusActive
		}
		return nil
	})
	if err == nil {
		s.notify()
		if settled && s.observer != nil {
			s.observer.ScheduledRunFinished(ctx, updated, result)
		}
	}
	return err
}

func (s *Service) requeueRejected(ctx context.Context, claimed Task, cause error) error {
	nowMS := s.now().UnixMilli()
	_, err := s.repo.Update(ctx, claimed.ID, func(task *Task) error {
		if task.ActiveRunID != claimed.ActiveRunID {
			return nil
		}
		task.ActiveRunID = ""
		task.ActiveRunStarted = 0
		forced := task.ActiveRunForced
		task.ActiveRunForced = false
		if task.RunCount > 0 {
			task.RunCount--
		}
		task.PendingRun = true
		task.PendingRunForced = task.PendingRunForced || forced
		if task.PendingSince == 0 {
			task.PendingSince = nowMS
		}
		task.RetryAt = nowMS + s.busyRetry.Milliseconds()
		task.LastStatus = RunStatusQueued
		task.LastError = cause.Error()
		if task.Enabled {
			task.Status = StatusActive
		}
		task.UpdatedAt = nowMS
		return nil
	})
	if err == nil {
		s.notify()
	}
	return err
}

func (s *Service) pauseForAuthorization(ctx context.Context, id ID, cause error) error {
	nowMS := s.now().UnixMilli()
	_, err := s.repo.Update(ctx, id, func(task *Task) error {
		task.Enabled = false
		task.Status = StatusError
		task.LastStatus = RunStatusFailed
		task.LastError = cause.Error()
		task.NextRunAt = 0
		task.PendingRun = false
		task.PendingRunForced = false
		task.PendingSince = 0
		task.RetryAt = 0
		task.UpdatedAt = nowMS
		return nil
	})
	return err
}

func (s *Service) deferAfterError(ctx context.Context, id ID, cause error) error {
	nowMS := s.now().UnixMilli()
	_, err := s.repo.Update(ctx, id, func(task *Task) error {
		task.RetryAt = nowMS + s.busyRetry.Milliseconds()
		task.LastStatus = RunStatusFailed
		task.LastError = cause.Error()
		task.UpdatedAt = nowMS
		return nil
	})
	return err
}

func (s *Service) authorizeRun(ctx context.Context, task Task) error {
	if s.chats == nil {
		return ErrChatNotFound
	}
	meta, err := s.chats.Get(ctx, task.ChatID)
	if err != nil {
		if errors.Is(err, servicechat.ErrNotFound) {
			return ErrChatNotFound
		}
		return fmt.Errorf("look up scheduled task chat: %w", err)
	}
	if meta.ProjectID == "" {
		return ErrProjectRequired
	}
	if string(meta.ProjectID) != string(task.ProjectID) {
		return ErrProjectMismatch
	}
	if s.identities != nil {
		registered, err := s.identities.IsRegistered(ctx, task.OwnerEmail)
		if err != nil {
			return fmt.Errorf("check scheduled task owner: %w", err)
		}
		if !registered {
			return ErrOwnerUnregistered
		}
		admin, err := s.identities.IsAdmin(ctx, task.OwnerEmail)
		if err != nil {
			return fmt.Errorf("check scheduled task owner role: %w", err)
		}
		if admin {
			return nil
		}
	}
	if s.access != nil {
		allowed, err := s.access.HasAccess(ctx, task.ProjectID, task.OwnerEmail)
		if err != nil {
			return fmt.Errorf("check scheduled task project access: %w", err)
		}
		if !allowed {
			return ErrAccessDenied
		}
	}
	return nil
}

func (s *Service) authorizeCaller(
	ctx context.Context,
	task Task,
	callerEmail string,
	isAdmin bool,
) error {
	if s.chats == nil {
		return ErrChatNotFound
	}
	meta, err := s.chats.Get(ctx, task.ChatID)
	if err != nil {
		if errors.Is(err, servicechat.ErrNotFound) {
			return ErrChatNotFound
		}
		return fmt.Errorf("look up scheduled task chat: %w", err)
	}
	if meta.ProjectID == "" {
		return ErrProjectRequired
	}
	if string(meta.ProjectID) != string(task.ProjectID) {
		return ErrProjectMismatch
	}
	if isAdmin {
		return nil
	}
	if s.identities != nil {
		registered, err := s.identities.IsRegistered(ctx, callerEmail)
		if err != nil {
			return fmt.Errorf("check scheduled task caller: %w", err)
		}
		if !registered {
			return ErrOwnerUnregistered
		}
	}
	if s.access != nil {
		allowed, err := s.access.HasAccess(ctx, task.ProjectID, callerEmail)
		if err != nil {
			return fmt.Errorf("check scheduled task caller project access: %w", err)
		}
		if !allowed {
			return ErrAccessDenied
		}
	}
	return nil
}

func (s *Service) validateCreationAccess(ctx context.Context, task Task, isAdmin bool) error {
	if s.chats == nil {
		return ErrChatNotFound
	}
	meta, err := s.chats.Get(ctx, task.ChatID)
	if err != nil {
		if errors.Is(err, servicechat.ErrNotFound) {
			return ErrChatNotFound
		}
		return err
	}
	if meta.ProjectID == "" {
		return ErrProjectRequired
	}
	if string(meta.ProjectID) != string(task.ProjectID) {
		return ErrProjectMismatch
	}
	if s.identities != nil {
		registered, err := s.identities.IsRegistered(ctx, task.OwnerEmail)
		if err != nil {
			return err
		}
		if !registered {
			return ErrOwnerUnregistered
		}
	}
	if !isAdmin && s.access != nil {
		allowed, err := s.access.HasAccess(ctx, task.ProjectID, task.OwnerEmail)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrAccessDenied
		}
	}
	return nil
}

func (s *Service) validateDefinition(task Task, now time.Time, requireFuture bool) error {
	if strings.TrimSpace(task.Name) == "" {
		return ErrNameRequired
	}
	if strings.TrimSpace(task.OwnerEmail) == "" {
		return ErrOwnerRequired
	}
	if strings.TrimSpace(task.Prompt) == "" {
		return ErrPromptRequired
	}
	if len(task.Prompt) > maxPromptBytes {
		return ErrPromptTooLarge
	}
	if task.ProjectID == "" {
		return ErrProjectRequired
	}
	if !servicechat.ValidID(task.ChatID) {
		return ErrChatNotFound
	}
	if task.MaxRuns < 0 {
		return ErrInvalidMaxRuns
	}
	if task.Overlap != OverlapQueueOne && task.Overlap != OverlapSkip {
		return ErrInvalidOverlap
	}
	location, err := time.LoadLocation(task.Timezone)
	if err != nil || location.String() != task.Timezone {
		return ErrInvalidTimezone
	}
	switch task.Kind {
	case KindOnce:
		if task.At <= 0 || (requireFuture && task.At <= now.UnixMilli()) {
			return ErrInvalidTime
		}
	case KindCron:
		if strings.TrimSpace(task.Cron) == "" {
			return ErrInvalidCron
		}
		if _, err := s.cron.Next(task.Cron, now, location); err != nil {
			return err
		}
		if err := s.validateCronInterval(task.Cron, now, location); err != nil {
			return err
		}
	default:
		return ErrInvalidKind
	}
	return nil
}

// validateCronInterval samples successive occurrences and rejects expressions
// that fire faster than the configured floor. Sampling is bounded (steps and
// horizon), so pathologically rare tight pairs can slip through — the claim
// path's fire-time deferral is the backstop that makes the floor airtight.
func (s *Service) validateCronInterval(expr string, now time.Time, location *time.Location) error {
	if s.minInterval <= 0 {
		return nil
	}
	const sampleSteps = 32
	horizon := now.Add(45 * 24 * time.Hour)
	previous, err := s.cron.Next(expr, now, location)
	if err != nil {
		return err
	}
	for i := 0; i < sampleSteps && previous.Before(horizon); i++ {
		next, err := s.cron.Next(expr, previous, location)
		if err != nil {
			return err
		}
		if gap := next.Sub(previous); gap < s.minInterval {
			return fmt.Errorf(
				"%w (fires %s apart, minimum is %s)",
				ErrIntervalTooSmall, gap, s.minInterval,
			)
		}
		previous = next
	}
	return nil
}

func (s *Service) nextOccurrence(task Task, after time.Time) (time.Time, error) {
	if task.Kind == KindOnce {
		return time.UnixMilli(task.At), nil
	}
	location, err := time.LoadLocation(task.Timezone)
	if err != nil {
		return time.Time{}, ErrInvalidTimezone
	}
	return s.cron.Next(task.Cron, after, location)
}

func (s *Service) nextAfterClaim(task Task, now time.Time) (int64, error) {
	if task.Kind == KindOnce {
		return 0, nil
	}
	next, err := s.nextOccurrence(task, now)
	if err != nil {
		return 0, err
	}
	return next.UnixMilli(), nil
}

func (s *Service) recoverClaims(ctx context.Context) error {
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	nowMS := s.now().UnixMilli()
	var recoveryErrors []error
	for _, task := range tasks {
		if task.ActiveRunID == "" {
			continue
		}
		_, err := s.repo.Update(ctx, task.ID, func(current *Task) error {
			if current.ActiveRunID == "" {
				return nil
			}
			wasForced := current.ActiveRunForced
			current.ActiveRunID = ""
			current.ActiveRunStarted = 0
			current.ActiveRunForced = false
			current.LastRunEnd = nowMS
			current.LastStatus = RunStatusAbandoned
			current.LastError = "scheduler stopped before the run reported completion"
			current.UpdatedAt = nowMS
			switch {
			case wasForced && !current.Enabled:
				// A manual run did not change the schedule's enabled/status
				// state, so recovery must not change it either.
			case current.Kind == KindOnce:
				current.Enabled = false
				current.Status = StatusError
				current.NextRunAt = 0
				current.PendingRun = false
				current.PendingRunForced = false
			case current.MaxRuns > 0 && current.RunCount >= current.MaxRuns:
				current.Enabled = false
				current.Status = StatusExhausted
				current.NextRunAt = 0
				current.PendingRun = false
				current.PendingRunForced = false
			case current.Enabled:
				current.Status = StatusActive
			default:
				current.Status = StatusPaused
			}
			return nil
		})
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
		}
	}
	return errors.Join(recoveryErrors...)
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	for {
		runErr := s.RunDue(ctx)
		runFailed := runErr != nil && !errors.Is(runErr, context.Canceled)
		if runFailed {
			log.Printf("schedules: run due tasks: %v", runErr)
		}
		delay, ok, err := s.delayUntilNext(ctx)
		if err != nil {
			log.Printf("schedules: find next task: %v", err)
			delay, ok = s.busyRetry, true
		} else if runFailed && (!ok || delay < s.busyRetry) {
			// A durable mutation can fail while reads still succeed (for
			// example, a full disk). The unchanged overdue task must not turn
			// that failure into a zero-delay CPU/log loop.
			delay, ok = s.busyRetry, true
		}

		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				continue
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-s.wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (s *Service) delayUntilNext(ctx context.Context) (time.Duration, bool, error) {
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return 0, false, err
	}
	nowMS := s.now().UnixMilli()
	var earliest int64
	for _, task := range tasks {
		if !task.Enabled && !task.PendingRunForced {
			continue
		}
		candidate := task.NextRunAt
		if task.RetryAt > nowMS &&
			(task.PendingRun || candidate == 0 || candidate <= nowMS) {
			candidate = task.RetryAt
		}
		// A pending occurrence with RetryAt == 0 is deliberately wake-driven:
		// the active task finishing calls notify. Treating it as immediately
		// due here would spin while the chat remains busy.
		if candidate <= 0 {
			continue
		}
		if earliest == 0 || candidate < earliest {
			earliest = candidate
		}
	}
	if earliest == 0 {
		return 0, false, nil
	}
	// time.Time.Sub saturates at the largest representable Duration. Directly
	// multiplying arbitrary Unix-millisecond input can overflow and turn a
	// far-future deadline into a zero-delay scheduler loop.
	delay := time.UnixMilli(earliest).Sub(time.UnixMilli(nowMS))
	if delay < 0 {
		delay = 0
	}
	return delay, true, nil
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// ScheduledPrompt is the backend-owned envelope injected into the existing
// chat. The exact final marker lets the scheduler disable the task without
// granting the agent broad schedule-management permissions.
func ScheduledPrompt(task Task) string {
	fire := fmt.Sprintf("%d", task.RunCount)
	if task.MaxRuns > 0 {
		fire = fmt.Sprintf("%d/%d", task.RunCount, task.MaxRuns)
	}
	return fmt.Sprintf(
		"[scheduled task %q, fire %s]\n\n%s\n\n"+
			"Continue the standing task from earlier in this chat. "+
			"If the standing goal is complete, end your response with exactly:\n"+
			"SCHEDULE_STATUS=COMPLETE\n\n"+
			"Otherwise, report the current status and what remains. "+
			"The backend will manage this schedule.",
		task.Name,
		fire,
		task.Prompt,
	)
}

func HasCompletionMarker(output string) bool {
	output = strings.TrimSpace(strings.ReplaceAll(output, "\r\n", "\n"))
	if output == "" {
		return false
	}
	lines := strings.Split(output, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	return last == "SCHEDULE_STATUS=COMPLETE" || last == "TASK_COMPLETE"
}

func applyUpdate(task *Task, input UpdateInput) bool {
	changed := false
	if input.Name != nil {
		task.Name = strings.TrimSpace(*input.Name)
	}
	if input.Prompt != nil {
		task.Prompt = strings.TrimSpace(*input.Prompt)
	}
	if input.Kind != nil && task.Kind != *input.Kind {
		task.Kind = *input.Kind
		changed = true
	}
	if input.At != nil && task.At != *input.At {
		task.At = *input.At
		changed = true
	}
	if input.Cron != nil {
		value := strings.TrimSpace(*input.Cron)
		if task.Cron != value {
			task.Cron = value
			changed = true
		}
	}
	if input.Timezone != nil {
		value := strings.TrimSpace(*input.Timezone)
		if task.Timezone != value {
			task.Timezone = value
			changed = true
		}
	}
	if input.MaxRuns != nil {
		task.MaxRuns = *input.MaxRuns
	}
	if input.Overlap != nil {
		task.Overlap = *input.Overlap
	}
	return changed
}

func normalizeTaskDefaults(task *Task) {
	task.Name = strings.TrimSpace(task.Name)
	task.OwnerEmail = normalizeEmail(task.OwnerEmail)
	task.Prompt = strings.TrimSpace(task.Prompt)
	task.Cron = strings.TrimSpace(task.Cron)
	task.Timezone = strings.TrimSpace(task.Timezone)
	if task.Timezone == "" {
		task.Timezone = "UTC"
	}
	if task.Overlap == "" {
		task.Overlap = OverlapQueueOne
	}
	if task.Kind == KindOnce {
		task.MaxRuns = 1
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isDue(task Task, nowMS int64) bool {
	if task.RetryAt > nowMS {
		return false
	}
	if task.PendingRun && task.ActiveRunID == "" &&
		(task.Enabled || task.PendingRunForced) {
		return true
	}
	if !task.Enabled {
		return false
	}
	return task.NextRunAt > 0 && task.NextRunAt <= nowMS
}

func taskDueAt(task Task) int64 {
	if task.RetryAt > 0 {
		return task.RetryAt
	}
	if task.PendingRun && task.ActiveRunID == "" {
		return 0
	}
	return task.NextRunAt
}

func isPermanentAuthorizationError(err error) bool {
	return errors.Is(err, ErrChatNotFound) ||
		errors.Is(err, ErrProjectRequired) ||
		errors.Is(err, ErrProjectMismatch) ||
		errors.Is(err, ErrOwnerUnregistered) ||
		errors.Is(err, ErrAccessDenied)
}

func newID() ID {
	var bytes [12]byte
	if _, err := crand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate scheduled task id: %v", err))
	}
	return ID(hex.EncodeToString(bytes[:]))
}

func newRunID() string {
	var bytes [12]byte
	if _, err := crand.Read(bytes[:]); err != nil {
		panic(fmt.Sprintf("generate scheduled run id: %v", err))
	}
	return hex.EncodeToString(bytes[:])
}
