package applications

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Catalog supplies the Go source of an application's backend. It is the registry,
// narrowed to the one thing the host needs from it.
type Catalog interface {
	BackendSource(applicationID string) (fs.FS, bool)
}

// EventSink accepts host-validated application events. The process-wide
// lifecycle bus implements it; keeping this boundary small prevents the
// process host from depending on subscription or routing policy.
type EventSink interface {
	Publish(context.Context, applications.Event)
}

// eventRuntimeBinder is implemented by the RPC transport. It is deliberately
// not part of applications.Backend: core binds its own runtime capability,
// while application API implementations remain unaware of transport setup.
type eventRuntimeBinder interface {
	BindEvents(applications.EventEmitter) error
}

// Host runs one backend process per installed instance.
//
// The unit is the instance, not the application: an application installed globally and in
// two projects is three processes, because each serves a different install
// with its own environment, durable data directory, and crash behaviour. They
// share one compiled binary and one non-durable application runtime directory
// for explicit cross-instance coordination.
type Host struct {
	root    string
	catalog Catalog
	builder *Builder
	logger  hclog.Logger
	events  EventSink

	launches           keyedLocks
	applicationChanges keyedLocks

	mu      sync.Mutex
	running map[string]*backendProcess
	// generations prevent work built from a pre-replacement catalog snapshot
	// from being launched after the application's source or manifest changes.
	generations map[string]uint64
	// terminateProcess is a seam for deterministic lifecycle concurrency tests.
	// Production always binds it to backendProcess.stop.
	terminateProcess func(*backendProcess)
}

// Options supplies process-host settings owned by the application edge.
type Options struct {
	GoTool string
	Events EventSink
}

// New builds a backend host that keeps compiled binaries, generated modules,
// per-instance data, and per-application shared runtime directories under
// root.
func New(root string, catalog Catalog, options Options) *Host {
	host := &Host{
		root:    root,
		catalog: catalog,
		builder: NewBuilder(root, options.GoTool),
		events:  options.Events,
		logger: hclog.New(&hclog.LoggerOptions{
			Name:   "app-backend",
			Level:  hclog.Info,
			Output: os.Stderr,
		}),
		launches:           newKeyedLocks(),
		applicationChanges: newKeyedLocks(),
		running:            map[string]*backendProcess{},
		generations:        map[string]uint64{},
	}
	host.terminateProcess = func(process *backendProcess) { process.stop() }
	return host
}

var _ svc.BackendHost = (*Host)(nil)

// handshakeTimeout bounds a backend's startup. A backend that has not completed
// go-plugin's handshake by then is not going to.
const handshakeTimeout = 30 * time.Second

// Ensure compiles the application's backend if needed, starts a process for the
// instance, and returns its manifest-enriched descriptor.
func (h *Host) Ensure(ctx context.Context, instance applications.Instance) (applications.Descriptor, error) {
	current, err := h.ensure(ctx, instance)
	if err != nil {
		return applications.Descriptor{}, err
	}
	return current.descriptor, nil
}

// Call forwards one request, starting the backend first if it is not running.
func (h *Host) Call(
	ctx context.Context,
	instance applications.Instance,
	request applications.Request,
) (applications.Response, error) {
	current, err := h.ensure(ctx, instance)
	if err != nil {
		return applications.Response{}, err
	}
	return current.call(ctx, request)
}

// Notify delivers one subscribed event to a running instance. Like Call, it
// lazily restores the backend process after a server restart; the service
// layer decides whether the instance is running and subscribed before asking.
func (h *Host) Notify(
	ctx context.Context,
	instance applications.Instance,
	event applications.Event,
) error {
	current, err := h.ensure(ctx, instance)
	if err != nil {
		return err
	}
	if !current.descriptor.SubscribesEvents {
		return fmt.Errorf("backend %s does not implement applications.EventSubscriber", instance.ApplicationID)
	}
	return current.notify(ctx, event)
}

// Stop terminates an instance's backend, keeping its data directory so a later
// start resumes with it.
func (h *Host) Stop(_ context.Context, instanceID string) error {
	unlock := h.lockInstanceLifecycle(instanceID)
	defer unlock()
	h.kill(instanceID)
	return nil
}

