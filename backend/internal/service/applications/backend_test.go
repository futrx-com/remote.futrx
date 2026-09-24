package applications

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// recordingHost stands in for the process supervisor. Every test here is about
// policy — who may call, when, and with what stamped on the request — so what
// the host does with a call matters far less than whether it was reached.
type recordingHost struct {
	ensured     []string
	stopped     []string
	removed     []string
	invalidated []string
	requests    []applications.Request
	deadline    bool

	ensureErr error
	response  applications.Response
}

func (h *recordingHost) Ensure(_ context.Context, instance applications.Instance) (applications.Descriptor, error) {
	h.ensured = append(h.ensured, instance.ID)
	if h.ensureErr != nil {
		return applications.Descriptor{}, h.ensureErr
	}
	return applications.Descriptor{Name: instance.ApplicationID, APIVersion: applications.APIVersion}, nil
}

func (h *recordingHost) Call(
	ctx context.Context,
	instance applications.Instance,
	request applications.Request,
) (applications.Response, error) {
	h.ensured = append(h.ensured, instance.ID)
	h.requests = append(h.requests, request)
	_, h.deadline = ctx.Deadline()
	return h.response, nil
}

func (h *recordingHost) Notify(
	context.Context,
	applications.Instance,
	applications.Event,
) error {
	return nil
}

func (h *recordingHost) Stop(_ context.Context, instanceID string) error {
	h.stopped = append(h.stopped, instanceID)
	return nil
}

func (h *recordingHost) Remove(_ context.Context, instanceID string) error {
	h.removed = append(h.removed, instanceID)
	return nil
}

func (h *recordingHost) InvalidateApplication(applicationID string) {
	h.invalidated = append(h.invalidated, applicationID)
}

func backendImage(mutate func(*Application)) Application {
	application := Application{
		ID:      "demo",
		Name:    "Demo",
		Scopes:  []Scope{ScopeGlobal},
		Backend: &ApplicationBackend{},
	}
	if mutate != nil {
		mutate(&application)
	}
	return application
}

// backendService builds a service with no container runtime at all, which is
// the point: a backend application installs and runs on a host with no LXD.
func backendService(application Application, instances []Instance, host BackendHost) (*Service, *fakeStore) {
	store := &fakeStore{global: instances}
	return New(
		&singleApplicationRegistry{application: application},
		store,
		nil, nil, nil,
		WithBackendHost(host),
	), store
}

func withInstance(application Application, instance Instance, host BackendHost) (*Service, *fakeStore) {
	return backendService(application, []Instance{instance}, host)
}

func runningInstance() Instance {
	return Instance{ID: "abc123", ApplicationID: "demo", Scope: ScopeGlobal, Status: StatusRunning}
}

func anyCaller() applications.Caller {
	return applications.Caller{Email: "user@example.com"}
}

func TestBackendInstanceCarriesManifestMetadata(t *testing.T) {
	application := backendImage(func(application *Application) {
		application.Name = "Manifest Name"
		application.Version = "3.2.1"
		application.Service = &ApplicationService{Name: "manifest", Command: []string{"/usr/local/bin/manifest"}}
		application.Publishers = []applications.PublisherDeclaration{{
			Name: "greetings",
			Events: []applications.EventDeclaration{{
				Name: "sent", Version: 1,
			}},
		}}
		application.Subscriptions = []applications.Subscription{{
			Publisher: "remote.applications", Events: []string{"started"},
		}}
	})
	instance := backendInstanceDetails(application, runningInstance())
	if instance.ApplicationName != "Manifest Name" {
		t.Fatalf("application name = %q, want Manifest Name", instance.ApplicationName)
	}
	if instance.ApplicationVersion != "3.2.1" {
		t.Fatalf("application version = %q, want 3.2.1", instance.ApplicationVersion)
	}
	if instance.Service != "manifest" {
		t.Fatalf("service = %q, want manifest", instance.Service)
	}
	if len(instance.Publishers) != 1 || instance.Publishers[0].Name != "greetings" {
		t.Fatalf("publishers = %+v", instance.Publishers)
	}
	if len(instance.Subscriptions) != 1 || instance.Subscriptions[0].Publisher != "remote.applications" {
		t.Fatalf("subscriptions = %+v", instance.Subscriptions)
	}
}

