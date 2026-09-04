# Adding an agent

Adding an integration means implementing one provider-owned module and adding
its factory constructor to the explicit configuration composition list. Shared
services should not gain a parallel provider switch or a second provisioning
list.

## Decide the contract first

Before writing code, answer these questions:

| Decision | Choices and consequence |
| --- | --- |
| Execution scope | `host` enables loose chats; `project` enables project chats and requires a profile |
| Runtime | Local CLI providers need command execution and usually a profile; a host-only remote API may omit the profile |
| Authentication | Reuse `managed-code`, `managed-device`, `managed-api-key`, `external`, or `none`; a new flow requires a deliberate shared-contract and UI change |
| Access gate | Only observable managed or no-auth modules can satisfy onboarding |
| Sessions | Declare resume only when the adapter can reliably resume; fork also requires resume |
| Skills | Choose `none`, `slash-command`, `dollar-mention`, or `instructions` to match what the runtime actually accepts |
| Modes | The shared saved-mode contract currently supports `default` and `plan`; a different native mode needs a coordinated backend/settings/frontend contract change |
| Extra tools | Declare Browser or scheduled tools only when shared preparation installs their assets and the adapter supplies required native wiring/runtime environment |
| Persistence | Identify the smallest provider-owned directories that must survive container replacement |

Choose one stable ID matching `[a-z][a-z0-9-]*`. Provider IDs appear in chats,
sessions, skills, settings, routes, and capability results, so changing one is
a data migration, not a cosmetic rename. A built-in may add a named
`agent.ProviderID` constant in
[`model.go`](../../../backend/internal/agent/model.go), but the catalog—not the
constant—registers it.

## Create the provider package

Use the existing packages under
[`backend/internal/integration/agents`](../../../backend/internal/integration/agents) as examples. A
typical integration looks like:

```text
backend/internal/integration/agents/acme/
├── factory.go              complete module declaration and construction
├── provider.go             Provider implementation and dependencies
├── command.go              provider-native launch arguments and environment
├── parser.go               run output to normalized agent events
├── capabilities.go         live discovery plus conservative fallback
├── capability_parser.go    provider-native model/control parsing
├── auth.go                 optional managed auth adapter
├── profile.go              optional host/project provisioning policy
└── assets/                 optional instructions or MCP templates
```

Files are responsibilities, not a mandatory template. Keep a protocol split
into more files when useful, and omit auth/profile files when the declared
contract does not need them. Do not move provider-native JSON, flags, fallback
catalogs, credential formats, or session extraction into shared services.

## Implement runtime and discovery

Implement `agent.Provider`:

1. `ID` returns the exact stable ID declared by the factory.
2. `Capabilities` probes the selected host or project execution environment,
   parses provider-native output, and returns normalized models, per-model
   controls, modes, defaults, warnings, and availability. Warning and
   unavailability text is user-facing: keep it concise and do not copy raw CLI
   output or credentials into it.
3. `Run` translates `agent.RunRequest` into the native CLI/protocol, honors
   context cancellation, and emits normalized `agent.Event` values.

Use the injected `agent.ProjectPreparer` rather than importing project-service
models or copying the start/provision/secret sequence. The generic module
factory constructs it; declare only genuine differences with
`module.WithProjectPreparation`. Its callback dependencies do not expose the
project resolver or the full container-provisioning port set. Use
`runtime.BuildContainerCommand` for the common `lxc exec` envelope. Keep host
commands, provider-native arguments, prompt transport, run protocol, and both
command and capability parsers provider-local. See
[Runtime and event parsing](02-runtime-and-event-parsing.md) and
[Capabilities, cache, and refresh](03-capabilities-cache-and-refresh.md) for
the required normalization and failure behavior.

Add the compile-time assertion near the factory:

```go
var _ agent.Provider = (*Provider)(nil)
```

## Declare provisioning policy

A project-scoped module must expose a complete `Profile()`. A host-scoped local
CLI also needs one so the updater can converge the host binary. Declare:

- display name, binary, exact semver pin, and provider-specific version args;
- install mode and exact npm package or pinned installation script;
- positive install and concurrent-install wait timeouts plus verification policy;
- project image label when project scope is enabled;
- credentials to synchronize, durable state mounts, shared instructions,
  non-secret runtime assets, workspace skill links, and Browser MCP
  templates as needed.

Add the version to
[`provisioning/versions.env`](../../../backend/internal/agent/provisioning/versions.env)
and read it through `provisioning.MustCLIVersion`; use `MustPin` for associated
checksums. Do not duplicate literal pins in provider code, tests, or docs.

