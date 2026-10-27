package applications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	api "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// AgentTurnRecord is the generic execution receipt. Application workflow state
// stays in its own DataDir; core stores only accepted input and execution results.
type AgentTurnRecord struct {
	Request api.AgentTurnRequest `json:"request"`
	Turn    api.AgentTurn        `json:"turn"`
}
type AgentTurnRepository interface {
	Get(context.Context, string, string) (AgentTurnRecord, bool, error)
	Put(context.Context, string, AgentTurnRecord) error
	Delete(context.Context, string, string) error
	Remove(context.Context, string) error
}
type AgentChats interface {
	Get(context.Context, servicechat.ID) (servicechat.Meta, error)
	EventPage(context.Context, servicechat.ID, servicechat.EventPageQuery) (servicechat.EventPage, error)
}
type AgentProjects interface {
	HasAccess(context.Context, serviceproject.ID, string) (bool, error)
}
type AgentIdentities interface {
	IsRegistered(context.Context, string) (bool, error)
	IsAdmin(context.Context, string) (bool, error)
}
type AgentPrompts interface {
	Start(prompt.StartInput, func(servicechat.Event)) (prompt.RunHandle, error)
}
type AgentDependencies struct {
	Chats      AgentChats
	Projects   AgentProjects
	Identities AgentIdentities
	Prompts    AgentPrompts
	Turns      AgentTurnRepository
}

// AgentTurnsFactory is implemented by the service and bound by the process host
// at composition time. The process adapter never decides execution authority.
type AgentTurnsFactory interface{ AgentTurnsForInstance(string) api.AgentTurns }
type AgentRuntimeHost interface{ SetAgentTurns(AgentTurnsFactory) }

type agentRequestKey struct{ instanceID, requestID string }

type agentRuntime struct {
	AgentDependencies
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	active  map[agentRequestKey]AgentTurnRecord
	wait    sync.WaitGroup
	service *Service
}

func WithAgentRuntime(ctx context.Context, deps AgentDependencies) Option {
	return func(s *Service) {
		ctx, cancel := context.WithCancel(ctx)
		s.agentRuntime = &agentRuntime{AgentDependencies: deps, ctx: ctx, cancel: cancel, active: map[agentRequestKey]AgentTurnRecord{}, service: s}
	}
}

type instanceAgentTurns struct {
	service    *Service
	instanceID string
}

func (s *Service) AgentTurnsForInstance(id string) api.AgentTurns { return &instanceAgentTurns{s, id} }
func (a *instanceAgentTurns) Start(in api.AgentTurnRequest) (api.AgentTurn, error) {
	return a.service.startAgentTurn(a.instanceID, in)
}
func (a *instanceAgentTurns) Read(in api.AgentTurnQuery) (api.AgentTurn, error) {
	return a.service.readAgentTurn(a.instanceID, in)
}
func (a *instanceAgentTurns) Forget(id string) error {
	return a.service.forgetAgentTurn(a.instanceID, id)
}

