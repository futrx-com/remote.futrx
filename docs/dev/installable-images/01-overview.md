# 01 — Overview

## What the catalog is

The catalog is a directory of subdirectories. Each subdirectory is one
installable image:

```
images/
  docs/              ← this documentation (reserved name, not an image)
  mysql/
    image.json       metadata
    install.sh       provisioner, run inside a container
    ui/              browser extension (optional)
    plugin/          Go backend, compiled and run on the host (optional)
  postgresql/
  redis/
  ui-playground/       fixture: extension only, no container
  ui-sandbox/          fixture: extension only, no container
  backend-playground/  fixture: Go plugin plus the UI that calls it
```

The whole tree is compiled into the server binary with `//go:embed images` in
[`registry.go`](../../../backend/internal/integration/containers/applications/registry.go). There is no runtime plugin directory, no
upload endpoint, and no way to add an image to a running server: adding one
means adding a directory and rebuilding. That single fact drives most of the
design, and the whole of the [security model](13-security-model.md).

Adding an image requires **no code changes**. `NewRegistry()` walks the
directory at startup, validates every entry, and the new app appears in the
Applications tab.

## The three halves of an image

```
                         image.json
                    /         |         \
          install.sh        plugin/        ui/
               |               |             |
     runs in a container   runs on the   runs in the browser
     (a service on a port)  host as a    (buttons, panels, popups)
                            process
```

An image may have any of them, or all three:

| Image | `install.sh` | `plugin/` | `ui/` | What it is |
|---|---|---|---|---|
| `postgresql` | yes | no | no | a database |
| `mysql` | yes | no | yes | a database that also adds a "Connect" action |
| `ui-playground` | no | no | yes | a pure UI plugin |
| `backend-playground` | no | yes | yes | a Go backend and the UI that calls it |

The `type` field in `image.json` says which shape it is — see
[03 — Image types](03-image-types.md). `plugin/` is covered in full by
[15 — Backend plugins](15-backend-plugins.md).

## The moving parts

```mermaid
flowchart TB
    subgraph Frontend["frontend/src"]
        FE_Section["ui/applications/ApplicationsSection.tsx"]
        FE_Catalog["ui/applications/ApplicationCatalog.tsx"]
        FE_Installed["ui/applications/InstalledApplications.tsx"]
        FE_Packages["ui/applications/ApplicationPackages.tsx"]
        FE_Hook["state/hooks/applications/useApplications.ts"]
        FE_Api["api/applicationsApi.ts<br/>api/project/projectApplicationsApi.ts"]
        FE_ExtHost["app/extensions/extensionHost.ts"]
        FE_ExtBackend["app/extensions/extensionBackend.ts"]
    end

    subgraph HTTP["transport/http/handlers — routes, authorization, JSON"]
        H_Main["applications_handler.go"]
        H_UI["applications_ui_handler.go"]
        H_Backend["applications_backend_handler.go"]
        H_Packages["applications_packages_handler.go"]
    end

    subgraph Service["service/applications — policy"]
        S_Service["service.go / install.go<br/>lifecycle.go / upgrade.go"]
        S_UIExt["ui_extensions.go"]
        S_Backend["backend.go"]
        S_Packages["packages.go"]
    end

    subgraph Integration["integration/containers/applications — the catalog and lxc"]
        I_Registry["registry.go<br/>validates the catalog, serves ui/ assets and plugin/ source"]
        I_RegParts["registry_ui.go / registry_plugin.go<br/>registry_packages.go / registry_skills.go"]
        I_Payload["container_payload.go<br/>stages container.tar.gz into the install script"]
        I_Installer["installer.go<br/>lxc launch, install.sh, systemd, proxy device"]
        I_Allocator["allocator.go — a free host port"]
        I_Packages["packages.go — uploaded .zip packages"]
    end

    subgraph Support["Supporting packages"]
        P_PluginHost["integration/pluginhost<br/>compiles plugin/, runs it over go-plugin"]
        P_FileApps["stores/fileapplications<br/>global.json, projects/{id}.json"]
        P_HostTools["integration/hosttools<br/>checksum-pinned host binaries"]
    end

    subgraph Catalog["images/ — embedded by go:embed"]
        C_Hello["hello-remote/<br/>the worked example: plugin/ + ui/"]
    end

    FE_Section --> FE_Catalog
    FE_Section --> FE_Installed
    FE_Section --> FE_Packages
    FE_Catalog --> FE_Hook
    FE_Installed --> FE_Hook
    FE_Packages --> FE_Hook
    FE_Hook --> FE_Api
    FE_ExtHost --> FE_Api
    FE_Api --> H_Main
    FE_Api --> H_UI
    FE_Api --> H_Packages
    FE_ExtBackend --> H_Backend

    H_Main --> S_Service
    H_UI --> S_UIExt
    H_Backend --> S_Backend
    H_Packages --> S_Packages

    S_Service --> I_Registry
    S_Service --> I_Installer
    S_Service --> I_Allocator
    S_Service --> P_FileApps
    S_UIExt --> I_Registry
    S_Backend --> P_PluginHost
    S_Packages --> I_Packages

    I_Registry --> I_RegParts
    I_Registry --> I_Payload
    I_Registry -. go:embed .-> C_Hello
    I_Installer --> P_HostTools
    I_Packages -. uploaded packages join the catalog .-> I_Registry
    P_PluginHost -. reads plugin/ source from .-> I_Registry
```

