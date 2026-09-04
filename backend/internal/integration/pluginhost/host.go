package pluginhost

import (
	"context"
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
	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// Catalog supplies the Go source of an image's plugin. It is the registry,
// narrowed to the one thing the host needs from it.
type Catalog interface {
	PluginSource(imageID string) (fs.FS, bool)
}

// Host runs one plugin process per installed instance.
//
// The unit is the instance, not the image: an image installed globally and in
// two projects is three processes, because each serves a different install
// with its own environment, its own data directory, and its own crash
// behaviour. They share one compiled binary.
type Host struct {
	root    string
	catalog Catalog
	builder *Builder
	logger  hclog.Logger

	launches keyedLocks

	mu      sync.Mutex
	running map[string]*pluginProcess
}

// Options supplies process-host settings owned by the application edge.
type Options struct {
	GoTool string
}

// New builds a plugin host that keeps compiled binaries, generated modules,
// and per-instance data under root.
func New(root string, catalog Catalog, options Options) *Host {
	return &Host{
		root:    root,
		catalog: catalog,
		builder: NewBuilder(root, options.GoTool),
		logger: hclog.New(&hclog.LoggerOptions{
			Name:   "app-plugin",
			Level:  hclog.Info,
			Output: os.Stderr,
		}),
		launches: newKeyedLocks(),
		running:  map[string]*pluginProcess{},
	}
}

var _ svc.BackendHost = (*Host)(nil)

// handshakeTimeout bounds a plugin's startup. A plugin that has not completed
// go-plugin's handshake by then is not going to.
const handshakeTimeout = 30 * time.Second

// Ensure compiles the image's plugin if needed, starts a process for the
// instance, and returns what the plugin reported about itself.
func (h *Host) Ensure(ctx context.Context, spec svc.BackendSpec) (appplugin.Descriptor, error) {
	current, err := h.ensure(ctx, spec)
	if err != nil {
		return appplugin.Descriptor{}, err
	}
	return current.descriptor, nil
}

// Call forwards one request, starting the plugin first if it is not running.
func (h *Host) Call(
	ctx context.Context,
	spec svc.BackendSpec,
	request appplugin.Request,
) (appplugin.Response, error) {
	current, err := h.ensure(ctx, spec)
	if err != nil {
		return appplugin.Response{}, err
	}
	return current.call(ctx, request)
}

// Stop terminates an instance's plugin, keeping its data directory so a later
// start resumes with it.
func (h *Host) Stop(_ context.Context, instanceID string) error {
	unlock := h.launches.lock(instanceID)
	defer unlock()
	h.kill(instanceID)
	return nil
}

// Remove terminates an instance's plugin and discards its data. Uninstalling
// is the only thing that deletes plugin state, which is what makes stop and
// start safe to use freely.
func (h *Host) Remove(ctx context.Context, instanceID string) error {
	if err := h.Stop(ctx, instanceID); err != nil {
		return err
	}
	if err := os.RemoveAll(h.dataDir(instanceID)); err != nil {
		return fmt.Errorf("remove plugin data for %s: %w", instanceID, err)
	}
	return nil
}

// Shutdown stops every running plugin. The server calls it on the way out so
// plugin processes do not outlive it.
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
}

// ---- launching --------------------------------------------------------------