func (s *Service) agentInstallation(ctx context.Context, id string) (Instance, error) {
	i, a, err := s.load(ctx, id)
	if err != nil || i.Status != StatusRunning || a.Backend == nil || !a.Backend.AgentTurns {
		return Instance{}, api.ErrAgentAccess
	}
	return i, nil
}
func (r *agentRuntime) authorize(ctx context.Context, i Instance, chatID, email string) (prompt.Actor, error) {
	if r.Chats == nil || r.Identities == nil || email == "" {
		return prompt.Actor{}, api.ErrAgentAccess
	}
	meta, err := r.Chats.Get(ctx, servicechat.ID(chatID))
	if err != nil {
		return prompt.Actor{}, fmt.Errorf("%w: chat unavailable", api.ErrAgentAccess)
	}
	if i.Scope == ScopeProject && string(meta.ProjectID) != i.ProjectID {
		return prompt.Actor{}, api.ErrAgentAccess
	}
	registered, err := r.Identities.IsRegistered(ctx, email)
	if err != nil {
		return prompt.Actor{}, err
	}
	if !registered {
		return prompt.Actor{}, api.ErrAgentAccess
	}
	admin, err := r.Identities.IsAdmin(ctx, email)
	if err != nil {
		return prompt.Actor{}, err
	}
	if !admin && meta.ProjectID != "" {
		if r.Projects == nil {
			return prompt.Actor{}, api.ErrAgentAccess
		}
		allowed, err := r.Projects.HasAccess(ctx, serviceproject.ID(meta.ProjectID), email)
		if err != nil {
			return prompt.Actor{}, err
		}
		if !allowed {
			return prompt.Actor{}, api.ErrAgentAccess
		}
	}
	return prompt.Actor{Email: email, IsAdmin: admin}, nil
}
func (s *Service) startAgentTurn(id string, in api.AgentTurnRequest) (api.AgentTurn, error) {
	unlock, ok := s.instanceLocks.tryRLock(id)
	if !ok {
		return api.AgentTurn{}, api.ErrAgentBusy
	}
	defer unlock()
	r := s.agentRuntime
	if r == nil || r.Turns == nil || r.Prompts == nil {
		return api.AgentTurn{}, api.ErrAgentUnavailable
	}
	if err := r.ctx.Err(); err != nil {
		return api.AgentTurn{}, err
	}
	if strings.TrimSpace(in.ChatID) == "" || len(in.ChatID) > 256 || strings.TrimSpace(in.RequestID) == "" || len(in.RequestID) > 256 || strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 32<<10 || len(in.Context) > 64<<10 || (len(in.Context) > 0 && !json.Valid(in.Context)) {
		return api.AgentTurn{}, errors.New("requestId, chatId and prompt required; prompt/context exceed limits or context is invalid JSON")
	}
	in.OwnerEmail = strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	in.Context = bytes.Clone(in.Context)
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	defer cancel()
	i, err := s.agentInstallation(ctx, id)
	if err != nil {
		return api.AgentTurn{}, err
	}
	actor, err := r.authorize(ctx, i, in.ChatID, in.OwnerEmail)
	if err != nil {
		return api.AgentTurn{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return api.AgentTurn{}, err
	}
	key := agentRequestKey{id, in.RequestID}
	record, active := r.active[key]
	if !active {
		var exists bool
		record, exists, err = r.Turns.Get(ctx, id, in.RequestID)
		if err != nil {
			return api.AgentTurn{}, err
		}
		if !exists {
			record = AgentTurnRecord{Request: in}
		}
	}
	if !sameAgentRequest(record.Request, in) {
		return api.AgentTurn{}, api.ErrAgentRequestChanged
	}
	if active || record.Turn.Status == "succeeded" || record.Turn.Status == "failed" {
		return record.Turn, nil
	}
	if len(r.active) >= 2 {
		return api.AgentTurn{}, api.ErrAgentBusy
	}
	record.Turn = api.AgentTurn{RequestID: in.RequestID, ChatID: in.ChatID, Status: "running"}
	if err := r.Turns.Put(ctx, id, record); err != nil {
		return api.AgentTurn{}, err
	}
	run, err := r.Prompts.Start(prompt.StartInput{
		ChatID: servicechat.ID(in.ChatID), Prompt: in.Prompt, Actor: actor, ParentContext: r.ctx,
		ApplicationInstanceID: id, ApplicationID: i.ApplicationID, ApplicationRequestID: in.RequestID, ApplicationContext: in.Context,
	}, nil)
	if err != nil {
		if removeErr := r.Turns.Delete(ctx, id, in.RequestID); removeErr != nil {
			return api.AgentTurn{}, removeErr
		}
		if errors.Is(err, prompt.ErrPromptAlreadyRunning) || errors.Is(err, prompt.ErrMaintenance) {
			return api.AgentTurn{}, api.ErrAgentBusy
		}
		return api.AgentTurn{}, err
	}
	record.Turn.TurnID = run.TurnID
	r.active[key] = record
	r.wait.Add(1)
	go r.await(id, key, record, run)
	// The pending receipt already fences restart recovery; retain admission if
	// recording the provider's transcript identity fails after the turn started.
	if err := r.Turns.Put(ctx, id, record); err != nil {
		return api.AgentTurn{}, err
	}
	return record.Turn, nil
}
func sameAgentRequest(a, b api.AgentTurnRequest) bool {
	return a.RequestID == b.RequestID && a.ChatID == b.ChatID && a.OwnerEmail == b.OwnerEmail && a.Prompt == b.Prompt && bytes.Equal(a.Context, b.Context)
}
func (r *agentRuntime) await(id string, key agentRequestKey, record AgentTurnRecord, run prompt.RunHandle) {
	defer r.wait.Done()
	defer func() { r.mu.Lock(); delete(r.active, key); r.mu.Unlock() }()
	select {
	case <-r.ctx.Done():
		return
	case result, ok := <-run.Done:
		if !ok {
			result.Err = errors.New("agent run ended without a result")
		}
		record.Turn.Status = "succeeded"
		record.Turn.Output = result.Output
		if result.Err != nil {
			record.Turn.Status = "failed"
			record.Turn.Error = result.Err.Error()
		}
		for {
			r.mu.Lock()
			_, _, existsErr := r.service.load(r.ctx, id)
			if errors.Is(existsErr, ErrNotFound) {
				r.mu.Unlock()
				return
			}
			err := r.Turns.Put(r.ctx, id, record)
			r.mu.Unlock()
			if err == nil {
				return
			}
			select {
			case <-r.ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}
func (s *Service) readAgentTurn(id string, query api.AgentTurnQuery) (api.AgentTurn, error) {
	unlock, ok := s.instanceLocks.tryRLock(id)
	if !ok {
		return api.AgentTurn{}, api.ErrAgentBusy
	}
	defer unlock()
	r := s.agentRuntime
	if r == nil || r.Turns == nil {
		return api.AgentTurn{}, api.ErrAgentUnavailable
	}
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	defer cancel()
	i, err := s.agentInstallation(ctx, id)
	if err != nil {
		return api.AgentTurn{}, err
	}
	r.mu.Lock()
	record, active := r.active[agentRequestKey{id, query.RequestID}]
	if !active {
		var found bool
		record, found, err = r.Turns.Get(ctx, id, query.RequestID)
		if err == nil && !found {
			err = api.ErrAgentTurnNotFound
		}
	}
	r.mu.Unlock()
	if err != nil {
		return api.AgentTurn{}, err
	}
	if _, err := r.authorize(ctx, i, record.Request.ChatID, record.Request.OwnerEmail); err != nil {
		return api.AgentTurn{}, err
	}
	turn := record.Turn
	if !active && turn.Status == "running" {
		turn.Status = "interrupted"
	}
	if turn.TurnID == "" {
		return turn, nil
	}
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	before := query.BeforeSeq
	for {
		page, err := r.Chats.EventPage(ctx, servicechat.ID(turn.ChatID), servicechat.EventPageQuery{Limit: 200, BeforeSeq: before})
		if err != nil {
			return api.AgentTurn{}, err
		}
		for n := len(page.Events) - 1; n >= 0; n-- {
			event := page.Events[n]
			if event.TurnID != turn.TurnID {
				continue
			}
			if len(turn.Events) == limit {
				turn.HasMore = true
				break
			}
			data, err := json.Marshal(event)
			if err != nil {
				return api.AgentTurn{}, err
			}
			turn.Events = append(turn.Events, data)
			turn.NextBefore = event.Seq
		}
		if turn.HasMore || !page.HasMore || page.NextBefore == 0 {
			break
		}
		before = page.NextBefore
	}
	// Transcript pages use chronological order, like the browser history API.
	for l, h := 0, len(turn.Events)-1; l < h; l, h = l+1, h-1 {
		turn.Events[l], turn.Events[h] = turn.Events[h], turn.Events[l]
	}
	return turn, nil
}
func (s *Service) forgetAgentTurn(id, requestID string) error {
	unlock, ok := s.instanceLocks.tryRLock(id)
	if !ok {
		return api.ErrAgentBusy
	}
	defer unlock()
	r := s.agentRuntime
	if r == nil || r.Turns == nil {
		return api.ErrAgentUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := s.agentInstallation(r.ctx, id); err != nil {
		return err
	}
	if _, active := r.active[agentRequestKey{id, requestID}]; active {
		return api.ErrAgentBusy
	}
	return r.Turns.Delete(r.ctx, id, requestID)
}
