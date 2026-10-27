package applications_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"futrx.local/catalog"
	hostapps "github.com/futrx-com/remote.futrx.com/internal/integration/applications"
	catalogapps "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	"github.com/futrx-com/remote.futrx.com/internal/lifecycle"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	api "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type agentBackendSource struct{ source fs.FS }

func (c agentBackendSource) BackendSource(string) (fs.FS, bool) { return c.source, true }

const agentBackendMain = `package main
import("encoding/json";"errors";"github.com/futrx-com/remote.futrx.com/pkg/applications";"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc")
type backend struct{turns applications.AgentTurns}
func main(){rpc.ServeWithRuntime(func(r applications.Runtime)applications.Backend{return &backend{turns:r.AgentTurns}})}
func(b *backend) Describe()(applications.Descriptor,error){return applications.Descriptor{APIVersion:applications.APIVersion},nil}
func(b *backend) Init(applications.Instance)error{return nil}
func(b *backend) Handle(r applications.Request)(applications.Response,error){
 if r.Path=="read"{turn,err:=b.turns.Read(applications.AgentTurnQuery{RequestID:"job"});if errors.Is(err,applications.ErrAgentTurnNotFound){return applications.JSON(404,"missing"),nil};if err!=nil{return applications.Response{},err};return applications.JSON(200,turn),nil}
 var request applications.AgentTurnRequest;if err:=json.Unmarshal(r.Body,&request);err!=nil{return applications.Response{},err}
 turn,err:=b.turns.Start(request);if errors.Is(err,applications.ErrAgentAccess){return applications.JSON(403,"denied"),nil};if err!=nil{return applications.Response{},err};return applications.JSON(200,turn),nil
}`

func TestIndependentApplicationStartsAndReadsThroughGeneratedSDK(t *testing.T) {
	f := newRuntimeFixture(t)
	s := f.service(t)
	source := fstest.MapFS{"main.go": &fstest.MapFile{Data: []byte(agentBackendMain)}}
	host := hostapps.New(t.TempDir(), agentBackendSource{source}, hostapps.Options{AgentTurns: s})
	defer host.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	instance := api.Instance{ID: "build-monitor", ApplicationID: "build-monitor", Scope: "project", ProjectID: "project", AgentTurns: true}
	if _, err := host.Ensure(ctx, instance); err != nil {
		t.Fatal(err)
	}
	missing, err := host.Call(ctx, instance, api.Request{Method: "GET", Path: "read"})
	if err != nil || missing.Status != 404 {
		t.Fatalf("typed reverse RPC error: %+v %v", missing, err)
	}
	input := agentInput("job")
	body, _ := json.Marshal(input)
	response, err := host.Call(ctx, instance, api.Request{Method: "POST", Path: "start", Body: body})
	if err != nil || response.Status != 200 {
		t.Fatalf("start: %+v %v", response, err)
	}
	f.prompts.finish(0, prompt.RunResult{Output: "build checked"})
	eventually(t, func() bool {
		response, err := host.Call(ctx, instance, api.Request{Method: "GET", Path: "read"})
		var turn api.AgentTurn
		_ = json.Unmarshal(response.Body, &turn)
		return err == nil && turn.Status == "succeeded" && turn.Output == "build checked"
	})
	input.ChatID = "other"
	body, _ = json.Marshal(input)
	denied, err := host.Call(ctx, instance, api.Request{Method: "POST", Path: "start", Body: body})
	if err != nil || denied.Status != 403 {
		t.Fatalf("typed authorization rejection: %+v %v", denied, err)
	}
}

func TestPackagedSchedulerOwnsDeliveryAcrossChildRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	registry, err := catalogapps.NewRegistry(catalog.FS, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := newRuntimeFixture(t)
	f.store.instances = map[string]svc.Instance{"scheduler": {ID: "scheduler", ApplicationID: "scheduled-tasks", Scope: svc.ScopeProject, ProjectID: "project", Status: svc.StatusRunning}}
	events := lifecycle.NewEventBus(ctx)
	defer events.Close()
	host := hostapps.New(t.TempDir(), registry, hostapps.Options{Events: events})
	defer host.Shutdown()
	s := svc.New(registry, f.store, nil, nil, nil, svc.WithBackendHost(host), svc.WithAgentRuntime(ctx, svc.AgentDependencies{Chats: runtimeChats{}, Projects: f.access, Identities: f.access, Prompts: f.prompts, Turns: f.repo}), svc.WithAgentToolURL("https://remote.test"))
	defer s.Close()
	host.SetAgentTurns(s)
	s.RestoreBackground(ctx)
	access, err := s.IssueApplicationTools(ctx, prompt.ApplicationToolRequest{Actor: prompt.Actor{Email: "owner@example.com"}, ChatID: "chat", ProjectID: "project"})
	if err != nil {
		t.Fatal(err)
	}
	defer access.Revoke()
	body, _ := json.Marshal(map[string]any{"name": "remember", "prompt": "Check build_123", "chatId": "forged", "kind": "once", "at": time.Now().Add(3 * time.Second).UTC().Format(time.RFC3339Nano)})
	response, err := s.CallAgentApplication(ctx, access.Env["REMOTE_APPLICATION_GRANT"], "scheduled-tasks", api.Request{Method: "POST", Path: "tasks", Body: body})
	if err != nil || response.Status != 201 {
		t.Fatalf("create: %d %s %v", response.Status, response.Body, err)
	}
	var task struct{ ID, ChatID string }
	_ = json.Unmarshal(response.Body, &task)
	if task.ChatID != "chat" {
		t.Fatal("application failed to bind chat context")
	}
	if err := host.Stop(ctx, "scheduler"); err != nil {
		t.Fatal(err)
	}
	s.RestoreBackground(ctx)
	eventually(t, func() bool { return f.prompts.count() == 1 })
	f.prompts.mu.Lock()
	input := f.prompts.inputs[0]
	f.prompts.mu.Unlock()
	if input.ApplicationID != "scheduled-tasks" || input.ApplicationRequestID == "" || input.ChatID != "chat" || input.Actor.Email != "owner@example.com" {
		t.Fatalf("origin: %+v", input)
	}
	// A child crash while Remote executes the turn must reconnect to its receipt.
	if err := host.Stop(ctx, "scheduler"); err != nil {
		t.Fatal(err)
	}
	s.RestoreBackground(ctx)
	f.prompts.finish(0, prompt.RunResult{Output: "reminder delivered"})
	eventually(t, func() bool {
		response, err := s.CallBackend(ctx, "scheduler", api.Request{Method: "GET", Path: "tasks/" + task.ID}, api.Caller{Email: "owner@example.com"})
		var task struct {
			Archived bool
			RunCount int
		}
		_ = json.Unmarshal(response.Body, &task)
		return err == nil && task.Archived && task.RunCount == 1
	})
	if f.prompts.count() != 1 {
		t.Fatal("child restart duplicated accepted work")
	}
}
