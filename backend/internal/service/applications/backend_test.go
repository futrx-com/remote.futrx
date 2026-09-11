package applications

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// recordingHost stands in for the process supervisor. Every test here is about
// policy — who may call, when, and with what stamped on the request — so what
// the host does with a call matters far less than whether it was reached.
type recordingHost struct {
	ensured  []string
	stopped  []string
	removed  []string
	requests []appplugin.Request
	deadline bool

	ensureErr error
	response  appplugin.Response
}

func (h *recordingHost) Ensure(_ context.Context, spec BackendSpec) (appplugin.Descriptor, error) {
	h.ensured = append(h.ensured, spec.Instance.ID)
	if h.ensureErr != nil {
		return appplugin.Descriptor{}, h.ensureErr
	}
	return appplugin.Descriptor{Name: spec.ImageID, APIVersion: appplugin.APIVersion}, nil
}

func (h *recordingHost) Call(
	ctx context.Context,
	spec BackendSpec,
	request appplugin.Request,
) (appplugin.Response, error) {
	h.ensured = append(h.ensured, spec.Instance.ID)
	h.requests = append(h.requests, request)
	_, h.deadline = ctx.Deadline()
	return h.response, nil
}

func (h *recordingHost) Stop(_ context.Context, instanceID string) error {
	h.stopped = append(h.stopped, instanceID)
	return nil
}

func (h *recordingHost) Remove(_ context.Context, instanceID string) error {
	h.removed = append(h.removed, instanceID)
	return nil
}

// backendRegistry answers with one image, so a test can shape exactly the
// image under test rather than the whole catalog.
type backendRegistry struct{ image Image }

func (r *backendRegistry) List() []Image { return []Image{r.image} }

func (r *backendRegistry) Get(id string) (Image, bool) {
	if id != r.image.ID {
		return Image{}, false
	}
	return r.image, true
}

func (r *backendRegistry) UIAsset(string, string) ([]byte, bool) { return nil, false }

func backendImage(mutate func(*Image)) Image {
	image := Image{
		ID:      "demo",
		Name:    "Demo",
		Type:    KindBackend,
		Scopes:  []Scope{ScopeGlobal},
		Backend: &ImageBackend{},
	}
	if mutate != nil {
		mutate(&image)
	}
	return image
}

// backendService builds a service with no container runtime at all, which is
// the point: a backend image installs and runs on a host with no LXD.
func backendService(image Image, instances []Instance, host BackendHost) (*Service, *fakeStore) {
	store := &fakeStore{global: instances}
	return New(
		&backendRegistry{image: image},
		store,
		nil, nil, nil,
		WithBackendHost(host),
	), store
}

func withInstance(image Image, instance Instance, host BackendHost) (*Service, *fakeStore) {
	return backendService(image, []Instance{instance}, host)
}

func runningInstance() Instance {
	return Instance{ID: "abc123", ImageID: "demo", Scope: ScopeGlobal, Status: StatusRunning}
}

func anyCaller() appplugin.Caller {
	return appplugin.Caller{Email: "user@example.com"}
}

