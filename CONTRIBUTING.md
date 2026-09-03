# Contributing to remote.futrx

Thanks for your interest in improving remote.futrx — a self-hosted workspace for Claude Code, Codex, MiniMax, Kimi Code, Antigravity, and future agent integrations.

Before you start, please read this document. It covers how the repository is laid out, how to build and test each part, and what we expect from commits and pull requests.

## Licensing and sign-off

remote.futrx is licensed under the [GNU AGPL-3.0](LICENSE). By contributing, you agree that your contributions are licensed under the same terms.

We use the [Developer Certificate of Origin](https://developercertificate.org/) (DCO) instead of a CLA. Sign off every commit to certify that you have the right to submit the change:

```bash
git commit -s
```

This appends a `Signed-off-by: Your Name <your@email>` line to the commit message.

## Repository layout

| Path | What it is |
| --- | --- |
| `backend/` | Go backend: HTTP/WebSocket transport, services, file-backed stores, LXD/Git/tmux integrations, and compiled-in agent modules |
| `frontend/` | Preact + Vite SPA. The production build is written to `backend/public/` and embedded into the Go binary via `go:embed` |
| `infra/` | Installer, updater, systemd/Caddy templates, base-image tooling, and shell test suite |
| `docs/` | Architecture and subsystem deep-dives — start with `docs/01-overview/01-system-overview.md` |

## Adding an agent integration

Agents are explicit compiled-in modules, not runtime plugins. Each provider
owns one validated factory declaration that binds its static descriptor,
provisioning profile, and project-preparation policy to fresh runtime and
authentication components. The generic factory constructs shared project
preparation and narrows application dependencies before invoking provider
code. The configuration composition root owns only reviewed order and
application-wide policy; the module catalog validates the complete set,
selects defaults, projects host/project profiles, and builds one consistent
runtime.

Read the [agent integration developer guide](docs/dev/agents/README.md) for the
complete registration, capability-discovery, authentication, execution,
event-parsing, provisioning, frontend, testing, and release flow. Its
[adding-an-agent checklist](docs/dev/agents/07-adding-an-agent.md) is the source
of truth for new integrations.

## Development setup

### Prerequisites

- **Go** 1.25+ (see `backend/go.mod`)
- **Node.js** 22.14+ (matches CI)
- **Linux with LXD** — only for running the full stack. Project workspaces are LXD containers, so the complete application only runs on a Linux host with LXD installed. Backend and frontend unit tests, builds, and most development run fine on macOS or any platform without LXD.

### Backend

```bash
cd backend
go build ./...
go test ./...
```

Run the server locally with `go run ./cmd/remote`. It listens on port `7682` by default (`PORT` overrides it). Endpoints that touch project workspaces need LXD; everything else (auth, chat stores, settings) works against the local filesystem.

### Frontend

```bash
cd frontend
npm install
npm run dev
```

The Vite dev server runs on port `5174` and proxies `/api` and `/ws` to a local backend on `127.0.0.1:7682`, so run the Go server alongside it.

Tests use the built-in Node test runner:

```bash
npm test
```

`npm run build` compiles the SPA into `backend/public/`; the next `go build` embeds it.

### Infra

Installer and deployment logic has its own shell tests:

```bash
bash infra/tests/health-check-test.sh
bash infra/tests/go-toolchain-test.sh
bash infra/tests/host-agent-install-test.sh
bash infra/tests/dns-resolve-test.sh
bash infra/tests/container-forwarding-test.sh
bash infra/tests/swap-provision-test.sh
bash infra/tests/release-version-test.sh
bash infra/tests/deploy-app-script-test.sh
bash infra/tests/qa-scripts-test.sh
bash .github/scripts/classify-release-test.sh
```

The root package exposes every QA deployment path. Commands that take a ref
require a clean checkout whose `HEAD` matches the pushed branch, tag, or commit:

```bash
npm run qa:install                         # public install on a fresh server
npm run qa:install -- <ref>                # candidate install on a fresh server
npm run qa:update -- <ref>                 # full infrastructure update
npm run qa:deploy-app -- <ref>             # app-only deploy from a pushed ref
npm run qa:deploy-local                    # app-only deploy of the working tree
npm run qa:test                            # QA wrapper contract tests
```

The npm deployment aliases default to `./.qa.env` in the repo root, matching
the `infra/qa/*.sh` scripts' own default of `.qa.env` in their worktree. Set
`QA_ENV_FILE=/path/to/.qa.env` to point at a shared QA configuration
elsewhere.

These commands are driven from a Linux, macOS, or Windows workstation. They
need `bash`, `git`, `ssh`, `scp`, `tar`, and `curl` on `PATH` — on Windows,
run them from Git Bash, which ships all of them. Hostname resolution for the
pre-flight check uses whichever of `getent`, `dscacheutil`, `dig`, `host`, or
`nslookup` the platform provides, and falls back to DNS-over-HTTPS through
`curl` when none is installed.

There is no CI that exercises the installer against a server. `infra/` changes reach a box only when its operator runs `sudo bash infra/update.sh` over SSH or applies a release tag from the in-app updater (both re-detect the box's hostname from the installed unit). Treat changes to `infra/` with extra care since they modify hosts in place.

## Making changes

- **Match the surrounding code.** The backend follows a strict layering: `transport → service → integration/store`. Don't reach across layers (e.g. no LXD calls from handlers). The frontend keeps state transitions in `frontend/src/state/` with unit tests pinning their behavior.
- **Write or update tests.** Backend packages have table-driven `_test.go` files next to the code. Frontend state changes should extend the corresponding `*.test.ts` file (and the `test` script in `frontend/package.json` if you add a new one). Note that CI does not currently run `go test` — run it locally before pushing.
- **Format and vet.** Run `gofmt` and `go vet ./...` on Go changes; `tsc -b` (via `npm run build`) must pass on frontend changes.
- **Update docs** when behavior changes — `README.md` for user-facing behavior, the relevant file under `docs/` for architecture changes.

## Commit messages

We use Conventional Commits with an area scope:

```text
type(scope): imperative summary
```

Types in use: `feat`, `fix`, `refactor`, `test`, `docs`. Scopes are area names like `containers`, `state`, `projects`, `files`, `workspaces`, `deploy`. Examples from history:

```text
fix(projects): serialize lifecycle transitions
refactor(state): encapsulate chat preference state
test(state): pin chat event projection behavior
```

Keep commits small and focused — one logical change per commit.

## Releases

Release tags use exactly `MAJOR.MINOR.PATCH`, without a `v` prefix. The version
determines what an installed server runs:

- Bump `PATCH` for frontend/backend-only releases.
- Bump `MINOR` or `MAJOR` when the release changes host dependencies, Caddy or
  systemd configuration, provider toolchains, workspace provisioning, or the
  reusable base image.

The tag workflow rejects patch releases that changed protected
infrastructure-managed paths since the previous release. This guard is a
minimum safety net, not a substitute for judgment: changes outside those paths
that require a newer host toolchain must also use a minor or major release.

The first release containing a change to the update machinery itself must be a
minor or major release so existing installations receive the new scripts via
full infrastructure convergence.

## Pull requests

1. Fork and create a topic branch from `main`.
2. Make your change with tests and docs.
3. Run the test suites listed above.
4. Open a PR describing **what** changed and **why**. Link any related issue.

## Reporting security issues

**Do not open a public issue for security vulnerabilities.** See [SECURITY.md](SECURITY.md) for the private reporting process.
