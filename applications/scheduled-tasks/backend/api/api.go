package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"time"

	"futrx.local/catalog/applications/scheduled-tasks/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const (
	maxTasks       = 100
	maxPromptBytes = 32 << 10
	retryInterval  = 15 * time.Second
)

type API struct {
	taskStore
	events lifecycle.TaskEvents
	turns  applications.AgentTurns
	now    func() time.Time
	router *applications.Router
	// retry deadlines are volatile; claims themselves are durable.
	retry map[string]time.Time
}

func New(events lifecycle.TaskEvents, turns applications.AgentTurns) *API {
	a := &API{events: events, turns: turns, now: time.Now, taskStore: taskStore{tasks: map[string]Task{}}, retry: map[string]time.Time{}, router: applications.NewRouter()}
	a.router.GET("health", "Scheduler health", a.health)
	a.router.GET("tasks", "List tasks owned by the caller", a.list)
	a.router.POST("tasks", "Create an active schedule", a.create)
	a.router.Handle("*", "tasks/*", "Read, pause, resume, delete or run a task", a.task)
	return a
}
func (a *API) Describe() (applications.Descriptor, error) {
	return applications.Descriptor{APIVersion: applications.APIVersion, Routes: a.router.Routes()}, nil
}
func (a *API) Init(instance applications.Instance) error {
	if instance.Scope != "project" || instance.ProjectID == "" || instance.DataDir == "" {
		return errors.New("scheduled tasks require a project instance and data directory")
	}
	if a.turns == nil {
		return errors.New("scheduled tasks require the application agent runtime")
	}
	if err := a.taskStore.load(instance); err != nil {
		return err
	}
	go a.loop()
	return nil
}
func (a *API) Handle(r applications.Request) (applications.Response, error) {
	if r.Agent != nil {
		if r.Path == "tasks/current/complete" {
			return a.completeCurrent(r), nil
		}
		if r.Agent.Background && r.Method != "GET" {
			return applications.Errorf(403, "scheduled turns may only complete their own task"), nil
		}
	}
	return a.router.Serve(r), nil
}
func (a *API) health(applications.Request) applications.Response {
	return applications.JSON(200, map[string]bool{"ok": true})
}
func (a *API) list(r applications.Request) applications.Response {
	a.mu.Lock()
	defer a.mu.Unlock()
	tasks := []Task{}
	for _, t := range a.tasks {
		if authorized(t, r.Caller) && (r.Agent == nil || r.Agent.ChatID == t.ChatID) && (r.QueryValue("chatId") == "" || r.QueryValue("chatId") == t.ChatID) {
			tasks = append(tasks, t)
		}
	}
	return applications.JSON(200, tasks)
}
func authorized(t Task, c applications.Caller) bool {
	return c.IsAdmin || (c.Email != "" && strings.EqualFold(t.OwnerEmail, c.Email))
}
func (a *API) create(r applications.Request) applications.Response {
	if strings.TrimSpace(r.Caller.Email) == "" {
		return applications.Errorf(403, "authenticated task owner required")
	}
	var t Task
	if err := json.Unmarshal(r.Body, &t); err != nil {
		return applications.Errorf(400, "invalid JSON")
	}
	if r.Agent != nil {
		t.ChatID = r.Agent.ChatID
	}
	t.Name = strings.TrimSpace(t.Name)
	t.Prompt = strings.TrimSpace(t.Prompt)
	if t.Name == "" || t.Prompt == "" || len(t.Prompt) > maxPromptBytes || t.ChatID == "" || t.MaxRuns < 0 {
		return applications.Errorf(400, "name, chatId and prompt are required; prompt must be at most 32 KiB and maxRuns nonnegative")
	}
	if t.Timezone == "" {
		t.Timezone = "UTC"
	}
	t.OwnerEmail = strings.ToLower(strings.TrimSpace(r.Caller.Email))
	t.Enabled = true
	t.Archived = false
	t.RunCount = 0
	t.ActiveRunID = ""
	t.LastError = ""
	t.LastRunAt = 0
	next, err := nextOccurrence(t, a.now())
	if err != nil {
		return applications.Errorf(400, "%s", err)
	}
	t.NextRunAt = next.UnixMilli()
	t.ID, err = newID()
	if err != nil {
		return applications.Errorf(500, "%s", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.tasks) >= maxTasks {
		return applications.Errorf(409, "project task limit reached (100)")
	}
	updated := maps.Clone(a.tasks)
	updated[t.ID] = t
	if err = a.commit(updated); err != nil {
		return applications.Errorf(500, "%s", err)
	}
	return applications.JSON(201, t)
}
func (a *API) task(r applications.Request) applications.Response {
	parts := strings.Split(strings.TrimPrefix(r.Path, "tasks/"), "/")
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if len(parts) > 2 {
		return applications.Errorf(404, "not found")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.tasks[id]
	if !ok || !authorized(t, r.Caller) || (r.Agent != nil && r.Agent.ChatID != t.ChatID) {
		return applications.Errorf(404, "task not found")
	}
	if r.Method == "GET" && action == "" {
		return applications.JSON(200, t)
	}
	updated := maps.Clone(a.tasks)
	switch {
	case r.Method == "DELETE" && action == "":
		delete(updated, id)
	case r.Method == "PATCH" && action == "":
		var in struct {
			Enabled  *bool `json:"enabled"`
			Archived *bool `json:"archived"`
		}
		if json.Unmarshal(r.Body, &in) != nil || (in.Enabled == nil && in.Archived == nil) {
			return applications.Errorf(400, "enabled or archived is required")
		}
		if in.Archived != nil {
			t.Archived = *in.Archived
			if t.Archived {
				t.Enabled = false
				t.NextRunAt = 0
			}
		}
		if in.Enabled != nil {
			t.Enabled = *in.Enabled
		}
		if t.Archived && t.Enabled {
			return applications.Errorf(409, "restore this archived task before resuming it")
		}
		if t.Enabled {
			if t.MaxRuns > 0 && t.RunCount >= t.MaxRuns {
				return applications.Errorf(409, "task has reached maxRuns")
			}
			if t.ActiveRunID == "" {
				next, err := nextOccurrence(t, a.now())
				if err != nil {
					return applications.Errorf(400, "%s", err)
				}
				t.NextRunAt = next.UnixMilli()
			}
		}
		updated[id] = t
	case r.Method == "POST" && action == "run":
		if t.Archived {
			return applications.Errorf(409, "restore this archived task before running it")
		}
		if t.ActiveRunID != "" {
			return applications.Errorf(409, "task already has a pending run")
		}
		runID, err := newID()
		if err != nil {
			return applications.Errorf(500, "%s", err)
		}
		t.ActiveRunID = runID
		updated[id] = t
	case r.Method == "POST" && action == "complete":
		var in struct {
			RunID string `json:"runId"`
		}
		if r.Agent == nil || !r.Agent.Background || json.Unmarshal(r.Body, &in) != nil || in.RunID == "" || in.RunID != t.ActiveRunID || r.Agent.RequestID != in.RunID {
			return applications.Errorf(403, "active run required")
		}
		t.Enabled = false
		t.Archived = true
		t.NextRunAt = 0
		updated[id] = t
	default:
		return applications.Errorf(405, "method not allowed")
	}
	if err := a.commit(updated); err != nil {
		return applications.Errorf(500, "%s", err)
	}
	if r.Method == "DELETE" {
		return applications.JSON(200, map[string]bool{"ok": true})
	}
	return applications.JSON(200, t)
}
func newID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// completeCurrent is application policy, interpreted from Remote-stamped context.
func (a *API) completeCurrent(r applications.Request) applications.Response {
	var claim struct {
		TaskID string `json:"taskId"`
		RunID  string `json:"runId"`
	}
	if r.Agent == nil || !r.Agent.Background || json.Unmarshal(r.Agent.Context, &claim) != nil || claim.TaskID == "" || claim.RunID != r.Agent.RequestID {
		return applications.Errorf(403, "active scheduled turn required")
	}
	r.Path = "tasks/" + claim.TaskID + "/complete"
	r.Body, _ = json.Marshal(map[string]string{"runId": claim.RunID})
	return a.task(r)
}