func TestCurrentManifestControlsReportedEnvironment(t *testing.T) {
	application := backendImage(func(application *Application) {
		application.Env = []EnvVar{
			{Key: "VISIBLE"},
			{Key: "SECRET", Secret: true},
		}
		application.Connection = Connection{UserEnv: "VISIBLE", PasswordEnv: "SECRET"}
	})
	instance := runningInstance()
	instance.Env = map[string]string{
		"VISIBLE": "current-user",
		"SECRET":  "current-secret",
		"REMOVED": "stale-value",
	}
	service, _ := withInstance(application, instance, &recordingHost{})

	view, ok, err := service.Get(context.Background(), instance.ID)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if len(view.EnvPublic) != 1 || view.EnvPublic["VISIBLE"] != "current-user" {
		t.Fatalf("public env = %v, want only VISIBLE", view.EnvPublic)
	}

	credentials, err := service.Credentials(context.Background(), instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Username != "current-user" || credentials.Password != "current-secret" {
		t.Fatalf("connection mapping = %+v", credentials)
	}
	if len(credentials.Env) != 2 || credentials.Env["REMOVED"] != "" {
		t.Fatalf("credential env = %v, want only currently declared inputs", credentials.Env)
	}
}

// The caller a backend sees is the one the transport resolved, never the one a
// request claimed. A backend authorizes against it, so it has to be unforgeable.
func TestCallBackendStampsTheResolvedCaller(t *testing.T) {
	host := &recordingHost{response: applications.Response{Body: []byte("ok")}}
	service, _ := withInstance(backendImage(nil), runningInstance(), host)

	response, err := service.CallBackend(
		context.Background(),
		"abc123",
		applications.Request{
			Method: "GET",
			Path:   "health",
			Caller: applications.Caller{Email: "attacker@example.com", IsAdmin: true},
		},
		applications.Caller{Email: "user@example.com", IsAdmin: false},
	)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(host.requests) != 1 {
		t.Fatalf("host saw %d requests", len(host.requests))
	}
	if got := host.requests[0].Caller; got.Email != "user@example.com" || got.IsAdmin {
		t.Errorf("caller = %+v, want the resolved one", got)
	}
	// A backend that answers without a status means 200; forwarding a zero
	// would produce an invalid HTTP response.
	if response.Status != 200 {
		t.Errorf("status = %d, want 200 by default", response.Status)
	}
	if !host.deadline {
		t.Error("the host was called without a deadline")
	}
}

func TestCallBackendClearsBrowserSuppliedContext(t *testing.T) {
	host := &recordingHost{}
	service, _ := withInstance(backendImage(nil), runningInstance(), host)

	_, err := service.CallBackend(
		context.Background(),
		"abc123",
		applications.Request{Context: applications.RequestContext{Chat: &applications.ChatContext{
			ID: "forged", ProjectID: "secret", WorkspaceRoot: "/etc",
		}}},
		anyCaller(),
	)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := host.requests[0].Context.Chat; got != nil {
		t.Fatalf("ordinary call retained forged chat context: %+v", got)
	}
}

func TestCallBackendForChatStampsTrustedContext(t *testing.T) {
	host := &recordingHost{}
	service, _ := withInstance(backendImage(nil), runningInstance(), host)
	trusted := applications.ChatContext{
		ID: "chat-1", ProjectID: "project-1", WorkspaceRoot: "/srv/projects/one/workspace",
	}

	_, err := service.CallBackendForChat(
		context.Background(),
		"abc123",
		applications.Request{Context: applications.RequestContext{Chat: &applications.ChatContext{
			ID: "forged", WorkspaceRoot: "/etc",
		}}},
		anyCaller(),
		trusted,
	)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := host.requests[0].Context.Chat
	if got == nil || *got != trusted {
		t.Fatalf("chat context = %+v, want %+v", got, trusted)
	}
}

func TestBackendChatScope(t *testing.T) {
	global := runningInstance()
	project := runningInstance()
	project.Scope = ScopeProject
	project.ProjectID = "project-1"
	chat := applications.ChatContext{
		ID: "chat-1", ProjectID: "project-1", WorkspaceRoot: "/srv/projects/one/workspace",
	}

	for _, tc := range []struct {
		name     string
		instance Instance
		chat     applications.ChatContext
		wantErr  error
	}{
		{name: "global install", instance: global, chat: chat},
		{name: "matching project install", instance: project, chat: chat},
		{
			name: "another project install", instance: project,
			chat:    applications.ChatContext{ID: "chat-2", ProjectID: "project-2", WorkspaceRoot: "/workspace"},
			wantErr: ErrBackendContextAccess,
		},
		{
			name: "project install in a loose chat", instance: project,
			chat:    applications.ChatContext{ID: "chat-3", WorkspaceRoot: "/workspace"},
			wantErr: ErrBackendContextAccess,
		},
		{
			name: "missing workspace", instance: global,
			chat:    applications.ChatContext{ID: "chat-4"},
			wantErr: ErrBackendContext,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := &recordingHost{}
			service, _ := withInstance(backendImage(nil), tc.instance, host)
			_, err := service.CallBackendForChat(
				context.Background(), "abc123", applications.Request{}, anyCaller(), tc.chat)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && len(host.requests) != 0 {
				t.Fatal("a refused chat context reached the backend")
			}
		})
	}
}

