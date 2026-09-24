package permission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Service is the permission application service: it evaluates checks for
// owning services and applies audited policy mutations under the delegation
// rules. It holds no policy itself; the Repository does.
type Service struct {
	registry  *Registry
	repo      Repository
	identity  IdentityDirectory
	evaluator *Evaluator
	now       func() time.Time
	newID     func() (string, error)
}

// Option customizes a Service, chiefly for deterministic tests.
type Option func(*Service)

// WithClock replaces the time source recorded in policy and audit records.
func WithClock(now func() time.Time) Option {
	return func(service *Service) { service.now = now }
}

// WithIDGenerator replaces the record ID source.
func WithIDGenerator(newID func() (string, error)) Option {
	return func(service *Service) { service.newID = newID }
}

// NewService validates the stored policy against the registry and returns the
// service. Stored policy that names an unregistered permission fails startup
// with an actionable error instead of being silently discarded.
func NewService(
	ctx context.Context,
	registry *Registry,
	repo Repository,
	identity IdentityDirectory,
	members ProjectMembership,
	options ...Option,
) (*Service, error) {
	if registry == nil {
		return nil, errors.New("permission registry is required")
	}
	if repo == nil {
		return nil, errors.New("permission repository is required")
	}
	if identity == nil {
		return nil, errors.New("permission identity directory is required")
	}
	state, err := repo.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load permission policy: %w", err)
	}
	if err := state.ValidateAgainst(registry); err != nil {
		return nil, err
	}
	service := &Service{
		registry:  registry,
		repo:      repo,
		identity:  identity,
		evaluator: NewEvaluator(registry, repo, identity, members),
		now:       time.Now,
		newID:     randomID,
	}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// Evaluate, Can, and Require expose the evaluator so the composition root can
// hand owning services a narrow authorizer.
func (s *Service) Evaluate(ctx context.Context, check Check) (Decision, error) {
	return s.evaluator.Evaluate(ctx, check)
}

func (s *Service) Can(ctx context.Context, check Check) (bool, error) {
	return s.evaluator.Can(ctx, check)
}

func (s *Service) Require(ctx context.Context, check Check) error {
	return s.evaluator.Require(ctx, check)
}

// mutation is the context a policy change runs in: the acting principal and
// the snapshot it holds under the store lock.
type mutation struct {
	service *Service
	ctx     context.Context
	actor   Actor
	state   State
	events  []AuditEvent
}

// mutate runs change inside Repository.Mutate. Authorization performed by
// change therefore sees exactly the state it is about to modify.
func (s *Service) mutate(ctx context.Context, change func(*mutation) error) error {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return ErrActorRequired
	}
	return s.repo.Mutate(ctx, func(state State) (State, []AuditEvent, error) {
		m := &mutation{service: s, ctx: ctx, actor: actor, state: state}
		if err := change(m); err != nil {
			return State{}, nil, err
		}
		return m.state, m.events, nil
	})
}

func (m *mutation) record(event AuditEvent) {
	event.At = m.service.now().UnixMilli()
	event.Actor = m.actor.Name()
	m.events = append(m.events, event)
}

func (m *mutation) evaluate(check Check) (Decision, error) {
	return m.service.evaluator.evaluate(m.ctx, m.state, m.actor, true, check)
}

// requireRegistered enforces that a permission target is an existing user.
func (m *mutation) requireRegistered(email string) error {
	registered, err := m.service.identity.IsRegistered(m.ctx, email)
	if err != nil {
		return err
	}
	if !registered {
		return ErrUserNotRegistered
	}
	return nil
}