Persistent-state device names, host directories, and container paths must be
unique; container paths must not overlap another provider's mount. Never put
mutable provider state in the replaceable container root when it must survive
an upgrade. See [Provisioning and updates](05-provisioning-and-updates.md).

## Put configuration in the owning layer

Use [`internal/config`](../../../backend/internal/config) only for application
composition and cross-provider deployment policy. The current global agent
settings are the capability-probe deadline, host CLI version-probe deadline,
healthy/degraded capability-cache TTLs, post-run credential-sync timeout, and
agent-browser idle TTL. Config loads those values and injects service-owned
option values; provider packages must not import `internal/config`.

Keep provider-specific policy in the provider package:

- binary, package, version arguments, install mechanism, install/wait limits,
  credentials, mounts, instructions, runtime assets, and Browser templates belong in
  `Profile()`;
- login timing and API-key creation URLs belong in the provider's auth
  configuration because upstream protocols differ;
- CLI arguments, probe commands, parser limits, fallbacks, and protocol
  backstops stay beside the adapter that understands them.

Do not add a global config field merely because a value is a duration. Add one
only when the policy applies across providers and the application operator—not
the upstream CLI—owns it.

## Build the provider-owned factory

`factory.go` joins the declarations and constructors. This managed-device
example is illustrative:

```go
const providerID agent.ProviderID = "acme"

func NewFactory() (module.Factory, error) {
    profile := Profile()
    return module.NewFactory(module.Descriptor{
        ID:                  providerID,
        Label:               "Acme",
        ExecutionScopes:     []module.ExecutionScope{module.ScopeHost, module.ScopeProject},
        Auth:                module.AuthManagedDevice,
        AuthInstructions:    "Sign in with the displayed Acme device code.",
        SatisfiesAccessGate: true,
        Features: module.Features{
            Sessions:       module.SessionSupport{Resume: true},
            Skills:         module.SkillsInstructions,
            BrowserTools:   true,
            ScheduledTools: true,
        },
    }, &profile, func(
        deps module.Dependencies,
        validatedProfile *provisioning.Profile,
    ) (module.Components, error) {
        binding := agentauth.NewDeviceBinding(providerID, NewAuth())
        return module.Components{
            Provider: newProvider(
                deps.ProjectPreparer,
                deps.CredentialCollector,
                *validatedProfile,
                deps.CredentialSyncTimeout,
            ),
            Auth: &binding,
        }, nil
    }, module.WithProjectPreparation(module.ProjectPreparationPolicy{
        BrowserAssets:     true,
        BrowserMCPRuntime: true,
    }))
}

var _ module.FactoryBuilder = NewFactory
```

Declare only the preparation differences the adapter needs:

| `ProjectPreparationPolicy` field | Effect |
| --- | --- |
| `CLIErrorOperation` | Replaces the shared provider-derived prefix when CLI readiness fails. |
| `CredentialErrorOperation` | Replaces the shared prefix for credential precheck or seeding failures. |
| `BeforeCredentials` | Receives an isolated profile clone immediately before generic credential seeding; it runs only when the profile has credential policy. |
| `SkillLinksRequired` | Makes workspace compatibility-link failure fatal instead of best effort. |
| `BrowserAssets` | Re-publishes the shared browser skill and script on every prepared run; failures remain best effort. |
| `BrowserMCPRuntime` | When a policy-gated request enables Browser, provisions MCP and starts browser core; failures are fatal and the descriptor must declare `BrowserTools`. |

With no option, a project-scoped module still receives the common default
preparer: project reconciliation, CLI readiness, profile credentials,
instructions, best-effort skill links, requested schedule tooling,
`boot.autostart`, and best-effort secret loading.

Construct provider and auth state inside the callback. Do not capture a mutable
auth service outside it or return an auth binding whose ID/flow differs from
the descriptor. `AuthNone` returns a nil binding; external auth uses an
external binding with instructions but cannot be an access-gate provider. See
[Authentication and access](04-authentication-and-access.md).

`Catalog.Build` receives application-facing `module.BuildDependencies` with the
project resolver, full container ports, and credential-sync timeout. The module
factory uses them to construct project preparation, then exposes only
`module.Dependencies{ProjectPreparer, CredentialCollector,
CredentialSyncTimeout}` to this callback. That projection withholds the
project resolver and full container ports. Adapters are expected to use the
shared preparer; direct project-service imports or copied preparation
orchestration violate the integration contract.

The callback receives a defensive copy of the exact profile already validated
by the factory and catalog. The factory-owned preparer retains a separate deep
clone of the same profile. Pass the callback snapshot only to provider-native
or post-run behavior that needs it; do not call `Profile()` again in
`newProvider`. A host-only integration that deliberately has no profile
receives `nil`, has no project preparer, and must not dereference the profile.