func TestCallBackendRefusals(t *testing.T) {
	for _, tc := range []struct {
		name        string
		application Application
		instance    Instance
		caller      applications.Caller
		host        BackendHost
		want        error
	}{
		{
			name:        "no backend host configured",
			application: backendImage(nil),
			instance:    runningInstance(),
			caller:      anyCaller(),
			host:        nil,
			want:        ErrUnavailable,
		},
		{
			name:        "the application ships no backend",
			application: backendImage(func(i *Application) { i.Backend = nil }),
			instance:    runningInstance(),
			caller:      anyCaller(),
			host:        &recordingHost{},
			want:        ErrNoBackend,
		},
		{
			name:        "the app is stopped",
			application: backendImage(nil),
			instance: Instance{
				ID: "abc123", ApplicationID: "demo", Scope: ScopeGlobal, Status: StatusStopped,
			},
			caller: anyCaller(),
			host:   &recordingHost{},
			want:   ErrNotRunning,
		},
		{
			name: "an admin-only backend and an ordinary caller",
			application: backendImage(func(i *Application) {
				i.Backend = &ApplicationBackend{Access: BackendAccessAdmin}
			}),
			instance: runningInstance(),
			caller:   anyCaller(),
			host:     &recordingHost{},
			want:     ErrBackendAccess,
		},
		{
			name:        "an unknown instance",
			application: backendImage(nil),
			instance:    Instance{ID: "other", ApplicationID: "demo", Status: StatusRunning},
			caller:      anyCaller(),
			host:        &recordingHost{},
			want:        ErrNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _ := withInstance(tc.application, tc.instance, tc.host)
			_, err := service.CallBackend(
				context.Background(), "abc123", applications.Request{Path: "health"}, tc.caller)
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// An admin-only backend still answers an administrator: the access level is a
// gate, not a ban.
func TestCallBackendAllowsAnAdministratorThroughAnAdminGate(t *testing.T) {
	host := &recordingHost{}
	application := backendImage(func(i *Application) {
		i.Backend = &ApplicationBackend{Access: BackendAccessAdmin}
	})
	service, _ := withInstance(application, runningInstance(), host)

	if _, err := service.CallBackend(
		context.Background(),
		"abc123",
		applications.Request{Path: "health"},
		applications.Caller{Email: "admin@example.com", IsAdmin: true},
	); err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(host.requests) != 1 {
		t.Errorf("the admin was not let through")
	}
}

func TestDescribeBackendReportsTheImagePolicy(t *testing.T) {
	host := &recordingHost{}
	application := backendImage(func(i *Application) { i.Backend = &ApplicationBackend{TimeoutMS: 2500} })
	service, _ := withInstance(application, runningInstance(), host)

	described, err := service.DescribeBackend(context.Background(), "abc123", anyCaller())
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if described.InstanceID != "abc123" || described.ApplicationID != "demo" {
		t.Errorf("identity = %+v", described)
	}
	if described.TimeoutMS != 2500 || described.Access != BackendAccessRegistered {
		t.Errorf("policy = %+v, want the application's timeout and the default access", described)
	}
	if described.Descriptor.APIVersion != applications.APIVersion {
		t.Errorf("descriptor = %+v", described.Descriptor)
	}
}

func TestDescribeBackendForChatUsesTheSameScopeGate(t *testing.T) {
	instance := runningInstance()
	instance.Scope = ScopeProject
	instance.ProjectID = "project-1"
	service, _ := withInstance(backendImage(nil), instance, &recordingHost{})

	_, err := service.DescribeBackendForChat(
		context.Background(),
		instance.ID,
		anyCaller(),
		applications.ChatContext{ID: "chat-1", ProjectID: "project-2", WorkspaceRoot: "/workspace"},
	)
	if !errors.Is(err, ErrBackendContextAccess) {
		t.Fatalf("error = %v, want ErrBackendContextAccess", err)
	}
}

// A backend's process follows its instance's status: installing or starting
// runs it, stopping ends it, and only uninstalling discards its data.
func TestLifecycleMovesTheBackendProcess(t *testing.T) {
	host := &recordingHost{}
	service, _ := backendService(backendImage(nil), nil, host)
	ctx := context.Background()

	installed, err := service.Install(ctx, InstallRequest{ApplicationID: "demo", Scope: ScopeGlobal})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	id := installed.ID
	if installed.Status != StatusRunning {
		t.Errorf("status = %q, want running", installed.Status)
	}
	if len(host.ensured) != 1 || host.ensured[0] != id {
		t.Errorf("install did not start the backend: %v", host.ensured)
	}

	if _, err := service.Stop(ctx, id); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(host.stopped) != 1 || host.stopped[0] != id {
		t.Errorf("stopped = %v", host.stopped)
	}
	if len(host.removed) != 0 {
		t.Error("stop discarded the backend's data; only uninstall may")
	}

	if _, err := service.Start(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(host.ensured) != 2 {
		t.Errorf("start did not run the backend again: %v", host.ensured)
	}

	if err := service.Uninstall(ctx, id); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(host.removed) != 1 || host.removed[0] != id {
		t.Errorf("removed = %v", host.removed)
	}
}

func TestFailedAttemptCannotBeStartedOrStopped(t *testing.T) {
	host := &recordingHost{}
	failed := runningInstance()
	failed.Status = StatusError
	failed.Error = "install failed"
	service, store := withInstance(backendImage(nil), failed, host)

	for _, action := range []struct {
		name string
		run  func() (View, error)
	}{
		{name: "start", run: func() (View, error) { return service.Start(context.Background(), failed.ID) }},
		{name: "stop", run: func() (View, error) { return service.Stop(context.Background(), failed.ID) }},
	} {
		t.Run(action.name, func(t *testing.T) {
			if _, err := action.run(); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("error = %v, want ErrInvalidState", err)
			}
		})
	}
	if len(host.ensured) != 0 || len(host.stopped) != 0 {
		t.Fatalf("failed attempt reached backend host: ensured=%v stopped=%v", host.ensured, host.stopped)
	}
	if len(store.puts) != 0 {
		t.Fatalf("failed attempt was rewritten: %+v", store.puts)
	}
}

// A backend that will not start is an install failure the user can see, not a
// silently half-installed app.
func TestInstallRecordsAFailingBackend(t *testing.T) {
	host := &recordingHost{ensureErr: errors.New("compile backend: syntax error")}
	service, store := backendService(backendImage(nil), nil, host)

	if _, err := service.Install(
		context.Background(), InstallRequest{ApplicationID: "demo", Scope: ScopeGlobal},
	); err == nil {
		t.Fatal("a backend that failed to start reported a successful install")
	}
	last := store.puts[len(store.puts)-1]
	if last.Status != StatusError || last.Error == "" {
		t.Errorf("stored instance = %+v, want an error status carrying the reason", last)
	}
}

func TestStoredAndReportedInstanceErrorsAreBounded(t *testing.T) {
	host := &recordingHost{ensureErr: errors.New("prefix " + strings.Repeat("x", 9000) + " tail")}
	service, store := backendService(backendImage(nil), nil, host)

	if _, err := service.Install(
		context.Background(), InstallRequest{ApplicationID: "demo", Scope: ScopeGlobal},
	); err == nil {
		t.Fatal("install succeeded despite the backend failure")
	}
	stored := store.puts[len(store.puts)-1]
	if got := len([]rune(stored.Error)); got > maxInstanceErrorRunes {
		t.Fatalf("stored error has %d runes, want at most %d", got, maxInstanceErrorRunes)
	}
	if !strings.Contains(stored.Error, "prefix ") || !strings.HasSuffix(stored.Error, " tail") {
		t.Fatalf("bounded error lost its context: %q", stored.Error)
	}

	// The read-side bound also protects records written by older releases.
	legacy := failedInstanceWithError(strings.Repeat("legacy", 2000))
	view := service.view(legacy)
	if got := len([]rune(view.Error)); got > maxInstanceErrorRunes {
		t.Fatalf("reported error has %d runes, want at most %d", got, maxInstanceErrorRunes)
	}
}

func failedInstanceWithError(message string) Instance {
	instance := runningInstance()
	instance.Status = StatusError
	instance.Error = message
	return instance
}

// A failed install leaves a record so its error and whatever it left in a
// container stay readable — but that record is an attempt, not an
// installation. Installing again has to retry it rather than refuse, or the
// only way out of a failed install is to uninstall something the user was
// never told they had.
func TestInstallingOverAFailedAttemptRetriesIt(t *testing.T) {
	host := &recordingHost{ensureErr: errors.New("compile backend: syntax error")}
	service, store := backendService(backendImage(nil), nil, host)
	ctx := context.Background()
	request := InstallRequest{ApplicationID: "demo", Scope: ScopeGlobal}

	if _, err := service.Install(ctx, request); err == nil {
		t.Fatal("the first install was expected to fail")
	}
	failed := store.puts[len(store.puts)-1]
	if failed.Status != StatusError {
		t.Fatalf("first attempt = %q, want error", failed.Status)
	}

	host.ensureErr = nil
	retried, err := service.Install(ctx, request)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retried.Status != StatusRunning {
		t.Errorf("retry status = %q, want running", retried.Status)
	}
	// The failed attempt is torn down, not left beside the working one: one
	// instance per application per scope is the invariant the rest of the system
	// reads.
	if len(host.removed) != 1 || host.removed[0] != failed.ID {
		t.Errorf("removed = %v, want the failed attempt %s", host.removed, failed.ID)
	}
	if !slices.Contains(store.deleted, failed.ID) {
		t.Errorf("deleted = %v, want the failed attempt %s", store.deleted, failed.ID)
	}
	instances, _ := store.ListGlobal(ctx)
	if len(instances) != 1 || instances[0].ID != retried.ID {
		t.Errorf("store holds %+v, want only the retried instance", instances)
	}
}

// Installing over a *working* instance is still refused: the retry path must
// not become a way to silently destroy a running app.
func TestInstallingOverAWorkingInstanceIsStillRefused(t *testing.T) {
	host := &recordingHost{}
	service, _ := withInstance(backendImage(nil), runningInstance(), host)

	_, err := service.Install(
		context.Background(), InstallRequest{ApplicationID: "demo", Scope: ScopeGlobal})
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("error = %v, want ErrAlreadyInstalled", err)
	}
	if len(host.removed) != 0 {
		t.Errorf("a running instance was torn down: %v", host.removed)
	}
}