Every arrow out of the service layer crosses an interface it declares itself:
`service/applications/ports.go` names `Registry`, `Installer`, `BackendHost`,
`Store` and `PortAllocator`, and the packages on the right implement them. That
is what keeps the domain testable without LXD, a Go toolchain, or a disk.

### Backend

| Layer | File | Responsibility |
|---|---|---|
| integration | `containers/applications/registry.go` | loads and validates the embedded catalog; serves `ui/` asset bytes and `plugin/` source |
| integration | `containers/applications/installer.go` | everything `lxc`-facing: containers, install scripts, proxy devices |
| integration | `pluginhost/` | everything toolchain- and process-facing: compiling `plugin/`, running it, forwarding calls |
| contract | `pkg/appplugin` | the types and interface a plugin is written against |
| service | `service/applications/service.go` | policy: install, lifecycle, which extensions a caller may load |
| service | `service/applications/backend.go` | policy: who may call a plugin, and when |
| transport | `transport/http/handlers/applications_handler.go` | routes, authorization, JSON |
| transport | `transport/http/handlers/applications_backend_handler.go` | forwarding a request to a plugin and its answer back |

The layering is strict: transport → service → integration. A handler never
runs `lxc` and never launches a process; the registry never decides who may see
what. `pkg/appplugin` sits outside the layering on purpose: it is the public
contract, so it depends on nothing but the standard library.

### Frontend

| File | Responsibility |
|---|---|
| `app/extensions/extensionHost.ts` | fetches the allowed extension list, injects CSS, imports entry modules |
| `app/extensions/extensionApi.ts` | builds the `remote` object handed to each extension |
| `state/stores/extensions/extensionStore.ts` | owns registered contributions and install visibility |
| `state/hooks/extensions/extensionContributionState.ts` | decides which contributions apply to a surface |
| `app/extensions/extensionBackend.ts` | resolves which running plugin a call reaches, and calls it |
| `config/extensions.ts` | the closed set of slot names and their icon sizing |
| `app/extensions/extensionPopup.ts` | the modal an extension can open |
| `ui/primitives/ExtensionSlot.tsx` | renders a slot's contributions into plain DOM nodes |

## What installing does

An install crosses every layer above, and what it actually provisions depends
entirely on the image's type — which is the single most surprising thing about
this subsystem, and the reason a `ui` or `backend` image works on a host with
no container runtime at all.

