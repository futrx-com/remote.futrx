package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type recordingEvents struct {
	calls [][2]string
	err   error
}

func (e *recordingEvents) Due(task, run string) error {
	e.calls = append(e.calls, [2]string{task, run})
	return e.err
}
func testAPI(t *testing.T, e *recordingEvents) *API {
	t.Helper()
	a := New(e, nil)
	a.instance = applications.Instance{Scope: "project", ProjectID: "project", DataDir: t.TempDir()}
	a.now = func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }
	return a
}
func call(t *testing.T, a *API, method, path string, body any, caller applications.Caller) applications.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := a.Handle(applications.Request{Method: method, Path: path, Body: data, Caller: caller})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

var owner = applications.Caller{Email: "owner@example.com"}

func createOnce(t *testing.T, a *API) Task {
	t.Helper()
	r := call(t, a, "POST", "tasks", map[string]any{"name": "reminder", "prompt": "Remind me to review build_123", "chatId": "aabbcc11", "kind": "once", "at": "2026-10-06T12:00:05Z"}, owner)
	if r.Status != 201 {
		t.Fatalf("create: %d %s", r.Status, r.Body)
	}
	var task Task
	if err := json.Unmarshal(r.Body, &task); err != nil {
		t.Fatal(err)
	}
	return task
}
func TestCreateActiveTaskPersistsAndFiresAtRequestedTime(t *testing.T) {
	events := &recordingEvents{}
	a := testAPI(t, events)
	task := createOnce(t, a)
	if !task.Enabled || task.NextRunAt == 0 {
		t.Fatalf("task parked: %+v", task)
	}
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
	if len(events.calls) != 0 {
		t.Fatal("fired early")
	}
	now := a.now().Add(5 * time.Second)
	a.now = func() time.Time { return now }
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
	if len(events.calls) != 1 || events.calls[0][0] != task.ID {
		t.Fatalf("events: %+v", events.calls)
	}
	var saved map[string]Task
	raw, err := os.ReadFile(a.filename())
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	run := events.calls[0][1]
	if saved[task.ID].ActiveRunID != run || run == "" {
		t.Fatal("published before claim was durable")
	}
	if err := a.finish(task.ID, run, applications.AgentTurn{Status: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	if a.tasks[task.ID].Enabled || a.tasks[task.ID].RunCount != 1 {
		t.Fatal("one-time task did not finish")
	}
}
func TestRejectInvalidDefinitionAndKeepOwnerIsolation(t *testing.T) {
	a := testAPI(t, &recordingEvents{})
	for _, input := range []map[string]any{
		{"name": "missing prompt", "kind": "once"},
		{"name": "past", "prompt": "message", "chatId": "chat", "kind": "once", "at": "2026-10-05T12:00:00Z"},
		{"name": "cron", "prompt": "message", "chatId": "chat", "kind": "cron", "cron": "60 * * * *"},
		{"name": "timezone", "prompt": "message", "chatId": "chat", "kind": "cron", "cron": "0 15 * * *", "timezone": "invalid"},
	} {
		if r := call(t, a, "POST", "tasks", input, owner); r.Status != 400 {
			t.Fatalf("accepted invalid definition: %s", r.Body)
		}
	}
	task := createOnce(t, a)
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		if r := call(t, a, method, "tasks/"+task.ID, map[string]bool{"enabled": true}, applications.Caller{Email: "other@example.com"}); r.Status != 404 {
			t.Fatalf("%s crossed ownership", method)
		}
	}
	if r := call(t, a, "POST", "tasks/"+task.ID+"/finish", map[string]string{"runId": "forged"}, applications.Caller{IsAdmin: true, Email: owner.Email}); r.Status != 405 {
		t.Fatal("removed core acknowledgment route is still exposed")
	}
}
func TestFailedPublicationRetriesSameDurableClaimAfterRestart(t *testing.T) {
	events := &recordingEvents{err: errors.New("bus unavailable")}
	a := testAPI(t, events)
	task := createOnce(t, a)
	now := a.now().Add(5 * time.Second)
	a.now = func() time.Time { return now }
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(a.filename())
	if err != nil {
		t.Fatal(err)
	}
	restarted := testAPI(t, &recordingEvents{})
	restarted.instance = a.instance
	restarted.now = a.now
	if err = json.Unmarshal(raw, &restarted.tasks); err != nil {
		t.Fatal(err)
	}
	if err = restarted.tick(); err != nil {
		t.Fatal(err)
	}
	retry := restarted.events.(*recordingEvents).calls
	if len(retry) != 1 || retry[0][0] != task.ID || retry[0][1] != events.calls[0][1] {
		t.Fatalf("claim changed on retry: %+v", retry)
	}
}
func TestWriteFailureDoesNotAcknowledgeTaskCreation(t *testing.T) {
	a := testAPI(t, &recordingEvents{})
	file := filepath.Join(a.instance.DataDir, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	a.instance.DataDir = file
	r := call(t, a, "POST", "tasks", map[string]any{"name": "reminder", "prompt": "message", "chatId": "chat", "kind": "once", "at": "2026-10-06T12:00:05Z"}, owner)
	if r.Status != 500 || len(a.tasks) != 0 {
		t.Fatal("failed write was acknowledged")
	}
}
func TestRecurringTaskReschedulesAndStopsAtMaxRuns(t *testing.T) {
	events := &recordingEvents{}
	a := testAPI(t, events)
	r := call(t, a, "POST", "tasks", map[string]any{"name": "daily", "prompt": "remind me", "chatId": "chat", "kind": "cron", "cron": "0 15 * * *", "timezone": "UTC", "maxRuns": 2}, owner)
	if r.Status != 201 {
		t.Fatalf("create: %s", r.Body)
	}
	var task Task
	_ = json.Unmarshal(r.Body, &task)
	for n := 1; n <= 2; n++ {
		now := time.UnixMilli(a.tasks[task.ID].NextRunAt)
		a.now = func() time.Time { return now }
		if err := a.tick(); err != nil {
			t.Fatal(err)
		}
		run := a.tasks[task.ID].ActiveRunID
		if err := a.finish(task.ID, run, applications.AgentTurn{Status: "succeeded"}); err != nil {
			t.Fatal(err)
		}
		if got := a.tasks[task.ID]; got.RunCount != n || got.Enabled != (n < 2) {
			t.Fatalf("run %d: %+v", n, got)
		}
	}
}
func TestInitRejectsCorruptStorageAndGlobalScope(t *testing.T) {
	a := New(&recordingEvents{}, nil)
	if err := a.Init(applications.Instance{Scope: "global", DataDir: t.TempDir()}); err == nil {
		t.Fatal("accepted global scope")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tasks.json"), []byte("broken JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Init(applications.Instance{Scope: "project", ProjectID: "project", DataDir: dir}); err == nil {
		t.Fatal("silently reset corrupt storage")
	}
}

func TestArchivePersistsPausesAndPreservesPendingRun(t *testing.T) {
	a := testAPI(t, &recordingEvents{})
	task := createOnce(t, a)
	call(t, a, "POST", "tasks/"+task.ID+"/run", nil, owner)
	run := a.tasks[task.ID].ActiveRunID
	r := call(t, a, "PATCH", "tasks/"+task.ID, map[string]bool{"archived": true}, owner)
	if r.Status != 200 {
		t.Fatalf("archive: %s", r.Body)
	}
	got := a.tasks[task.ID]
	if !got.Archived || got.Enabled || got.NextRunAt != 0 || got.ActiveRunID != run {
		t.Fatalf("archive: %+v", got)
	}
	raw, err := os.ReadFile(a.filename())
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]Task
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved[task.ID].Archived {
		t.Fatal("archive not durable")
	}
	for _, method := range []string{"PATCH", "POST"} {
		path := "tasks/" + task.ID
		if method == "POST" {
			path += "/run"
		}
		if r := call(t, a, method, path, map[string]bool{"enabled": true}, owner); r.Status != 409 {
			t.Fatalf("archived task allowed %s: %d", method, r.Status)
		}
	}
	if r := call(t, a, "PATCH", "tasks/"+task.ID, map[string]bool{"archived": false}, owner); r.Status != 200 {
		t.Fatal(string(r.Body))
	}
	if a.tasks[task.ID].Archived || a.tasks[task.ID].Enabled {
		t.Fatal("restore must remain paused")
	}
}

func TestArchiveRejectsUnauthorizedInvalidAndFailedWrites(t *testing.T) {
	a := testAPI(t, &recordingEvents{})
	task := createOnce(t, a)
	path := "tasks/" + task.ID
	if r := call(t, a, "PATCH", path, map[string]bool{"archived": true}, applications.Caller{Email: "other@example.com"}); r.Status != 404 {
		t.Fatal("archive crossed ownership")
	}
	if r := call(t, a, "PATCH", path, map[string]string{"archived": "yes"}, owner); r.Status != 400 {
		t.Fatal("accepted invalid archive")
	}
	a.instance.DataDir = filepath.Join(a.instance.DataDir, "missing")
	if r := call(t, a, "PATCH", path, map[string]bool{"archived": true}, owner); r.Status != 500 {
		t.Fatal("acknowledged failed archive write")
	}
	if a.tasks[task.ID].Archived || !a.tasks[task.ID].Enabled {
		t.Fatal("failed write mutated task")
	}
}

func TestBusyRunRetriesThenFinishesAndArchives(t *testing.T) {
	events := &recordingEvents{}
	a := testAPI(t, events)
	turns := &fakeTurns{err: applications.ErrAgentBusy}
	a.turns = turns
	task := createOnce(t, a)
	call(t, a, "POST", "tasks/"+task.ID+"/run", nil, owner)
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
	run := a.tasks[task.ID].ActiveRunID
	if run == "" || a.tasks[task.ID].RunCount != 0 {
		t.Fatal("busy run lost its durable claim")
	}
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
	if len(turns.requests) != 1 {
		t.Fatal("retried early")
	}
	now := a.now().Add(15 * time.Second)
	a.now = func() time.Time { return now }
	turns.err = nil
	turns.turn = applications.AgentTurn{Status: "succeeded"}
	if err := a.tick(); err != nil {
		t.Fatal(err)
	}
	got := a.tasks[task.ID]
	if len(turns.requests) != 2 || turns.requests[1].RequestID != run || got.ActiveRunID != "" || !got.Archived || got.RunCount != 1 {
		t.Fatalf("retry/finish: %+v", got)
	}
}

type fakeTurns struct {
	requests  []applications.AgentTurnRequest
	turn      applications.AgentTurn
	err       error
	forgotten []string
}

func (f *fakeTurns) Start(in applications.AgentTurnRequest) (applications.AgentTurn, error) {
	f.requests = append(f.requests, in)
	return f.turn, f.err
}
func (f *fakeTurns) Read(applications.AgentTurnQuery) (applications.AgentTurn, error) {
	return f.turn, f.err
}
func (f *fakeTurns) Forget(id string) error { f.forgotten = append(f.forgotten, id); return nil }

func TestAcknowledgmentFailureReplaysReceiptBeforeForgetting(t *testing.T) {
	a := testAPI(t, &recordingEvents{})
	turns := &fakeTurns{turn: applications.AgentTurn{Status: "succeeded", Output: "TASK_COMPLETE"}}
	a.turns = turns
	task := createOnce(t, a)
	call(t, a, "POST", "tasks/"+task.ID+"/run", nil, owner)
	task = a.tasks[task.ID]
	original := a.instance.DataDir
	a.instance.DataDir = filepath.Join(original, "missing")
	if err := a.execute(task); err == nil {
		t.Fatal("acknowledged failed task write")
	}
	if a.tasks[task.ID].ActiveRunID != task.ActiveRunID || len(turns.forgotten) != 0 {
		t.Fatal("forgot receipt before durable acknowledgment")
	}
	a.instance.DataDir = original
	if err := a.execute(task); err != nil {
		t.Fatal(err)
	}
	if a.tasks[task.ID].RunCount != 1 || len(turns.forgotten) != 1 || turns.requests[0].RequestID != turns.requests[1].RequestID {
		t.Fatal("receipt was not reused")
	}
}
func TestApplicationEnforcesCompletionOnlyAndChatScope(t *testing.T) {
	a := testAPI(t, &recordingEvents{})
	task := createOnce(t, a)
	call(t, a, "POST", "tasks/"+task.ID+"/run", nil, owner)
	task = a.tasks[task.ID]
	context, _ := json.Marshal(map[string]string{"taskId": task.ID, "runId": task.ActiveRunID})
	agent := &applications.AgentContext{ChatID: task.ChatID, Background: true, RequestID: task.ActiveRunID, Context: context}
	for _, path := range []string{"tasks", "tasks/" + task.ID + "/run"} {
		r, _ := a.Handle(applications.Request{Method: "POST", Path: path, Caller: owner, Agent: agent})
		if r.Status != 403 {
			t.Fatal("background turn gained management access")
		}
	}
	foreign := *agent
	foreign.ChatID = "another-chat"
	r, _ := a.Handle(applications.Request{Method: "POST", Path: "tasks/current/complete", Caller: owner, Agent: &foreign})
	if r.Status != 404 {
		t.Fatal("completion crossed chat")
	}
	r, _ = a.Handle(applications.Request{Method: "POST", Path: "tasks/current/complete", Caller: owner, Agent: agent})
	if r.Status != 200 || !a.tasks[task.ID].Archived {
		t.Fatalf("complete current: %d %s", r.Status, r.Body)
	}
	if _, err := a.Handle(applications.Request{Method: "GET", Path: "tasks", Caller: owner, Agent: &foreign}); err != nil {
		t.Fatal(err)
	}
}
func TestRecurringCompletionMarkersAreApplicationOwned(t *testing.T) {
	for _, output := range []string{"done\nTASK_COMPLETE\n", "done\nSCHEDULE_STATUS=COMPLETE\n"} {
		a := testAPI(t, &recordingEvents{})
		r := call(t, a, "POST", "tasks", map[string]any{"name": "monitor", "prompt": "check deployment", "chatId": "chat", "kind": "cron", "cron": "* * * * *", "timezone": "UTC"}, owner)
		var task Task
		_ = json.Unmarshal(r.Body, &task)
		call(t, a, "POST", "tasks/"+task.ID+"/run", nil, owner)
		if err := a.finish(task.ID, a.tasks[task.ID].ActiveRunID, applications.AgentTurn{Status: "succeeded", Output: output}); err != nil {
			t.Fatal(err)
		}
		if !a.tasks[task.ID].Archived || a.tasks[task.ID].Enabled {
			t.Fatal("completion marker did not stop recurrence")
		}
	}
}