// Remove terminates an instance's backend and discards its data. Uninstalling
// is the only thing that deletes backend state, which is what makes stop and
// start safe to use freely.
//
// Killing and deleting happen under one hold of the launch lock. Taking it
// twice would leave a window between them in which a request already inside
// ensure could launch a replacement process against the instance being removed:
// the uninstall would then delete the data directory of a live backend and
// return, leaving that backend running with nothing left to address it by.
func (h *Host) Remove(_ context.Context, instanceID string) error {
	unlock := h.lockInstanceLifecycle(instanceID)
	defer unlock()
	h.kill(instanceID)
	if err := os.RemoveAll(h.dataDir(instanceID)); err != nil {
		return fmt.Errorf("remove backend data for %s: %w", instanceID, err)
	}
	return nil
}

// InvalidateApplication terminates every process created from applicationID
// and advances its generation. The application boundary waits for a process
// already completing its handshake, then terminates it before returning; a
// launch built from an older snapshot refuses to start and retries against the
// current catalog.
//
// Package replacement calls this immediately after the registry atomically
// swaps its view, before the application-updated event can reach subscribers.
func (h *Host) InvalidateApplication(applicationID string) {
	unlockApplication := h.applicationChanges.lock(applicationID)
	defer unlockApplication()

	h.mu.Lock()
	h.generations[applicationID]++
	var invalidated []*backendProcess
	for _, current := range h.running {
		if current.applicationID != applicationID {
			continue
		}
		invalidated = append(invalidated, current)
	}
	h.mu.Unlock()

	for _, current := range invalidated {
		h.terminateProcess(current)
	}
	h.mu.Lock()
	for instanceID, current := range h.running {
		if current.applicationID == applicationID {
			delete(h.running, instanceID)
		}
	}
	h.mu.Unlock()
	if err := os.RemoveAll(h.sharedRuntimeDir(applicationID)); err != nil {
		h.logger.Warn("remove invalidated application runtime directory", "application", applicationID, "error", err)
	}
}

// Shutdown stops every running backend. The server calls it on the way out so
// backend processes do not outlive it.
func (h *Host) Shutdown() {
	h.mu.Lock()
	ids := make([]string, 0, len(h.running))
	for id := range h.running {
		ids = append(ids, id)
	}
	h.mu.Unlock()

	for _, id := range ids {
		_ = h.Stop(context.Background(), id)
	}
	if err := os.RemoveAll(h.sharedRuntimeRoot()); err != nil {
		h.logger.Warn("remove application runtime directories", "error", err)
	}
}

// ---- launching --------------------------------------------------------------

// ensure returns a live process for the instance, launching one if there is
// none or if the previous one exited.
func (h *Host) ensure(ctx context.Context, instance applications.Instance) (*backendProcess, error) {
	instanceID := instance.ID
	applicationID := instance.ApplicationID
	configuration, err := backendConfiguration(instance)
	if err != nil {
		return nil, fmt.Errorf("encode backend %s configuration: %w", applicationID, err)
	}

	for {
		generation, current := h.snapshot(applicationID, instanceID)
		// A live process needs nothing else when it was initialized from this
		// exact instance configuration and the application's current package
		// generation. This remains the path every ordinary request takes.
		if processMatches(current, applicationID, generation, configuration) && current.running() {
			return current, nil
		}

		source, ok := h.catalog.BackendSource(applicationID)
		if !ok {
			return nil, fmt.Errorf("%w: %s ships no backend source", svc.ErrNoBackend, applicationID)
		}
		// Building stays outside the launch lock so two instances of the same
		// application share one build instead of queueing behind each other's launches.
		binary, err := h.builder.Build(ctx, applicationID, source)
		if err != nil {
			return nil, err
		}

		// Package invalidation holds the application boundary while it advances
		// the generation and terminates old children. Taking the same boundary
		// here prevents a replacement process from starting before every old
		// process has actually exited.
		unlockApplication := h.applicationChanges.lock(applicationID)
		unlock := h.launches.lock(instanceID)

		// Re-check under the lock: another caller may have launched it while this
		// one was building, or package replacement may have advanced the
		// generation. Retrying after replacement re-reads the current source.
		currentGeneration, current := h.snapshot(applicationID, instanceID)
		if currentGeneration != generation {
			unlock()
			unlockApplication()
			continue
		}
		if processMatches(current, applicationID, generation, configuration) &&
			current.binary == binary && current.running() {
			unlock()
			unlockApplication()
			return current, nil
		}
		if current != nil {
			// An exited or differently configured client is replaced. This is
			// what makes a crashed backend recover and changed instance metadata
			// reach Backend.Init on the next call.
			h.kill(instanceID)
		}
		started, err := h.launch(ctx, instance, binary, configuration, generation)
		unlock()
		unlockApplication()
		if errors.Is(err, errBackendInvalidated) {
			continue
		}
		return started, err
	}
}