Retain `CredentialCollector`, the profile, and the sync timeout only when the
provider pulls refreshed credentials after a successful project run. Otherwise
the provider constructor should take only the preparer and its native execution
settings, such as the validated CLI binary. A provider callback never receives
the credential push port; pre-run seeding belongs to shared preparation.

Factory validation rejects project-preparation policy on a host-only module,
Browser MCP runtime preparation without `BrowserTools`, and credential hooks
or credential-error overrides when the profile has no credential policy. A nil
factory option is also invalid.

If the new module should be the default, remove the current default declaration
before setting `Default: true`; the catalog rejects multiple defaults. Confirm
the module supports `host`, because defaults must be usable for loose chats.

## Register it

Add only the factory constructor to the ordered builder list in
[`config/agents.go`](../../../backend/internal/config/agents.go):

```go
builders := []module.FactoryBuilder{
    claude.NewFactory,
    codex.NewFactory,
    minimax.NewFactory,
    kimi.NewFactory,
    antigravity.NewFactory,
    acme.NewFactory,
}
```

The selected position controls presentation and fallback-default order. Do not
add a package `init`, a global mutable registry, a hard-coded auth card, a
separate host installer entry, or a separate base-image entry. Catalog
descriptors and profile projections drive those generic paths.

The frontend accepts provider IDs as strings and renders existing auth modes,
models, controls, and feature metadata generically. Provider-specific frontend
code is needed only for an intentionally new contract or bespoke product
surface, not for ordinary registration. New providers use the generic
provider-keyed session map; do not add another legacy `...SessionID` field.

## Test the complete module

Add focused tests for every provider-owned boundary:

- command arguments, environment, cancellation, resume, and session behavior;
- factory wiring for the provider's project-preparation options;
- run and capability parser fixtures, including malformed and partial output;
- live, fallback, warning, and unavailable capability results;
- authentication status and lifecycle when auth is managed;
- exact profile pin, version arguments, install policy, persistent state, and
  assets;
- factory identity, scope, auth flow, features, and fresh component builds;
- configuration catalog order, default, project profiles, and host profiles.

Extend `internal/service/agent/execution` tests for changes to common project
lifecycle, ordering, error, or best-effort semantics. Extend
`internal/integration/agents/runtime` tests for changes to common
container-command and environment precedence. Keep native arguments and
protocol assertions in the provider package.

Extend transport/frontend tests only when the shared contract changes. Add an
infrastructure or release-classifier regression whenever a new profile path,
install mechanism, asset location, or update-sensitive file convention is
introduced.

Run the repository checks from its root:

```bash
cd backend
go build ./...
go test ./...
go vet ./...

cd ../frontend
npm test
npm run build

cd ..
for test_script in infra/tests/*-test.sh; do
    bash "$test_script"
done
bash .github/scripts/classify-release-test.sh
git diff --check
```

Use targeted `go test -race` coverage for the new provider and any concurrent
catalog, auth, prompt, or provisioning service it changes.

## Document and release it

Update this guide when the shared contract changes. For a normal new provider,
also update the user-facing agent controls/auth instructions, relevant
workspace architecture pages, API documentation if the surface changed,
operations, known limitations, and the threat model when credentials or
network powers change.

When a local/project provider adds a preinstalled CLI or durable provider home,
also update the embedded
[`provisioning/assets/AGENTS.md`](../../../backend/internal/agent/provisioning/assets/AGENTS.md)
workspace instructions. That template currently enumerates the installed CLIs,
provider-home paths, and durable-mount count; those human-facing details are not
generated from the module catalog.

A new module, provisioning profile, CLI pin, durable mount, host installation,
or base-image dependency requires a minor or major release and the full
infrastructure updater. An application-only deploy does not converge host CLIs,
rebuild the base image, or replace existing project containers. Validate that
full path in QA before publishing the release. See
[Deployment and operations](../../04-operations/09-deployment-and-operations.md#update-flow).

## Completion checklist

- The provider owns its factory, runtime translation, parsers, auth adapter,
  capability discovery, and provisioning policy.
- `config.NewAgentModules` is the only registration edit.
- Factory and catalog validation pass without weakening an invariant.
- Host/project scopes and all feature declarations match demonstrated behavior.
- Generic APIs and frontend surfaces show the provider without a special-case
  list.
- State survives the replacement boundary declared by the profile.
- Embedded workspace instructions reflect any new preinstalled CLI, provider
  home, or durable mount.
- Backend, frontend, race, infrastructure, and release tests pass.
- Documentation, security implications, release classification, and the full
  update path are covered.