```mermaid
sequenceDiagram
    actor User
    participant UI as ApplicationCatalog.tsx
    participant API as applications_handler.go
    participant Svc as service/applications
    participant Reg as applications.Registry
    participant Store as stores/fileapplications
    participant Host as pluginhost
    participant Proj as project service
    participant Inst as applications.Installer
    participant LXD as LXD

    User->>UI: Install
    UI->>API: POST /api/applications<br/>or /api/projects/{id}/applications
    API->>Svc: Install(request)
    Svc->>Reg: the image, and its install script
    Reg-->>Svc: Image + script bytes (payload already staged)
    Svc->>Svc: resolve env — defaults, generated secrets, required fields

    alt ui or backend image
        Note over Svc,LXD: no container, no port, no proxy device
        Svc->>Store: persist as running
        Svc->>Host: Ensure — compile plugin/ if stale, start the process
    else service or tool image
        alt project scope
            Svc->>Proj: container name, and ready it
        else global scope
            Svc->>Svc: name a dedicated container
        end
        opt type == service
            Svc->>Svc: allocate a free host port, from defaultExternal up
        end
        Svc->>Store: persist as installing — a crash here stays recoverable
        Inst->>LXD: launch the dedicated container (global scope only)
        Inst->>LXD: run install.sh as root, then start the systemd unit
        opt type == service
            Inst->>LXD: add the proxy device that maps the host port
        end
        Svc->>Host: Ensure, when the image ships a plugin/ too
        Svc->>Store: persist as running
    end

    Svc-->>API: the instance
    API-->>UI: 200 — it appears under Installed
```

## How an extension reaches the screen

```
1. User installs an image        POST /api/applications
                                 (or /api/projects/{id}/applications)
                                          |
2. SPA re-syncs                  GET /api/applications/ui
                                 → [{ image, global, projectIds }]
                                          |
3. Host injects stylesheets      <link href="/api/applications/catalog/
                                          <id>/ui/style/*.css">
                                          |
4. Host imports the entry        import("/api/applications/catalog/
                                          <id>/ui/scripts/main.js")
                                          |
5. Entry module registers        remote.ui.addIconButton(slot, {...})
                                          |
6. A slot renders it             <ExtensionSlot name="chat.header.actions" />
                                 → contributions filtered by scope + `when`
                                 → each draws into its own <div>
                                          |
7. It calls its own backend      remote.backend.call("health")
                                 → /api/applications/<instance>/backend/health
                                 → the image's compiled Go plugin
```

Steps 2–6 repeat whenever the installed set changes — install, uninstall,
start, or stop — so a contributed button appears and disappears without a page
reload.

## Three gates, in order

An extension has to pass all three before anything of it appears:

1. **It must be in the build.** The catalog is embedded; there is no runtime
   installation of new images.
2. **The user must have installed it**, and the instance must be *running*.
   Being in the catalog puts nothing on anyone's screen. See
   [12 — HTTP API](12-http-api.md) for `GET /api/applications/ui`.
3. **The surface must be in scope.** A globally installed extension renders
   everywhere; one installed in a project renders only inside that project.
   See [08 — Scoping and visibility](08-scoping-and-visibility.md).

Gate 1 is a build decision. Gate 2 is enforced by the backend. Gate 3 is
enforced by the frontend registry, per contribution, on every render.

## Design decisions worth knowing

**Slots are a closed set.** An extension cannot render anywhere it likes. A
slot is a promise about where something draws and what context it receives, and
that promise is what survives refactors of the surrounding UI. Adding a slot is
a deliberate change to the SPA. See [05 — Slots](05-slots.md).

**Contributions are plain DOM.** An extension gets an `HTMLElement` and draws
into it however it likes. It never touches the component tree, so extension
code is ordinary JavaScript rather than Preact, and a throwing extension cannot
break the render of the surface around it.

**Failure is always local.** A broken entry module, a throwing predicate, a
throwing click handler, a missing view — each is caught and logged, and costs
that one extension its own UI. Everything else keeps working.

**The catalog fails loudly.** A malformed `image.json`, a `ui` block naming a
file that does not exist, a `type` that declares a port it cannot have — all of
these fail `NewRegistry()`, which means the build and the tests fail. A broken
image never reaches a browser as a 404.