// The caller a plugin sees is the one the transport resolved, never the one a
// request claimed. A plugin authorizes against it, so it has to be unforgeable.
func TestCallBackendStampsTheResolvedCaller(t *testing.T) {
	host := &recordingHost{response: appplugin.Response{Body: []byte("ok")}}
	service, _ := withInstance(backendImage(nil), runningInstance(), host)

	response, err := service.CallBackend(
		context.Background(),
		"abc123",
		appplugin.Request{
			Method: "GET",
			Path:   "health",
			Caller: appplugin.Caller{Email: "attacker@example.com", IsAdmin: true},
		},
		appplugin.Caller{Email: "user@example.com", IsAdmin: false},
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
	// A plugin that answers without a status means 200; forwarding a zero
	// would produce an invalid HTTP response.
	if response.Status != 200 {
		t.Errorf("status = %d, want 200 by default", response.Status)
	}
	if !host.deadline {
		t.Error("the host was called without a deadline")
	}
}

func TestCallBackendRefusals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		image    Image
		instance Instance
		caller   appplugin.Caller
		host     BackendHost
		want     error
	}{
		{
			name:     "no plugin host configured",
			image:    backendImage(nil),
			instance: runningInstance(),
			caller:   anyCaller(),
			host:     nil,
			want:     ErrUnavailable,
		},
		{
			name:     "the image ships no plugin",
			image:    backendImage(func(i *Image) { i.Type = KindUI; i.Backend = nil }),
			instance: runningInstance(),
			caller:   anyCaller(),
			host:     &recordingHost{},
			want:     ErrNoBackend,
		},
		{
			name:  "the app is stopped",
			image: backendImage(nil),
			instance: Instance{
				ID: "abc123", ImageID: "demo", Scope: ScopeGlobal, Status: StatusStopped,
			},
			caller: anyCaller(),
			host:   &recordingHost{},
			want:   ErrNotRunning,
		},
		{
			name: "an admin-only plugin and an ordinary caller",
			image: backendImage(func(i *Image) {
				i.Backend = &ImageBackend{Access: BackendAccessAdmin}
			}),
			instance: runningInstance(),
			caller:   anyCaller(),
			host:     &recordingHost{},
			want:     ErrBackendAccess,
		},
		{
			name:     "an unknown instance",
			image:    backendImage(nil),
			instance: Instance{ID: "other", ImageID: "demo", Status: StatusRunning},
			caller:   anyCaller(),
			host:     &recordingHost{},
			want:     ErrNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _ := withInstance(tc.image, tc.instance, tc.host)
			_, err := service.CallBackend(
				context.Background(), "abc123", appplugin.Request{Path: "health"}, tc.caller)
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// An admin-only plugin still answers an administrator: the access level is a
// gate, not a ban.
func TestCallBackendAllowsAnAdministratorThroughAnAdminGate(t *testing.T) {
	host := &recordingHost{}
	image := backendImage(func(i *Image) {
		i.Backend = &ImageBackend{Access: BackendAccessAdmin}
	})
	service, _ := withInstance(image, runningInstance(), host)

	if _, err := service.CallBackend(
		context.Background(),
		"abc123",
		appplugin.Request{Path: "health"},
		appplugin.Caller{Email: "admin@example.com", IsAdmin: true},
	); err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(host.requests) != 1 {
		t.Errorf("the admin was not let through")
	}
}

func TestDescribeBackendReportsTheImagePolicy(t *testing.T) {
	host := &recordingHost{}
	image := backendImage(func(i *Image) { i.Backend = &ImageBackend{TimeoutMS: 2500} })
	service, _ := withInstance(image, runningInstance(), host)

	described, err := service.DescribeBackend(context.Background(), "abc123", anyCaller())
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if described.InstanceID != "abc123" || described.ImageID != "demo" {
		t.Errorf("identity = %+v", described)
	}
	if described.TimeoutMS != 2500 || described.Access != BackendAccessRegistered {
		t.Errorf("policy = %+v, want the image's timeout and the default access", described)
	}
	if described.Descriptor.APIVersion != appplugin.APIVersion {
		t.Errorf("descriptor = %+v", described.Descriptor)
	}
}

// A plugin's process follows its instance's status: installing or starting
// runs it, stopping ends it, and only uninstalling discards its data.
func TestLifecycleMovesThePluginProcess(t *testing.T) {
	host := &recordingHost{}
	service, _ := backendService(backendImage(nil), nil, host)
	ctx := context.Background()

	installed, err := service.Install(ctx, InstallRequest{ImageID: "demo", Scope: ScopeGlobal})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	id := installed.ID
	if installed.Status != StatusRunning {
		t.Errorf("status = %q, want running", installed.Status)
	}
	if len(host.ensured) != 1 || host.ensured[0] != id {
		t.Errorf("install did not start the plugin: %v", host.ensured)
	}

	if _, err := service.Stop(ctx, id); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if len(host.stopped) != 1 || host.stopped[0] != id {
		t.Errorf("stopped = %v", host.stopped)
	}
	if len(host.removed) != 0 {
		t.Error("stop discarded the plugin's data; only uninstall may")
	}

	if _, err := service.Start(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(host.ensured) != 2 {
		t.Errorf("start did not run the plugin again: %v", host.ensured)
	}

	if err := service.Uninstall(ctx, id); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(host.removed) != 1 || host.removed[0] != id {
		t.Errorf("removed = %v", host.removed)
	}
}

// A plugin that will not start is an install failure the user can see, not a
// silently half-installed app.
func TestInstallRecordsAFailingPlugin(t *testing.T) {
	host := &recordingHost{ensureErr: errors.New("compile plugin: syntax error")}
	service, store := backendService(backendImage(nil), nil, host)

	if _, err := service.Install(
		context.Background(), InstallRequest{ImageID: "demo", Scope: ScopeGlobal},
	); err == nil {
		t.Fatal("a plugin that failed to start reported a successful install")
	}
	last := store.puts[len(store.puts)-1]
	if last.Status != StatusError || last.Error == "" {
		t.Errorf("stored instance = %+v, want an error status carrying the reason", last)
	}
}

// A failed install leaves a record so its error and whatever it left in a
// container stay readable — but that record is an attempt, not an
// installation. Installing again has to retry it rather than refuse, or the
// only way out of a failed install is to uninstall something the user was
// never told they had.
func TestInstallingOverAFailedAttemptRetriesIt(t *testing.T) {
	host := &recordingHost{ensureErr: errors.New("compile plugin: syntax error")}
	service, store := backendService(backendImage(nil), nil, host)
	ctx := context.Background()
	request := InstallRequest{ImageID: "demo", Scope: ScopeGlobal}

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
	// instance per image per scope is the invariant the rest of the system
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
		context.Background(), InstallRequest{ImageID: "demo", Scope: ScopeGlobal})
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("error = %v, want ErrAlreadyInstalled", err)
	}
	if len(host.removed) != 0 {
		t.Errorf("a running instance was torn down: %v", host.removed)
	}
}
