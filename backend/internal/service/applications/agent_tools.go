package applications

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	api "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

var ErrInvalidAgentGrant = errors.New("invalid or expired application capability")

type applicationGrant struct {
	request   prompt.ApplicationToolRequest
	instances map[string]string
	expires   time.Time
}
type applicationTools struct {
	mu     sync.Mutex
	apiURL string
	grants map[string]applicationGrant
	now    func() time.Time
}

func WithAgentToolURL(baseURL string) Option {
	return func(s *Service) {
		s.agentTools = &applicationTools{apiURL: strings.TrimRight(baseURL, "/") + "/agent-api/applications", grants: map[string]applicationGrant{}, now: time.Now}
	}
}

// IssueApplicationTools grants a turn access only to running, explicitly
// opted-in installations visible to that chat. A background application's
// tools are limited to its own installation; workflow permissions stay in it.
func (s *Service) IssueApplicationTools(ctx context.Context, request prompt.ApplicationToolRequest) (prompt.ApplicationToolAccess, error) {
	if s.agentTools == nil || s.agentRuntime == nil {
		return prompt.ApplicationToolAccess{}, nil
	}
	if request.Actor.Email == "" {
		return prompt.ApplicationToolAccess{}, nil
	}
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return prompt.ApplicationToolAccess{}, err
	}
	candidates := []Instance{}
	for _, i := range instances {
		if i.Status != StatusRunning || (i.Scope == ScopeProject && i.ProjectID != string(request.ProjectID)) || (request.ApplicationInstanceID != "" && i.ID != request.ApplicationInstanceID) {
			continue
		}
		_, app, err := s.load(ctx, i.ID)
		if err == nil && app.Backend != nil && app.Backend.AgentTools {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return prompt.ApplicationToolAccess{}, nil
	}
	actor, err := s.agentRuntime.authorize(ctx, Instance{Scope: ScopeProject, ProjectID: string(request.ProjectID)}, string(request.ChatID), strings.ToLower(strings.TrimSpace(request.Actor.Email)))
	if err != nil {
		return prompt.ApplicationToolAccess{}, err
	}
	request.Actor = actor
	request.Context = append([]byte(nil), request.Context...)
	grant := applicationGrant{request: request, instances: map[string]string{}, expires: s.agentTools.now().Add(4 * time.Hour)}
	access := prompt.ApplicationToolAccess{}
	for _, i := range candidates {
		if i.Status != StatusRunning || (i.Scope == ScopeProject && i.ProjectID != string(request.ProjectID)) {
			continue
		}
		if request.ApplicationInstanceID != "" && i.ID != request.ApplicationInstanceID {
			continue
		}
		_, app, err := s.load(ctx, i.ID)
		if err != nil || app.Backend == nil || !app.Backend.AgentTools {
			continue
		}
		if app.Backend.Audience() == BackendAccessAdmin && !actor.IsAdmin {
			continue
		}
		if existing, ok := grant.instances[app.ID]; ok {
			prior, _, err := s.load(ctx, existing)
			if err == nil && prior.Scope == ScopeProject {
				continue
			}
		}
		grant.instances[app.ID] = i.ID

	}
	if len(grant.instances) == 0 {
		return access, nil
	}
	ids := make([]string, 0, len(grant.instances))
	for id := range grant.instances {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if app, ok := s.registry.Get(id); ok {
			for _, name := range app.Skills {
				access.Skills = append(access.Skills, servicechat.SkillRef{Name: name, Command: name, Source: "remote"})
			}
		}
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return access, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	tools := s.agentTools
	tools.mu.Lock()
	for key, g := range tools.grants {
		if !g.expires.After(tools.now()) {
			delete(tools.grants, key)
		}
	}
	tools.grants[token] = grant
	tools.mu.Unlock()
	access.Env = map[string]string{"REMOTE_APPLICATION_API": tools.apiURL, "REMOTE_APPLICATION_GRANT": token}
	access.Revoke = func() { tools.mu.Lock(); delete(tools.grants, token); tools.mu.Unlock() }
	return access, nil
}

func (s *Service) CallAgentApplication(ctx context.Context, token, applicationID string, request api.Request) (api.Response, error) {
	if s.agentTools == nil || s.agentRuntime == nil {
		return api.Response{}, ErrInvalidAgentGrant
	}
	tools := s.agentTools
	tools.mu.Lock()
	grant, ok := tools.grants[token]
	if ok && !grant.expires.After(tools.now()) {
		delete(tools.grants, token)
		ok = false
	}
	tools.mu.Unlock()
	if !ok {
		return api.Response{}, ErrInvalidAgentGrant
	}
	id, ok := grant.instances[applicationID]
	if !ok {
		return api.Response{}, api.ErrAgentAccess
	}
	instance, app, err := s.load(ctx, id)
	if err != nil || instance.Status != StatusRunning || app.Backend == nil || !app.Backend.AgentTools {
		return api.Response{}, api.ErrAgentAccess
	}
	actor, err := s.agentRuntime.authorize(ctx, instance, string(grant.request.ChatID), grant.request.Actor.Email)
	if err != nil {
		return api.Response{}, err
	}
	request.Agent = &api.AgentContext{ChatID: string(grant.request.ChatID)}
	if id == grant.request.ApplicationInstanceID {
		request.Agent.Background = true
		request.Agent.RequestID = grant.request.ApplicationRequestID
		request.Agent.Context = append([]byte(nil), grant.request.Context...)
	}
	return s.callBackend(ctx, id, request, api.Caller{Email: actor.Email, IsAdmin: actor.IsAdmin})
}
