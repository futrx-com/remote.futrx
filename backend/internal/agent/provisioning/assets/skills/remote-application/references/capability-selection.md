# Remote application capability selection

Use this reference to translate requested behavior into the smallest package shape. Confirm exact schemas in the current repository documentation before writing files.

## Package shape

```text
<application>/
  README.md
  application.json
  infra/
    install.sh                  optional custom container provisioning
  backend/
    main.go                     optional host executable and composition root
    api/                        optional request-handling package
    lifecycle/                  optional publisher/subscriber ownership
    container/                  optional programs built inside the target LXD container
      main.go                   one binary named after the application ID, or
      cmd/<binary>/main.go      explicitly named or multiple binaries
  ui/                           optional browser extension
    scripts/main.js
    style/*.css
    views/*.html
    assets/*
  skills/<skill>/SKILL.md       optional workflow published into projects
```

The host backend and container programs are different build contexts. Do not add `go.mod`, `go.sum`, `go.work`, or `go.work.sum` to the host backend tree; Remote generates that module and excludes `backend/container/`. A container tree may use the separately documented container build behavior.

## Map requirements to capabilities

| User requirement | Select | Do not invent |
|---|---|---|
| Browser button, panel, popup, or contextual action | `ui/` | backend or container infrastructure when no server work exists |
| Server-side computation or protected host operation called by the UI | `backend/main.go` plus `backend/api/` | LXD container, port, or service |
| Command that must execute inside the project/application container | `backend/container/` | host backend unless the browser or Remote server must call it |
| Long-running process inside a container | manifest `service`; usually a container-built or provisioned executable | a systemd unit written by `infra/install.sh` |
| Custom account, directory, package, configuration, or state provisioning | idempotent `infra/install.sh` | generic service lifecycle already represented in the manifest |
| Network listener that users or a host backend must reach | `port.internal` and only the needed port options | connection metadata or credentials unrelated to that listener |
| Service readiness | `healthcheck` tied to the actual internal listener | a placeholder endpoint |
| Configurable install value | `env[]`, consumed by installer, service, or backend | unused inputs |
| Real connection credentials for a database/service | `connection`, mapped to real declared env values | user/password/database fields for an app with no such resources |
| Host CLI downloaded for backend use | checksum-pinned `hostTools[]` | mutable or unverified downloads |
| Business event emitted by the application | manifest `publishers[]` plus one typed lifecycle owner per publisher | an application-implemented runtime or publisher registration hook |
| Consumption of another publisher's event | `subscriptions[]` plus `EventSubscriber` behavior | subscriptions merely to observe core lifecycle; core publishes its lifecycle automatically |
| Agent workflow installed with the app | `skills/<name>/SKILL.md` | a project skill for behavior unrelated to the application |

## Host backend boundaries

```text
backend/main.go
  receives core Runtime
  constructs concrete lifecycle publishers
  injects their interfaces into api.New(...)
  calls rpc.Serve or rpc.ServeWithRuntime

backend/api/
  implements Describe, Init, and Handle
  owns routes and request/response translation
  invokes injected business capabilities

backend/lifecycle/<publisher>.go
  owns the API-facing interface for that publisher
  owns the concrete emitter
  owns local publisher/event/version constants and payload construction
```

Multiple manifest publishers are multiple lifecycle owners, even when every concrete publisher receives the same core-owned `applications.EventEmitter`. Do not collapse them into a generic application publisher just to reduce constructor arguments.

## Questions that justify clarification

Ask the user only when the answer cannot be inferred and changes the product substantially, for example:

- global, project, or both scopes;
- built-in catalog entry versus distributable uploaded package;
- whether a process is one-shot or supervised;
- whether data must survive stop, uninstall, or project deletion;
- whether a UI action needs privileged server work;
- the actual schema and audience of an event payload;
- whether a value is a secret or ordinary configuration.

Names, descriptions, sensible initial version strings, internal helper structure, and ordinary test placement usually do not require another question.

## Completion pressure test

Before handoff, account for every manifest field and folder:

- What real resource or behavior does it represent?
- Which implementation consumes it?
- How can a user or test observe it?
- Who owns its lifecycle and cleanup?
- Does uninstall remove it, intentionally leave it in a project container, or delete a dedicated global container?
- Can any secret cross into logs, ordinary API responses, health output, or documentation?

Remove anything without a concrete answer.