// ensure returns a live process for the instance, launching one if there is
// none or if the previous one exited.
func (h *Host) ensure(ctx context.Context, spec svc.BackendSpec) (*pluginProcess, error) {
	instanceID := spec.Instance.ID

	// A live process needs nothing else, and this is the path every request
	// takes. The catalog is embedded, so an image's source — and therefore the
	// binary compiled from it — cannot change while the server runs: there is
	// nothing a per-call rebuild check could discover. Doing one anyway would
	// re-read and re-hash the whole plugin on every request and funnel
	// concurrent calls through the builder's per-image lock.
	if current := h.lookup(instanceID); current != nil && current.running() {
		return current, nil
	}

	source, ok := h.catalog.PluginSource(spec.ImageID)
	if !ok {
		return nil, fmt.Errorf("%w: %s ships no plugin source", svc.ErrNoBackend, spec.ImageID)
	}
	// Building stays outside the launch lock so two instances of the same
	// image share one build instead of queueing behind each other's launches.
	binary, err := h.builder.Build(ctx, spec.ImageID, source)
	if err != nil {
		return nil, err
	}

	unlock := h.launches.lock(instanceID)
	defer unlock()

	// Re-check under the lock: another caller may have launched it while this
	// one was building.
	if current := h.lookup(instanceID); current != nil {
		if current.binary == binary && current.running() {
			return current, nil
		}
		// An exited client means the plugin crashed, which is answered by
		// replacing the process — that is why a crashed plugin recovers on the
		// next call.
		h.kill(instanceID)
	}
	return h.launch(ctx, spec, binary)
}

func (h *Host) launch(ctx context.Context, spec svc.BackendSpec, binary string) (*pluginProcess, error) {
	instanceID := spec.Instance.ID
	dataDir := h.dataDir(instanceID)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create plugin data directory: %w", err)
	}

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: pluginHandshake,
		Plugins:         goplugin.PluginSet{backendPluginName: &backendPlugin{}},
		Cmd:             exec.Command(binary),
		Logger:          h.logger.Named(spec.ImageID),
		StartTimeout:    handshakeTimeout,
	})

	started, err := h.connect(client, spec, dataDir)
	if err != nil {
		client.Kill()
		return nil, err
	}
	started.binary = binary

	h.mu.Lock()
	h.running[instanceID] = started
	h.mu.Unlock()
	return started, nil
}

// connect completes the handshake, checks the contract version, and hands the
// instance over. Every failure here kills the process rather than leaving a
// half-initialized plugin reachable.
func (h *Host) connect(client *goplugin.Client, spec svc.BackendSpec, dataDir string) (*pluginProcess, error) {
	protocol, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("start plugin %s: %w", spec.ImageID, err)
	}
	raw, err := protocol.Dispense(backendPluginName)
	if err != nil {
		return nil, fmt.Errorf("connect to plugin %s: %w", spec.ImageID, err)
	}
	backend, ok := raw.(appplugin.Backend)
	if !ok {
		return nil, fmt.Errorf("plugin %s served an unexpected type %T", spec.ImageID, raw)
	}
	descriptor, err := backend.Describe()
	if err != nil {
		return nil, fmt.Errorf("describe plugin %s: %w", spec.ImageID, err)
	}
	if descriptor.APIVersion != appplugin.APIVersion {
		return nil, fmt.Errorf(
			"plugin %s reports contract version %d, this server speaks %d",
			spec.ImageID, descriptor.APIVersion, appplugin.APIVersion)
	}
	if err := backend.Init(instanceOf(spec, dataDir)); err != nil {
		return nil, fmt.Errorf("initialize plugin %s: %w", spec.ImageID, err)
	}
	return &pluginProcess{client: client, backend: backend, descriptor: descriptor}, nil
}

// instanceOf projects an installed instance into the view a plugin gets. It
// carries the resolved environment, secrets included: a plugin runs on the
// host on the image's behalf, and an image's plugin needs the password its own
// install script generated.
func instanceOf(spec svc.BackendSpec, dataDir string) appplugin.Instance {
	instance := spec.Instance
	instance.DataDir = dataDir
	return instance
}

func (h *Host) lookup(instanceID string) *pluginProcess {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running[instanceID]
}

// kill terminates an instance's process. The caller holds its launch lock.
func (h *Host) kill(instanceID string) {
	h.mu.Lock()
	current := h.running[instanceID]
	delete(h.running, instanceID)
	h.mu.Unlock()

	if current != nil {
		current.stop()
	}
}

func (h *Host) dataDir(instanceID string) string {
	return filepath.Join(h.root, "data", instanceID)
}
