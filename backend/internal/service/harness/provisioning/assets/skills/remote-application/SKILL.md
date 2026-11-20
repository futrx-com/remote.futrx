---
name: remote-application
description: Create or extend an installable remote.futrx application from product requirements. Use when asked to scaffold or build a catalog application, design its application.json, or add application-owned infrastructure, container programs, a host backend, publishers, UI extensions, or project skills. Do not use for unrelated Remote core features.
---

# Remote Application

Turn the user's requested behavior into a complete, minimal Remote application. Build only capabilities backed by real implementation; never add demonstration fields, credentials, database metadata, services, or events that the application does not actually have.

## Establish the target

1. Locate the `remote.futrx` repository root and inspect its instructions, Git status, and existing application/package context before editing.
2. Determine the application ID, display name, supported scope, delivery form, and observable behavior from the request. Ask only when a missing choice would materially change the product; otherwise make a narrow, explicit assumption.
3. Decide whether the user wants a built-in catalog entry under `applications/<id>/` or a standalone directory/ZIP for upload. Preserve the repository's policy when the location is already clear.
4. Read [references/capability-selection.md](references/capability-selection.md) before selecting folders or manifest fields.

## Read the live contract

The repository documentation is authoritative and may have changed since this skill was written. Always read these files from the current checkout before implementing:

- `applications/README.md`
- `docs/dev/installable-applications/01-overview.md`
- `docs/dev/installable-applications/02-application-json.md`
- `docs/dev/installable-applications/03-application-capabilities.md`
- `docs/dev/installable-applications/11-testing.md`
- `docs/dev/installable-applications/13-security-model.md`

Then read only the capability-specific documents that apply:

- container provisioning, commands, a manifest service, ports, or health checks: `04-install-scripts.md` and `17-versions-and-upgrades.md`
- UI contributions: `05-slots.md`, `06-extension-api.md`, `08-scoping-and-visibility.md`, and `09-styling-and-icons.md`
- host backend: `15-application-backends.md`
- uploaded package or ZIP: `16-uploaded-packages.md`
- publishers or subscriptions: `18-application-events.md`

Use `applications/hello-remote/` as the worked architectural example when the request involves multiple layers, a service, or events. Do not copy its unrelated showcase capabilities.

## Design the smallest honest application

- Start with `README.md` and `application.json`. The directory name and manifest `id` must match; `version` is a non-empty string; scopes must reflect where the behavior can actually run.
- Declare only inputs a user can meaningfully configure and the code consumes. Omit `connection` unless a real user, password, or database exists.
- Treat secrets as secrets across the manifest, logs, UI responses, health checks, and documentation.
- Prefer platform-owned mechanisms over application-owned plumbing: manifest services own systemd, declared ports own LXD proxying, health checks own readiness, and host tools own verified downloads.
- Put only genuinely custom, idempotent container provisioning in `infra/install.sh`.
- Keep host execution and container execution separate. The host backend root is `backend/main.go`; request handling belongs in `backend/api/`; container programs belong in `backend/container/`.
- Keep `backend/main.go` as the composition root. It constructs dependencies and serves the API; it does not own request behavior or publisher payloads.
- For each application publisher, keep its consumer-facing interface, concrete emitter, event identity, and payload construction together under `backend/lifecycle/`. Construct each publisher separately in `backend/main.go` from core-owned `Runtime.Events` and inject its interface into the API.
- The manifest is the publisher registry. Application code chooses when business events occur; core validates and dispatches emissions. Do not add `subscriptions` unless the application truly consumes another publisher's events.
- Keep UI modules disposal-safe and scoped. Use Remote's extension APIs and theme tokens; do not reach into undocumented SPA internals.
- Add an application-owned skill under `skills/<name>/SKILL.md` only when installing the application should publish that workflow into a project.

## Implement a vertical slice

Build the shortest end-to-end path that proves the requested behavior:

1. Write the manifest and package README.
2. Add only the selected capability folders.
3. Connect each declared field to implementation and an observable outcome.
4. Add focused tests at the owner of each behavior. For events, test publisher name, event name, version, payload, and the API trigger separately.
5. Update application documentation with install, use, verification, upgrade, and cleanup behavior that actually exists.
6. Bump an existing application's version when installed copies must reconverge. A brand-new application starts at the version chosen by the user or the repository convention.

Avoid speculative abstractions, placeholder screens, fake resources, and broad showcase behavior. A user asking for one tool should get one coherent tool, not another Hello Remote.

## Validate proportionally

Run focused checks first, then the repository suites affected by the package:

- `jq empty applications/<id>/application.json`
- `bash -n` for each shell script
- application package tests and the catalog registry tests
- `go test ./...` from the catalog module when Go source is present
- `go test ./...` from `backend/` for catalog, installer, host-builder, or core integration changes
- `npm test` and `npm run build` from `frontend/` when UI behavior or its contract changed
- `go vet ./...` in every changed Go module when the change is ready to hand off
- `git diff --check` and a final search for stale names, placeholder fields, secret exposure, and undeclared or unused manifest capabilities

When the app has a host backend, include a test that compiles it through Remote's generated-module builder rather than relying only on the repository module. When it provisions a container service, test the manifest contract and provide safe runtime verification steps.

Do not push, upload, install, deploy, or mutate a live catalog unless the user explicitly asks. Report the files created, capabilities selected, checks run, and any intentionally omitted capability.