var errBackendInvalidated = errors.New("application backend invalidated during launch")

func (h *Host) launch(
	ctx context.Context,
	instance applications.Instance,
	binary string,
	configuration [32]byte,
	generation uint64,
) (*backendProcess, error) {
	instanceID := instance.ID
	applicationID := instance.ApplicationID
	dataDir := h.dataDir(instanceID)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create backend data directory: %w", err)
	}
	sharedRuntimeDir := h.sharedRuntimeDir(applicationID)
	if err := os.MkdirAll(sharedRuntimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create backend shared runtime directory: %w", err)
	}

	command := exec.Command(binary)
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: backendHandshake,
		Plugins:         goplugin.PluginSet{backendName: &backendAdapter{}},
		Cmd:             command,
		Logger:          h.logger.Named(applicationID),
		StartTimeout:    handshakeTimeout,
	})
	processClient := &pluginProcessClient{client: client, command: command}

	type connectResult struct {
		process *backendProcess
		err     error
	}
	connected := make(chan connectResult, 1)
	go func() {
		process, err := h.connect(client, processClient, instance, dataDir, sharedRuntimeDir)
		connected <- connectResult{process: process, err: err}
	}()

	var started *backendProcess
	select {
	case result := <-connected:
		if result.err != nil {
			client.Kill()
			return nil, result.err
		}
		started = result.process
	case <-ctx.Done():
		// go-plugin's net/rpc calls cannot be canceled individually. Killing the
		// child closes every transport connection and releases a Describe, event
		// runtime binding, or Init call that ignored its deadline.
		client.Kill()
		return nil, fmt.Errorf("initialize backend %s timed out: %w", applicationID, ctx.Err())
	}
	started.applicationID = applicationID
	started.binary = binary
	started.configuration = configuration
	started.generation = generation

	h.mu.Lock()
	if h.generations[applicationID] != generation {
		h.mu.Unlock()
		started.stop()
		return nil, errBackendInvalidated
	}
	h.running[instanceID] = started
	h.mu.Unlock()
	return started, nil
}

// connect completes the handshake, checks the contract version, and hands the
// instance over. Every failure here kills the process rather than leaving a
// half-initialized backend reachable.
func (h *Host) connect(
	client *goplugin.Client,
	processClient processClient,
	instance applications.Instance,
	dataDir string,
	sharedRuntimeDir string,
) (*backendProcess, error) {
	applicationID := instance.ApplicationID
	protocol, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("start backend %s: %w", applicationID, err)
	}
	raw, err := protocol.Dispense(backendName)
	if err != nil {
		return nil, fmt.Errorf("connect to backend %s: %w", applicationID, err)
	}
	backend, ok := raw.(applications.Backend)
	if !ok {
		return nil, fmt.Errorf("backend %s served an unexpected type %T", applicationID, raw)
	}
	descriptor, err := backend.Describe()
	if err != nil {
		return nil, fmt.Errorf("describe backend %s: %w", applicationID, err)
	}
	if descriptor.APIVersion != applications.APIVersion {
		return nil, fmt.Errorf(
			"backend %s reports contract version %d, this server speaks %d",
			applicationID, descriptor.APIVersion, applications.APIVersion)
	}
	// application.json is the metadata source of truth for the package. A
	// backend is one capability of that package, so making every backend repeat
	// the same name and version in Describe only creates values that can drift.
	descriptor.Name = instance.ApplicationName
	descriptor.Version = instance.ApplicationVersion
	if len(instance.Publishers) > 0 && h.events == nil {
		return nil, fmt.Errorf("bind events for backend %s: event bus unavailable", applicationID)
	}
	if len(instance.Subscriptions) > 0 && !descriptor.SubscribesEvents {
		return nil, fmt.Errorf(
			"backend %s declares subscriptions but does not implement applications.EventSubscriber",
			applicationID,
		)
	}
	if len(instance.Publishers) > 0 {
		runtime, ok := backend.(eventRuntimeBinder)
		if !ok {
			return nil, fmt.Errorf("backend %s event runtime transport is unavailable", applicationID)
		}
		if err := runtime.BindEvents(newInstancePublisher(instance, h.events)); err != nil {
			return nil, fmt.Errorf("bind events for backend %s: %w", applicationID, err)
		}
	}
	if err := backend.Init(instanceWithHostDirectories(instance, dataDir, sharedRuntimeDir)); err != nil {
		return nil, fmt.Errorf("initialize backend %s: %w", applicationID, err)
	}
	descriptor.PublishesEvents = len(instance.Publishers) > 0
	return &backendProcess{client: processClient, backend: backend, descriptor: descriptor}, nil
}

// pluginProcessClient keeps the exact command used to launch the child so a
// wedged RPC transport can be terminated without looking the process up again
// by PID. Client.Kill then performs go-plugin's ordinary asynchronous cleanup.
type pluginProcessClient struct {
	client  *goplugin.Client
	command *exec.Cmd
}

func (c *pluginProcessClient) Exited() bool { return c.client.Exited() }

func (c *pluginProcessClient) Kill() { c.client.Kill() }

func (c *pluginProcessClient) ForceKill() {
	if c.command != nil && c.command.Process != nil {
		_ = c.command.Process.Kill()
	}
}

// instanceWithHostDirectories adds host-owned storage to the instance before
// it crosses the backend boundary. DataDir belongs to one installed copy;
// SharedRuntimeDir is common to every process of the same application and is
// deliberately non-durable. The instance also carries the resolved
// environment, secrets included: an application's backend needs the password
// its own install script generated.
func instanceWithHostDirectories(
	instance applications.Instance,
	dataDir string,
	sharedRuntimeDir string,
) applications.Instance {
	instance.DataDir = dataDir
	instance.SharedRuntimeDir = sharedRuntimeDir
	return instance
}

func (h *Host) lookup(instanceID string) *backendProcess {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running[instanceID]
}

func (h *Host) snapshot(applicationID, instanceID string) (uint64, *backendProcess) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.generations[applicationID], h.running[instanceID]
}

// lockInstanceLifecycle serializes Stop and Remove with package invalidation
// as well as with launches. The application id is process-owned rather than
// supplied by the caller, so discovering it needs a short first hold of the
// instance launch lock. The lock is then reacquired in the canonical
// application-before-instance order used by ensure, and the process is
// rechecked in case invalidation or replacement won the gap.
//
// Keeping the application boundary through process termination matters for
// SharedRuntimeDir: invalidation must not remove and recreate that directory
// while an old process still holds a lock on an unlinked inode.
func (h *Host) lockInstanceLifecycle(instanceID string) func() {
	for {
		unlockInstance := h.launches.lock(instanceID)
		current := h.lookup(instanceID)
		if current == nil {
			return unlockInstance
		}
		applicationID := current.applicationID
		unlockInstance()

		unlockApplication := h.applicationChanges.lock(applicationID)
		unlockInstance = h.launches.lock(instanceID)
		current = h.lookup(instanceID)
		if current == nil || current.applicationID == applicationID {
			return func() {
				unlockInstance()
				unlockApplication()
			}
		}

		unlockInstance()
		unlockApplication()
	}
}

func processMatches(
	current *backendProcess,
	applicationID string,
	generation uint64,
	configuration [32]byte,
) bool {
	return current != nil &&
		current.applicationID == applicationID &&
		current.generation == generation &&
		current.configuration == configuration
}

func backendConfiguration(instance applications.Instance) ([32]byte, error) {
	encoded, err := json.Marshal(instance)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

// kill terminates an instance's process. The caller holds its launch lock.
func (h *Host) kill(instanceID string) {
	h.mu.Lock()
	current := h.running[instanceID]
	delete(h.running, instanceID)
	h.mu.Unlock()

	if current != nil {
		h.terminateProcess(current)
	}
}

func (h *Host) dataDir(instanceID string) string {
	return filepath.Join(h.root, "data", instanceID)
}

func (h *Host) sharedRuntimeRoot() string {
	return filepath.Join(h.root, "runtime")
}

func (h *Host) sharedRuntimeDir(applicationID string) string {
	return filepath.Join(h.sharedRuntimeRoot(), applicationID)
}
