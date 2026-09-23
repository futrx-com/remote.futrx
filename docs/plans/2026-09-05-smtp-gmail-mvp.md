# SMTP email delivery (Gmail app-password MVP)

**Status:** in-progress
**Tier:** structural — it adds three new packages (`integration/smtp`, `service/email`, `stores/fileemail`), a new `DATA_DIR` file, and a new outbound network protocol; the tier check-in was held with the requester before this file was written (decisions recorded under *Design → Approach*).
**Scope:** Let a server administrator save one Gmail address plus its 16-character app password in Settings, verify those credentials against `smtp.gmail.com` before they are persisted, and send a test email that proves delivery works.
**Out of scope:**
- Wiring any existing feature to send email. Nothing in the product emails anyone after this change; the only send path is the explicit admin-triggered test message.
- Non-Gmail SMTP providers, configurable host/port/TLS mode, OAuth2/XOAUTH2 authentication.
- Per-user email preferences, opt-outs, templates, HTML bodies, attachments, queuing, or retry.
- Encryption at rest for the stored app password. It follows the repository's existing plaintext-0600 posture (see *Hard constraints*) and is recorded as a known risk, not fixed here.
- Anything under `infra/`. No installer, Caddy, systemd, or base-image change is required, so this stays a PATCH-class release.

---

## Orientation                              <!-- Phase 1 -->

**Languages and build.** Go backend (`backend/go.mod:3` — `go 1.25.13`) and a Preact + Vite SPA (`frontend/package.json`). Root `package.json` holds only QA deployment aliases; it is not a workspace root.

**Backend dependencies (from `backend/go.mod:5-12`).** Direct: `github.com/SherClockHolmes/webpush-go v1.4.0`, `github.com/creack/pty v1.1.24`, `github.com/gorilla/websocket v1.5.3`, `github.com/tus/tusd/v2 v2.9.2`, `golang.org/x/crypto v0.54.0`, `golang.org/x/oauth2 v0.36.0`. **This change adds no dependency** — `net/smtp`, `crypto/tls`, `net/mail`, and `mime` are all standard library. `backend/go.sum` is a lock artifact and must not be hand-edited; it needs no change here.

**Frontend versions (from `frontend/package.json`).** `preact ^10.23.0`, `zustand ^5.0.15`, `vite ^8.2.2`, `typescript ^5.5.4`, `tailwindcss ^3.4.10`. `frontend/package-lock.json` is generated — do not hand-edit; no dependency is added.

**Repo shape.** Single repository, four deployable/organisational units (`CONTRIBUTING.md`, "Repository layout"): `backend/` (Go), `frontend/` (SPA, built into `backend/public/` and embedded by `go:embed`), `infra/` (installer/updater/shell tests), `docs/`. Backend entry point `backend/cmd/remote/main.go`; composition roots are `backend/internal/service/services.go` (services), `backend/internal/stores/stores.go` (stores), `backend/internal/config/agents.go` (agent modules).

**Documentation locations.** `README.md`, `AGENTS.md` (QA workflows), `CONTRIBUTING.md`, `ARCHITECTURE.md`, `SECURITY.md`, and the `docs/` tree (`01-overview/`, `02-user-guide/`, `02-workspaces/`, `03-platform/`, `04-operations/`, `dev/`, `threat-model.md`, `known-limitations.md`). There is **no ADR directory** and **no `docs/plans/` directory** — this file creates it.

**Build / run / check commands** (`CONTRIBUTING.md`, "Development setup"):
- Backend: `cd backend && go build ./...`, `go test ./...`, `gofmt`, `go vet ./...`. Server runs with `go run ./cmd/remote`, listening on `7682` (`PORT` overrides).
- Frontend: `cd frontend && npm install`, `npm run dev` (Vite on `5174`, proxies `/api` and `/ws` to `127.0.0.1:7682`), `npm run build` (`tsc -b && vite build`, output to `backend/public/`), `npm test` (`node --experimental-strip-types --test`, auto-discovering `*.test.ts`).
- QA deploy: `npm run qa:deploy-app -- <ref>` from the repo root (`AGENTS.md`).

**Test setup.** Backend uses table-driven `_test.go` files beside the code (e.g. `backend/internal/service/push/service_test.go`, `backend/internal/transport/http/handlers/push_handler_test.go`). Frontend uses the Node built-in runner on `*.test.ts` beside the code (e.g. `frontend/src/state/hooks/auth/localAuthFormState.test.ts`). **CI does not run `go test`** (`CONTRIBUTING.md`, "Making changes") — every backend test in this plan must be run locally.

**Data layer and migrations.** There is no database and **no migration tooling** (`ARCHITECTURE.md:54`). Persistence is flat JSON/JSONL under `DATA_DIR` (`/opt/remote.futrx/data`) written by hand-rolled file stores using temp-file + rename with an in-process mutex (`ARCHITECTURE.md:243`, `backend/internal/stores/fileauth/store.go:319-343`). Adding state means adding a file and a store; there is no schema to migrate.

**Generated / vendored / never-hand-edit paths.** `backend/public/` (Vite build output, embedded via `backend/static.go`), `backend/go.sum`, `frontend/package-lock.json`, `frontend/tsconfig.tsbuildinfo`, `vendors/`, `backend/internal/agent/provisioning/versions.env` (the single version manifest, symlinked at `infra/versions.env`), and the `.dev-data*/` local runtime directories.

**Lint / format / types.** No golangci-lint config; the stated bar is `gofmt` + `go vet ./...`. Frontend type-checking is `tsc -b` via `npm run build`; `frontend/tsconfig.json` governs it. There is no ESLint/Prettier config.

---

## Hard constraints (this project)          <!-- Phase 2 -->

- **Strict downward layering.** "The backend follows a strict layering: `transport → service → integration/store`. Don't reach across layers (e.g. no LXD calls from handlers)." — source: `CONTRIBUTING.md`, "Making changes"; restated at `ARCHITECTURE.md:75-90`. No `net/smtp` call may appear in a handler or in `service/`.
- **Tests live beside the code and must be run locally.** "Backend packages have table-driven `_test.go` files next to the code… Note that CI does not currently run `go test` — run it locally before pushing." — source: `CONTRIBUTING.md`.
- **`gofmt` + `go vet ./...` on Go changes; `tsc -b` must pass on frontend changes.** — source: `CONTRIBUTING.md`, "Format and vet".
- **Conventional Commits with an area scope**, types `feat|fix|refactor|test|docs`. — source: `CONTRIBUTING.md`, "Commit messages".
- **Every commit is signed off (DCO).** "Sign off every commit… `git commit -s`." — source: `CONTRIBUTING.md`, "Licensing and sign-off".
- **Release classification.** "Bump `PATCH` for frontend/backend-only releases. Bump `MINOR` or `MAJOR` when the release changes host dependencies, Caddy or systemd configuration, provider toolchains, workspace provisioning, or the reusable base image." — source: `CONTRIBUTING.md`, "Releases". This change touches none of those, so it is PATCH-class and needs no `infra/` edit.
- **Docs are updated when behaviour changes.** "`README.md` for user-facing behavior, the relevant file under `docs/` for architecture changes." — source: `CONTRIBUTING.md`.
- **Never commit credentials; QA settings come from `.qa.env`.** — source: `AGENTS.md`. `.qa.env` is git-ignored; only `.qa.env.example` is tracked. No real Gmail address or app password may appear in any tracked file, test fixture, or this plan.
- **`DATA_DIR` secrets are plaintext at mode 0600 by existing design.** `session.key`, `oauth.json`, `local-admin.json`, and `projectsecrets/*.json` are "plaintext files. There is no encryption at rest and no key rotation mechanism." — source: `docs/threat-model.md:220` (finding 18). The new SMTP credential file conforms to this posture and extends finding 18 rather than inventing a one-off encryption scheme (see *Deviations* — none — and *Risks*).
- **Single process owns `DATA_DIR`.** "Neither path adds `fsync`, file locking, or a transaction spanning multiple stores. The design assumes exactly one backend process touching `DATA_DIR`." — source: `ARCHITECTURE.md:243`. An in-process mutex is the correct and sufficient guard for the new store.
- **The `/api/*` middleware gate is not authorization.** "Admin-only routes and project-membership routes re-check authorization per handler." — source: `ARCHITECTURE.md:111`. The new handler must perform its own admin check.

---

## Sources read                             <!-- Phase 2 -->

| Document | What it settled |
| --- | --- |
| `.claude/README.md` | `.claude/skills/` holds the repeatable recipes; `.claude/memory/` is described but **does not exist** in this checkout, so there is no prior project-memory to honour. |
| `.claude/skills/Planning/SKILL.md` | The pipeline this plan follows. Its bundled `references/design-review.md` is **absent** from the checkout (only `SKILL.md` is present), so Phase 5 was executed against the summaries in `SKILL.md` itself. |
| `AGENTS.md` | QA is script-driven (`infra/qa/install.sh`, `update.sh`, `deploy-app.sh`); `deploy-app.sh` is the right cycle for a frontend/backend-only change; credentials come from `.qa.env` and are never committed. |
| `CONTRIBUTING.md` | Layering rule, test placement and the "CI does not run `go test`" warning, gofmt/vet/tsc bar, Conventional Commits + DCO sign-off, PATCH-class release rule, docs-update rule, exact build/test commands. |
| `ARCHITECTURE.md` | Four-layer backend (`transport → service → integration/stores`), composition roots, the `DATA_DIR` persistence table this change extends (`:222-243`), the temp-file+rename convention, the "admin routes re-check authorization per handler" rule (`:111`), and the frontend's strict `config → models → transport → api → state → app → ui` order (`:290`). |
| `README.md` | User-facing setup narrative ("### 3. Create your first project", `README.md:148-163`) is where an admin-facing setup step belongs. |
| `SECURITY.md` | Vulnerabilities are reported privately; this change introduces no reporting-process change. |
| `docs/threat-model.md` | Finding 18 (`:220`) enumerates the plaintext-at-rest credential files by name; a new credential file must be added to that list. Finding 10 (`:211`) confirms the project's flat admin/member model — an admin-only surface is the correct gate for a server-wide credential. |
| `docs/03-platform/07-data-and-frontend-state.md` | Holds the `DATA_DIR` directory tree (`:18`) and its per-file table (`:151`), both of which enumerate `oauth.json` and must gain the new file. |
| `docs/03-platform/08-api-and-realtime.md` | Holds the endpoint table; `:35` documents `GET, PUT /api/admin/auth/google` "admin only" — the row format the new routes must match. |
| `docs/known-limitations.md` | Single-host, no clustering — confirms one in-process store with a mutex is architecturally sufficient. |
| `docs/plans/` | **Does not exist.** No prior plan and therefore no `## Outcome` section exists for this or any adjacent area; nothing to avoid repeating. |
| `docs/dev/agents/`, `docs/dev/push-notifications/` | The two existing subsystem dev guides. Neither covers email; this MVP does not add a third (see *Maintainability*). |

---

## Context                                  <!-- Phase 3 -->

### 1. Which module and layer, and its boundaries

Three layers are touched, in the repository's own downward order (`ARCHITECTURE.md:75-90`):

- **`backend/internal/integration/`** — "typed wrappers over host tools" and outbound protocols. It may import the standard library and third-party clients; nothing above it. Prior sibling: `backend/internal/integration/webpush/` (`client.go`, `transport.go`, `key.go`, `model.go`, plus `_test.go` beside each). This is where `net/smtp` belongs.
- **`backend/internal/service/`** — all policy. It defines the interfaces it consumes in its own `ports.go` and never imports `transport`. Prior sibling: `backend/internal/service/push/` (`ports.go:13-25` declares `Repository` and `Sender`; `service.go:11-24` composes them).
- **`backend/internal/stores/`** — file-backed persistence guarded by an in-process mutex. Prior sibling: `backend/internal/stores/fileauth/store.go`.

`backend/internal/transport/http/handlers/` sits above all three and may only call `service`.

### 2. Reference implementation

**`GET/PUT /api/admin/auth/google` — the Google OAuth client-credential settings feature.** It is the same shape as this change in every respect that matters: an admin-only settings surface that accepts a credential pair, validates it, persists it to one 0600 JSON file under `DATA_DIR`, and returns a response that deliberately omits the secret. Its parts:

- Handler: `backend/internal/transport/http/handlers/auth_google_config_handler.go:14-59`
- Service entry point: `backend/internal/service/auth/service.go:221-223` → `google_authenticator.go:103-111`
- Port: `backend/internal/service/auth/ports.go:5-8` (`OAuthConfigStore`)
- Model + typed errors: `backend/internal/service/auth/model.go:17`, `:26`, `:31-34`
- Store: `backend/internal/stores/fileauth/store.go:39-83` (read/save `oauth.json`) and `:319-343` (`writeJSONLocked`: `MkdirAll 0700` → `CreateTemp` → `Chmod 0600` → encode → `Rename`)
- Frontend route constant: `frontend/src/config/routes.ts:9`
- Frontend API client: `frontend/src/api/authApi.ts:50-58`
- Frontend controller hook: `frontend/src/state/hooks/auth/useGoogleOAuthSettingsController.ts`
- Frontend card: `frontend/src/ui/settings/GoogleOAuthSettings.tsx`
- Mounted in Settings: `frontend/src/ui/settings/SettingsPage.tsx:261` (`{isAdmin && <GoogleOAuthSettings />}`)

**Important caveat about this reference.** `googleConfigHandler` is *unexported*, has no `NewXHandler` constructor, is **not** a field on `httptransport.Handlers`, and is **not** constructed in `transport.go`. It is a member of `AuthHandler` (`backend/internal/transport/http/handlers/auth_handler.go:17`, constructed at `:28`, registered at `:38`) and reaches the mux through the `Handlers.Auth` slot. It is therefore the reference for the **credential-settings semantics** (admin gate, validate-then-write, secret never echoed) but **not** for HTTP wiring — email is not an auth flow and must not be bolted onto `AuthHandler`.

**Wiring reference:** `PushHandler` (`backend/internal/transport/http/handlers/push_handler.go:18-37`) is the model for a standalone feature handler: exported type, `NewPushHandler(...)` constructor, `RegisterRoutes`, its own `Handlers` field (`backend/internal/transport/http/server.go:31`), its own `register(...)` line (`server.go:66`), and construction in `transport.go:118-122`.

**Outbound-integration reference:** `backend/internal/service/push/` + `backend/internal/integration/webpush/`, which is how this repository models "a service that owns policy and delegates one network protocol to a `Sender` port" (`backend/internal/service/push/ports.go:19-25`). Note the shape of the seam precisely: the integration client's own signature does **not** match the service port. A small adapter type in the `service` package bridges the two and translates errors — `webPushSender` at `backend/internal/service/web_push_sender.go:15-44`, composed at `backend/internal/service/services.go:333`. This change needs the equivalent adapter (step 5), and omitting it is the most likely way the composition fails to compile.

### 3. Naming, file layout, and export conventions in that area

- Backend package directories are lowercase single words; a service package is `service/<noun>/` with `model.go`, `ports.go`, `service.go` (`backend/internal/service/push/`, `backend/internal/service/chat/`).
- File stores are `stores/file<noun>/store.go` with `func New(dataDir string)` and are aggregated in `backend/internal/stores/stores.go:48-63` (the `Stores` struct) and `:65-136` (`New`).
- Service packages are imported at composition with a `service` prefix alias: `servicepush "…/internal/service/push"` (`backend/internal/service/services.go:20`, `backend/internal/stores/stores.go:11`).
- Sentinel errors are package-level `Err…` values in `model.go` (`backend/internal/service/auth/model.go:17-29`).
- Handlers are `<noun>_handler.go` in `package httphandlers`, expose `RegisterRoutes(*http.ServeMux)`, and are constructed by `NewXHandler(...)` (`backend/internal/transport/http/handlers/push_handler.go:24-37`).
- Frontend layering is strictly `config → models → transport → api → state → app → ui` (`ARCHITECTURE.md:290`). Every URL is a constant in `frontend/src/config/routes.ts`; every response shape is a type in `frontend/src/models/`; every call goes through `requestJson` in `frontend/src/api/apiRequest.ts`.

### 4. Errors, validation, logging, authorization, configuration, transactions

- **Errors:** services return sentinel errors (`backend/internal/service/auth/model.go:17-29`); handlers translate to a JSON `{"error": "..."}` body via `httptransport.SendErr` (`backend/internal/transport/http/response.go:14-16`). The Google handler passes the service error text straight through with `400` (`auth_google_config_handler.go:51-54`).
- **Validation:** happens twice, deliberately. The service/authenticator validates policy (`google_authenticator.go:107-109` returns `ErrInvalidOAuthConfig`), and the store re-trims and re-checks before writing (`fileauth/store.go:75-79`). The frontend also pre-validates for fast feedback (`useGoogleOAuthSettingsController.ts`, the `!clientId.trim() || !clientSecret.trim()` guard).
- **Logging:** the composition root logs degradation and continues rather than failing startup (`backend/internal/service/services.go:306-333`, `newPush` returns a disabled service on every error path). Credentials are never logged.
- **Authorization:** every admin route re-checks in the handler — `callerEmailFromRequest` then `h.auth.IsAdmin` (`auth_google_config_handler.go:19-27`; helper at `backend/internal/transport/http/handlers/users_handler.go:170`). The `/api/*` middleware only establishes a registered session (`ARCHITECTURE.md:111`).
- **Configuration:** application-wide policy lives in `backend/internal/config/config.go` as typed option structs mirrored into `service.Dependencies` (`backend/internal/config/config.go:11-19`, `backend/internal/service/services.go:56-79`). Per-deployment *credentials* are not config — they live in `DATA_DIR` and are entered through the UI, exactly as `oauth.json` is.
- **Transactions:** none exist. Atomicity per file is temp-file + rename under one mutex (`fileauth/store.go:319-343`).

### 5. End-to-end flow of the reference implementation

`PUT /api/admin/auth/google`
→ `googleConfigHandler.serve` resolves the caller and rejects non-admins (`auth_google_config_handler.go:19-27`)
→ decodes `{clientId, clientSecret}` with `readJSONBody` (`agent_auth_handler.go:273`)
→ `auth.ConfigureGoogleOAuth` (`service/auth/service.go:221`)
→ `googleAuthenticator.configure` validates, calls `store.SaveOAuthConfig`, then swaps the live client (`google_authenticator.go:103-111`)
→ `fileauth.Store.SaveOAuthConfig` trims, re-validates, and writes `oauth.json` at 0600 via temp-file + rename (`fileauth/store.go:69-83`)
→ handler responds `200` with `{configured, clientId, redirectUrl}` — **the secret is never echoed back** (`auth_google_config_handler.go:29-35`).

### Flow today for email

**None.** `grep -rnE "net/smtp|SMTP" backend/internal backend/cmd` returns no sender, and `grep -rn "mail\." backend/internal` matches only `net/mail` address parsing at `backend/internal/integration/webpush/subscriber.go:17`. The product's one outbound notification channel today is Web Push — the "**Notifications.**" paragraph at `ARCHITECTURE.md:290` describes it — and this change does not touch it.

### Wiring points this change must extend

| Point | Anchor |
| --- | --- |
| Store aggregate struct, constructor body, **and its return literal** | `backend/internal/stores/stores.go:48-63`, `:65-120`, `:122-136` |
| Service dependency struct | `backend/internal/service/services.go:56-79` |
| Service aggregate struct | `backend/internal/service/services.go:105-128` |
| Service aggregate return literal | `backend/internal/service/services.go:288-303` |
| Integration→service port adapter | `backend/internal/service/web_push_sender.go:15-44` (the pattern to copy) |
| **`service.Dependencies` literal in `main.go`** — one field per store, e.g. `Push: storeSet.Push` | `backend/cmd/remote/main.go:82-101` (`stores.New` is at `:69`) |
| Transport handler slot + registration | `backend/internal/transport/http/server.go:22-47`, `:58-73` |
| Handler construction | `backend/internal/transport/transport.go:97-138` |
| Frontend URL constants | `frontend/src/config/routes.ts:7-131` |
| Settings tab union + nav + panel | `frontend/src/ui/settings/SettingsPage.tsx:31-39`, `:41-95`, `:217-306` |
| Icon set (**no `Mail` icon exists**) | `frontend/src/ui/primitives/icons.tsx:8-67` |

---

## Architecture conformance                 <!-- Phase 4 -->

| Convention observed | Evidence (`path:line`) | How this change follows it |
| --- | --- | --- |
| Outbound protocols live in `integration/`, never in `service/` or `transport/` | `ARCHITECTURE.md:82-89`; `backend/internal/integration/webpush/client.go` | `net/smtp` and `crypto/tls` are imported only by `backend/internal/integration/smtp/`. |
| A service declares the narrow ports it consumes in its own `ports.go`, and a small adapter in `package service` bridges the integration client to that port | `backend/internal/service/push/ports.go:13-25` + `backend/internal/service/web_push_sender.go:15-44` | `service/email/ports.go` declares `Store` and `Sender`; `backend/internal/service/email_sender.go` adapts `integration/smtp` to `Sender`, filling the `From` header and mapping causes onto the service's sentinels. |
| A store returns `(nil, nil)` when the record was never written, because absence is a valid state rather than an error | `backend/internal/service/auth/ports.go:46-52` (`TwoFactorStore.Get`) | `fileemail.Credentials` returns `(nil, nil)` when `smtp.json` does not exist. (`fileauth.OAuthConfig` instead returns a sentinel; the `TwoFactorStore` convention is the one followed, and the difference is noted in step 4.) |
| Credential settings persist to one 0600 JSON file under `DATA_DIR` via temp-file + rename under a mutex | `backend/internal/stores/fileauth/store.go:69-83`, `:319-343`; `ARCHITECTURE.md:243` | `stores/fileemail` writes `DATA_DIR/smtp.json` with a `writeJSONLocked` copied in shape from `fileauth`. |
| Sentinel `Err…` values in `model.go`, translated to `{"error"}` JSON at the edge | `backend/internal/service/auth/model.go:17-29`; `backend/internal/transport/http/response.go:14-16` | `service/email/model.go` defines `ErrNotConfigured`, `ErrInvalidAddress`, `ErrInvalidAppPassword`, `ErrVerificationFailed`; the handler maps them to 400/409/502. |
| Admin routes re-check `IsAdmin` inside the handler | `ARCHITECTURE.md:111`; `backend/internal/transport/http/handlers/auth_google_config_handler.go:19-27` | `emailSettingsHandler.serve` repeats that exact preamble before any branch. |
| Secrets are never echoed in a response | `backend/internal/transport/http/handlers/auth_google_config_handler.go:29-35` | The settings response is `{configured, address}` only; the app password has no read path at any layer above the store. |
| A standalone feature handler is `<noun>_handler.go` with an exported type + `NewXHandler`, constructed in `transport.go`, slotted in `Handlers` | `backend/internal/transport/http/handlers/push_handler.go:18-37`; `backend/internal/transport/http/server.go:31`, `:66`; `backend/internal/transport/transport.go:118-122` | `email_settings_handler.go` adds `EmailSettings RouteRegistrar` to `Handlers`, one `register(...)` line, and one construction site. It is **not** attached to `AuthHandler` — email is not an auth flow. |
| Store is created in `stores.New`, surfaced on the `Stores` struct, **and returned in the literal**, then passed through the `service.Dependencies` literal in `main.go` | `backend/internal/stores/stores.go:48-63`, `:121-136`; `backend/cmd/remote/main.go:82-101` | `Email` field + `fileemail.New(dataDir)` + `Email: email` in the return + `Email: storeSet.Email` in `main.go`, following the `Push` chain end to end. |
| A degraded external dependency disables the feature instead of failing startup | `backend/internal/service/services.go:306-333` (`newPush`) | `email.New(store, sender)` is total; `Configured()` is false until an admin saves credentials, and every entry point returns `ErrNotConfigured`. |
| Frontend order `config → models → transport → api → state → ui` | `ARCHITECTURE.md:290` | New files are added in exactly that order and import only downward. |
| URLs are constants in `config/routes.ts`; calls go through `requestJson` | `frontend/src/config/routes.ts:9`; `frontend/src/api/apiRequest.ts:5-24` | `API_ROUTES.email.{settings,test}` + `frontend/src/api/emailApi.ts`. |
| Pure form logic is extracted beside the hook and unit-tested | `frontend/src/state/hooks/auth/localAuthFormState.ts` + `localAuthFormState.test.ts` | `emailSettingsForm.ts` + `emailSettingsForm.test.ts` hold and pin the address/app-password normalisation. |
| Table-driven `_test.go` beside the code | `backend/internal/service/push/service_test.go` | Every new backend package ships its test file in the same commit. |

**Deviations:** None.

---

## Design                                   <!-- Phase 5 -->

> **Note:** the skill's bundled `references/design-review.md` is not present in this checkout (`.claude/skills/Planning/` contains only `SKILL.md`), so this section was written against the rubric summarised in `SKILL.md` itself.

### Forces

- **F1 — Credentials must be proven before they are stored.** Stated in the request: "you must verify it before save". A saved-but-wrong password would fail silently at the first real send, long after the admin left the screen. This forces a `Verify` operation that performs a genuine SMTP login, and forces validation to sit *before* the store write.
- **F2 — A real network round-trip must not make the code untestable.** `CONTRIBUTING.md` requires a test beside the code, and no test may reach `smtp.gmail.com`. This forces the SMTP conversation behind a port the service can fake, and forces the dialer inside the integration package to be substitutable for its own tests.
- **F3 — The app password must never leave the host.** It is a Google account credential with mail-send authority. This forces a one-way data flow: writable through the API, readable only by the store and the SMTP client, absent from every response body, log line, and error string.
- **F4 — Gmail's app password is presented to humans as four space-separated groups.** Google's UI shows `abcd efgh ijkl mnop`; an admin will paste it that way. This forces whitespace-stripping normalisation before the 16-character length rule and before authentication.
- **F5 — The remote system fails in several distinguishable ways.** Wrong password, unreachable network, TLS failure, and Gmail rejecting the account (2FA not enabled on the Google account, so no app password is valid) are different problems with different fixes. This forces a verification result that carries a usable message to the admin rather than a bare boolean.
- **F6 — "Verified credentials" and "a message actually arrived" are different claims.** A successful `AUTH` proves login, not delivery. The requester chose to include a test send, so this forces a second, separate operation on top of verification.
- **F7 — Server-wide credential, flat admin/member model.** One SMTP identity serves the whole box, and `docs/threat-model.md:211` establishes that members are not trusted with server-wide secrets. This forces admin-only authorization on every route, re-checked in the handler.
- **F8 — There is no migration tooling and no database.** `ARCHITECTURE.md:54`, `:243`. This forces a new standalone `DATA_DIR` file whose absence is the valid "not configured" state, and forbids any scheme that would need a backfill.
- **F9 — The blocking network call sits on an HTTP request.** A hung TCP connect to Gmail would otherwise pin a handler goroutine indefinitely. This forces an explicit timeout and context propagation on both `Verify` and `Send`.

### Approach

A new `service/email` package owns the policy — normalise, validate, verify, persist, send — and delegates the protocol to a `Sender` port. That port is satisfied not by the SMTP client directly but by a thin `emailSender` adapter in `package service`, exactly as `webPushSender` adapts the Web Push client (`backend/internal/service/web_push_sender.go:15-44`). Below the adapter, a new `integration/smtp` package built on `net/smtp` + `crypto/tls` speaks the protocol. A new `stores/fileemail` persists one credential record to `DATA_DIR/smtp.json` at mode 0600 using the same temp-file+rename+mutex shape as `fileauth`. A new admin-only handler exposes `GET/PUT/DELETE /api/admin/email` and `POST /api/admin/email/test`. The frontend adds an **Email** tab to Settings whose card mirrors `GoogleOAuthSettings`.

**Two type families, deliberately not shared.** `service/email` owns `Credentials{Address, AppPassword}` and `Message{To, Subject, Body}` — no `From`, because the sender identity is not the caller's to choose. `integration/smtp` owns `Account{Address, AppPassword}` and `Message{From, To, Subject, Body}` — the wire representation. The adapter is the single place that maps one to the other and sets `From` from `Credentials.Address`. This keeps the layering rule intact: `integration/smtp` imports nothing from `service/`.

`Configure` is the load-bearing sequence and answers F1 directly: normalise → validate → `sender.Verify` against `smtp.gmail.com:587` → **only then** `store.Save`. A failed verification leaves any previously stored credential untouched, so a bad edit cannot break a working configuration.

Four decisions were confirmed with the requester at the structural check-in:

1. **MVP scope = configure + verify + test send.** No existing feature begins sending email (F6).
2. **A new "Email" tab in Settings**, rather than a card inside Notifications.
3. **App-password validation = strip all whitespace, then require exactly 16 characters** (F4). The alphabet is not constrained, so a future change to Google's character set cannot lock an admin out.
4. **Host and port are fixed constants** — `smtp.gmail.com:587` with STARTTLS. Not configurable, not environment-driven, not stored. "First edition we will use SMTP from Gmail" is the whole requirement, and a generic-SMTP form would be structure with no force behind it.

The sender address is not constrained to a `gmail.com` domain: Google Workspace accounts authenticate to the same host with a custom-domain address, and rejecting them would be a false failure. `net/mail.ParseAddress` is the only address rule.

### Patterns

| Concern | Pattern | Force | Prior art | Rejected alternative — reason |
| --- | --- | --- | --- | --- |
| Keep the SMTP protocol out of policy code | Port/adapter — `email.Sender` interface declared by the consumer, implemented by `integration/smtp` | F2, and the layering constraint | `backend/internal/service/push/ports.go:19-25` implemented by `backend/internal/integration/webpush` and adapted at `services.go:333` | Calling `net/smtp` from `service/email` — violates `CONTRIBUTING.md`'s layering rule and makes every service test open a socket. |
| Persist one server-wide credential | File-backed store with in-process mutex and temp-file+rename, absence = not configured | F8 | `backend/internal/stores/fileauth/store.go:39-83`, `:319-343` | Adding a field to `oauth.json` — that file is `auth`'s, guarded by `auth`'s mutex, and validated as an OAuth pair at `fileauth/store.go:63-65`; SMTP state would break its invariant. |
| Prove credentials before persisting | Verify-then-write ordering inside one service method | F1 | `google_authenticator.configure` validates before `store.SaveOAuthConfig` (`google_authenticator.go:103-111`) — this change extends the same ordering from local validation to a live remote check | Save-then-verify with a stored `status` field — leaves an unusable credential on disk and adds state that can go stale. |
| Substitute the network in the integration's own tests | Unexported dialer field on the client struct, defaulted in `New`, replaced by the package's own tests | F2 | `publicDialer{lookup, dial}` at `backend/internal/integration/webpush/transport.go:22-26`, substituted with fakes at `transport_test.go:20-22` and `:41-48` | A build tag or an env var to skip the test — hides the code path that matters most instead of exercising it. |
| Bridge the integration client's signature to the service port | Adapter type in `package service`, constructed at composition | F2, and the layering constraint (the service must not import `integration`) | `webPushSender` at `backend/internal/service/web_push_sender.go:15-44`, wired at `services.go:333` | Making `integration/smtp` implement `email.Sender` directly — the integration package would have to import `service/email` for its `Credentials` and `Message` types, inverting the dependency the layering rule fixes. |
| Report *why* verification failed | Typed sentinel errors wrapping the underlying cause, translated once at the edge | F5 | `backend/internal/service/auth/model.go:17-29` + `auth_google_config_handler.go:51-54` | Returning `bool` — the admin cannot tell "wrong password" from "network blocked", which are the two most likely failures. |
| Bound the outbound call | `context.Context` on both port methods + a package-level dial/command timeout constant | F9 | `AgentOptions.CapabilityTimeout` bounds a provider probe (`backend/internal/config/config.go:24-27`) | A `config.Config` field — this is a fixed protocol timeout, not deployment policy; adding an env var invents configuration nobody asked for. |
| Represent the credential in the service | Two types: `Credentials{Address, AppPassword}` inbound-only, `Settings{Configured, Address}` outbound-only | F3 | `OAuthConfig` (in) versus the handler's `{configured, clientId, redirectUrl}` map (out) — `auth_google_config_handler.go:29-35` | One struct with a `json:"-"` password field — one forgotten tag or one `%+v` in a log leaks it; separate types make the leak impossible to write. |
| Frontend admin settings surface | Self-contained controller hook + presentational card, mounted behind `isAdmin` | F7 | `useGoogleOAuthSettingsController.ts` + `GoogleOAuthSettings.tsx`, mounted at `SettingsPage.tsx:261` | Threading email state through `SettingsContainer` props — `SettingsPage`'s prop list is already 30+ entries; the Google card's self-contained hook is the established escape from that. |
| Test the input rules without a browser | Pure normalisation module beside the hook | F4 | `frontend/src/state/hooks/auth/localAuthFormState.ts` + its `.test.ts` | Inlining the rules in the hook — `npm test` cannot reach them, and the space-stripping rule is the single most likely thing to regress. |

### SOLID review

- **S:** `integration/smtp.Client` knows only how to hold an SMTP conversation with Gmail; `service/email.Service` knows only the order of operations (normalise → validate → verify → persist); `stores/fileemail.Store` knows only the on-disk representation. The one change that would touch all three — "support a second provider" — is exactly the change declared out of scope.
- **O:** Adding a second recipient or a different message body extends `email.Message` and its builder without editing `Service.Configure` or the store. Adding a non-Gmail provider means a second `Sender` implementation, not a modification of `Service`.
- **L:** `Sender` has two methods (`Verify`, `Send`) and no hidden contract beyond "return a non-nil error when the operation failed". The test fake in `service/email/service_test.go` and the real `smtp.Client` are interchangeable at every call site; no method requires a `*smtp.Client` concrete type.
- **I:** `service/email` declares `Store` (three methods: `Credentials`, `Save`, `Delete`) and `Sender` (two methods) separately, so the file store is never handed the sender's methods and the SMTP client never sees persistence. This mirrors `push`'s split of `Repository` from `Sender` (`push/ports.go:13-25`) rather than one wide `EmailBackend` interface.
- **D:** The composition root (`services.go`) is the only place that knows `fileemail` and `integration/smtp` exist; `service/email` imports neither. The handler depends on `*email.Service`, matching how `PushHandler` depends on `*servicepush.Service` (`push_handler.go:18-22`).

### Extension points

| Extension point | Mechanism | Variation anticipated | Why plausible now |
| --- | --- | --- | --- |
| `email.Sender` | Interface declared in `service/email/ports.go`, implementation chosen in `services.go` | A fake sender that records calls instead of dialing | Required by this change's own tests — `service_test.go` and `email_settings_handler_test.go` cannot run without it, and CI does not run `go test`, so a socket-free test is the only test that gets run at all. |
| `smtp.Client` dialer + host fields | Unexported `dial func(ctx, network, addr) (net.Conn, error)` and `addr`/`host` fields, defaulted in `New`, replaced by the package's own tests | An in-process fake SMTP server bound to `127.0.0.1` | Required by `integration/smtp/client_test.go` in step 2; without it the STARTTLS+AUTH sequence has no test at all. Same seam and same reason as `publicDialer` in `webpush/transport.go:22-26`. |

No other extension point is introduced. In particular there is **no** provider registry, **no** message-template abstraction, **no** queue, and **no** configurable transport — none of them answers a force in 5a.

### Maintainability

- **Placement.** Every new file sits beside an existing sibling of the same kind: `integration/smtp/` beside `integration/webpush/`, `service/email/` beside `service/push/`, `stores/fileemail/` beside `stores/fileauth/`, `email_settings_handler.go` beside `push_handler.go`, `EmailSettings.tsx` beside `GoogleOAuthSettings.tsx`.
- **Reuse.** Address parsing uses `net/mail`, already used at `backend/internal/integration/webpush/subscriber.go:17`. The handler reuses `callerEmailFromRequest` (`users_handler.go:170`), `readJSONBody` (`agent_auth_handler.go:273`), `httptransport.SendJSON` / `SendErr` (`response.go:8-16`). The frontend reuses `requestJson` (`apiRequest.ts`) and the existing Tailwind card classes verbatim from `GoogleOAuthSettings.tsx`.
- **Dependencies.** None added. `net/smtp`, `crypto/tls`, `net/mail`, `mime`, `time` are standard library; `backend/go.mod` and `go.sum` are unchanged. (`net/smtp` is frozen upstream but fully supported, and STARTTLS+PLAIN over TLS is exactly what it does well.)
- **Dead code.** None. This adds a capability; nothing is superseded, and no existing file loses its last caller.
- **Naming.** `Configure`/`Settings`/`Disable` on the service echo `ConfigureGoogleOAuth`/`GoogleOAuthEnabled` (`service/auth/service.go:221-231`); `fileemail.New(dataDir)` echoes `fileauth.New(dataDir)`; `useEmailSettingsController` echoes `useGoogleOAuthSettingsController`. A reader who knows the Google OAuth feature can read this one without stopping.
- **New subsystem doc.** Not added. `docs/dev/` holds guides for genuinely extensible subsystems (agents, push); a fixed single-provider MVP does not warrant a third. The endpoint table, the data table, and the threat model are updated instead (steps 8–9).

---

## Steps                                    <!-- Phase 6 -->

Run every command from the repository root unless the step says otherwise. After each backend step run `gofmt -l backend` (expect empty output) and `cd backend && go vet ./...`, per `CONTRIBUTING.md`. Steps 1–5 leave the repository building with the feature wired but unreachable; step 6 is the first that changes observable behaviour.

1. **SMTP integration: message building** — files: `backend/internal/integration/smtp/message.go`, `backend/internal/integration/smtp/message_test.go` (new).
   Change: package `smtp`. Define `Message{From, To, Subject, Body string}` and `func buildRFC5322(Message) ([]byte, error)`, emitting `From`, `To`, `Subject` (RFC 2047 encoded via `mime.QEncoding.Encode("utf-8", subject)`), `Date` (`time.Now().Format(time.RFC1123Z)`), `MIME-Version: 1.0`, `Content-Type: text/plain; charset="utf-8"`, CRLF line endings, and a blank-line header/body separator. **No `Message-ID` header is generated** — Gmail assigns one on submission, and inventing a right-hand side here would be a guess with no correct answer. Reject any header value containing CR or LF, and any empty `From`/`To` (header-injection and malformed-envelope guard).
   Verify: `cd backend && go test ./internal/integration/smtp/ -run TestBuild` — table-driven cases cover an ASCII subject (asserting the raw subject appears unencoded), a non-ASCII subject (asserting a `=?utf-8?` encoded-word appears and the raw runes do not), a `To` containing `\r\n` (asserting a non-nil error), an empty `From` (non-nil error), and the presence of the `\r\n\r\n` separator before the body.

2. **SMTP integration: client** — files: `backend/internal/integration/smtp/client.go`, `backend/internal/integration/smtp/client_test.go` (new).
   Change: same package `smtp`; import the standard library as `netsmtp "net/smtp"` — the package's own name would otherwise shadow it. Declare `const (GmailHost = "smtp.gmail.com"; GmailPort = "587"; dialTimeout = 15 * time.Second)` and `type Account struct{ Address, AppPassword string }`.
   `type Client struct { host string; addr string; dial func(ctx context.Context, network, addr string) (net.Conn, error) }`; `func New() *Client` sets `host: GmailHost`, `addr: net.JoinHostPort(GmailHost, GmailPort)`, and `dial: (&net.Dialer{Timeout: dialTimeout}).DialContext`.
   Add unexported `func newTestClient(host, addr string, dial func(context.Context, string, string) (net.Conn, error), skipTLS bool) *Client` plus an unexported `skipTLS bool` field, used **only** by `client_test.go`, so the fake server does not need a certificate.
   `Verify(ctx context.Context, account Account) error`: dial → `netsmtp.NewClient(conn, c.host)` → `Hello("localhost")` → `StartTLS(&tls.Config{ServerName: c.host})` unless `skipTLS` → `Auth(netsmtp.PlainAuth("", account.Address, account.AppPassword, c.host))` → `Quit`.
   `Send(ctx context.Context, account Account, msg Message) error`: the same prefix, then `buildRFC5322(msg)` → `Mail(msg.From)` → `Rcpt(msg.To)` → `Data` → write → `Close` → `Quit`.
   Both apply `conn.SetDeadline` from the context deadline (falling back to `dialTimeout`) and `defer conn.Close()`. **Neither may ever place `account.AppPassword` in a returned error** — wrap only the server's reply text.
   Verify: `cd backend && go test ./internal/integration/smtp/` — a fake SMTP server on a `127.0.0.1` listener drives four cases: a successful `AUTH` (decoding the base64 PLAIN payload and asserting it carries exactly the supplied address and password), a `535` rejection (asserting the error text mentions the server reply and `strings.Contains(err.Error(), password)` is **false**), a listener that accepts then closes without a greeting (asserting a non-nil error and a return within the deadline rather than a hang), and a successful `Send` (asserting the server received `MAIL FROM` with the account address and the body bytes).

3. **Email service** — files: `backend/internal/service/email/model.go`, `ports.go`, `credentials.go`, `service.go`, `credentials_test.go`, `service_test.go` (new).
   Change: `model.go` defines `Credentials{Address, AppPassword string}`, `Settings{Configured bool; Address string}`, `Message{To, Subject, Body string}` (**no `From`** — the sender identity comes from the stored credentials, not the caller), `AppPasswordLength = 16`, and sentinels `ErrNotConfigured`, `ErrInvalidAddress`, `ErrInvalidAppPassword`, `ErrInvalidRecipient`, `ErrVerificationFailed`, `ErrSendFailed`.
   `ports.go` declares `Store{Credentials(ctx) (*Credentials, error); Save(ctx, Credentials) error; Delete(ctx) error}` — documented, like `TwoFactorStore.Get` (`service/auth/ports.go:46-52`), as returning `(nil, nil)` when nothing was ever saved — and `Sender{Verify(ctx, Credentials) error; Send(ctx, Credentials, Message) error}`.
   `credentials.go` holds `normalizeAddress(string) (string, error)` (trim, lowercase, `mail.ParseAddress`, then require `parsed.Address == addr` — which **rejects `Display Name <a@b.com>` by design**, since the stored value is an envelope address, not a header) and `normalize(Credentials) (Credentials, error)` (normalise the address; strip every `unicode.IsSpace` rune from the password; require exactly `AppPasswordLength` runes, else `ErrInvalidAppPassword`).
   `service.go` holds `New(Store, Sender) *Service` and:
   - `func (s *Service) configured() bool` — `s != nil && s.store != nil && s.sender != nil`, mirroring `push.Service.Enabled` (`service/push/service.go:28-31`). **Every exported method returns `ErrNotConfigured` when it is false**, so a nil store (the state before step 5 lands) cannot panic.
   - `Settings(ctx) (Settings, error)` — returns `Settings{Configured: false}` with a **nil error** when the store holds nothing; only a real I/O failure is an error.
   - `Configure(ctx, Credentials) (Settings, error)` — normalise → `sender.Verify` → on failure return `fmt.Errorf("%w: %v", ErrVerificationFailed, cause)` **without touching the store** → `store.Save` → return `Settings{true, address}`.
   - `SendTest(ctx, recipient string) error` — `normalizeAddress(recipient)` or `ErrInvalidRecipient`; load credentials or `ErrNotConfigured`; build a fixed subject/body; `sender.Send`; wrap a failure as `fmt.Errorf("%w: %v", ErrSendFailed, cause)`.
   - `Disable(ctx) error` — `store.Delete`, idempotent.
   Verify: `cd backend && go test ./internal/service/email/` — `credentials_test.go` is table-driven over spaced/tabbed/15-rune/17-rune/empty passwords and over valid, invalid, and `Display Name <a@b.com>` addresses (the last **expected to fail** with `ErrInvalidAddress`). `service_test.go` uses a fake store and fake sender to pin: (a) a failing `Verify` returns an error satisfying `errors.Is(err, ErrVerificationFailed)`, carries the cause's text, and records **zero** `Save` calls; (b) a successful `Configure` saves the lowercased address and the whitespace-stripped password; (c) `SendTest` with an empty store returns `ErrNotConfigured`; (d) `SendTest` passes `Message.To` through and the fake sender receives the stored credentials; (e) `Settings` on an empty store returns `{Configured: false}` and a nil error; (f) a `Service` built with a nil store returns `ErrNotConfigured` from every method instead of panicking.

4. **File store** — files: `backend/internal/stores/fileemail/store.go`, `store_test.go` (new); `backend/internal/stores/stores.go` (edit).
   Change: `fileemail.New(dataDir) *Store` with a `sync.Mutex`, reading/writing `DATA_DIR/smtp.json` (`{"address": "...", "appPassword": "..."}`) through a `writeJSONLocked` copied in shape from `backend/internal/stores/fileauth/store.go:319-343` (`MkdirAll 0700` → `CreateTemp` → `Chmod 0600` → encode → `Rename`), and a leading `ctx.Err()` select in each method as every method in `fileauth` does. `Credentials` returns `(nil, nil)` on `os.ErrNotExist` — **this deliberately differs from `fileauth.oauthConfigLocked`, which returns a sentinel**; the `(nil, nil)` convention is the one `serviceauth.TwoFactorStore` documents (`service/auth/ports.go:46-52`) and the one `service/email`'s port declares. `Delete` treats `os.ErrNotExist` as success. Return type satisfies `serviceemail.Store` (add a `var _ serviceemail.Store = (*Store)(nil)` assertion).
   In `stores.go`: add `Email serviceemail.Store` to the `Stores` struct (`:48-63`), add `email := fileemail.New(dataDir)` in `New` (`:65-120`), **and add `Email: email,` to the returned `Stores{...}` literal (`:122-136`)** — omitting the last is an unused-variable compile error.
   Verify: `cd backend && go test ./internal/stores/... && go build ./...` — asserts a round-trip through `t.TempDir()`, `(nil, nil)` before any write, `Delete` idempotence on a missing file, and `os.Stat(filepath.Join(dir, "smtp.json")).Mode().Perm() == 0o600`.

5. **Compose the service and adapt the sender** — files: `backend/internal/service/email_sender.go` (new); `backend/internal/service/services.go`, `backend/cmd/remote/main.go` (edit).
   Change: `email_sender.go` defines, in `package service`, `type emailSender struct{ client *smtp.Client }` with `Verify(ctx, serviceemail.Credentials) error` and `Send(ctx, serviceemail.Credentials, serviceemail.Message) error`. Each converts `Credentials` → `smtp.Account` and, for `Send`, builds `smtp.Message{From: credentials.Address, To: msg.To, Subject: msg.Subject, Body: msg.Body}`. This file is the **only** place that maps between the two type families, and it mirrors `web_push_sender.go:15-44` line for line in structure.
   In `services.go`: add `Email serviceemail.Store` to `Dependencies` (`:56-79`), `Email *serviceemail.Service` to `Services` (`:105-128`), and `Email: serviceemail.New(deps.Email, emailSender{client: smtp.New()}),` to the returned `Services{...}` literal beside `Push:` (`:288-303`). Import `serviceemail "…/internal/service/email"` and `"…/internal/integration/smtp"`, matching the `servicepush`/`webpush` alias convention (`services.go:11`, `:20`).
   In `main.go`: add `Email: storeSet.Email,` to the `service.Dependencies` literal (`:82-101`), beside `Push: storeSet.Push`. **Without this line the feature is wired to a nil store and is dead at runtime.**
   Verify: `cd backend && go build ./... && go vet ./... && go test ./internal/service/...` — the build is what proves the adapter satisfies `serviceemail.Sender` and that all three literals were updated; the tests confirm nothing existing regressed. Then `grep -n "Email:" cmd/remote/main.go internal/service/services.go internal/stores/stores.go` must return one line in each of the three files.

6. **HTTP handler** — files: `backend/internal/transport/http/handlers/email_settings_handler.go`, `email_settings_handler_test.go` (new); `backend/internal/transport/http/server.go`, `backend/internal/transport/transport.go` (edit).
   Change: exported `EmailSettingsHandler` with `NewEmailSettingsHandler(email *serviceemail.Service, auth *serviceauth.Service) *EmailSettingsHandler` and `RegisterRoutes(mux)` binding `/api/admin/email` and `/api/admin/email/test` — the `PushHandler` shape (`push_handler.go:18-37`), **not** the `AuthHandler` embedding used by the Google config handler. Every request repeats the `callerEmailFromRequest` + `IsAdmin` preamble from `auth_google_config_handler.go:19-27` (401 then 403).
   Status mapping, using `errors.Is` against the service sentinels and `err.Error()` as the message so the wrapped cause reaches the admin:
   - `GET /api/admin/email` → `200 {"configured": bool, "address": string}`.
   - `PUT /api/admin/email {address, appPassword}` → `200` with that same body; `400` on `ErrInvalidAddress` or `ErrInvalidAppPassword`; `502` on `ErrVerificationFailed`; `409` on `ErrNotConfigured`.
   - `DELETE /api/admin/email` → `204`, **including when nothing was configured** (idempotent, so a double-click is not an error).
   - `POST /api/admin/email/test {to}` → `200 {"sent": true}`; `400` on `ErrInvalidRecipient`; `409` on `ErrNotConfigured`; `502` on `ErrSendFailed`.
   - Any other method on either route → `405`, matching `auth_google_config_handler.go:56-57`.
   Add `EmailSettings RouteRegistrar` to `Handlers` (`server.go:22-47`), one `register(handlers.EmailSettings)` line (`server.go:58-73`), and construction in `transport.go` beside `Push:` (`transport.go:118-122`).
   Verify: `cd backend && go test ./internal/transport/...` — new tests build a handler over a real `email.Service` with a fake store and fake sender and assert: `403` for a signed-in non-admin; `400` for a 15-character password; `502` when the fake sender fails verification; `204` for `DELETE` against an empty store; `405` for `PATCH`; and that a `200` `GET` body contains `address` and does **not** contain `appPassword`. Follows the request-construction style of `push_handler_test.go`. *(The unauthenticated-401 case is asserted here at handler level only — `callerEmailFromRequest` failing on a request with no session cookie. The `/api/*` middleware gate that rejects unauthenticated traffic before it reaches any handler is `middleware/auth.go`'s existing responsibility and is not re-tested by this change; the end-to-end 401 is acceptance criterion 6.)*

7. **Frontend: routes, model, API client, form rules** — files: `frontend/src/models/email.ts`, `frontend/src/api/emailApi.ts`, `frontend/src/state/hooks/server/emailSettingsForm.ts`, `frontend/src/state/hooks/server/emailSettingsForm.test.ts` (new); `frontend/src/config/routes.ts` (edit).
   Change: add `email: { settings: "/api/admin/email", test: "/api/admin/email/test" }` to `API_ROUTES` (`routes.ts:7-131`). `models/email.ts` exports `interface EmailSettings { configured: boolean; address: string }`. `emailApi` exposes `get()`, `save(address, appPassword)`, `remove()`, and `sendTest(to)` over `requestJson`, exactly as `googleOAuthApi` does (`api/authApi.ts:50-58`); `remove` and `sendTest` use `"DELETE"`/`"POST"` and rely on `requestJson`'s `204` → `undefined` branch (`apiRequest.ts:22`).
   `emailSettingsForm.ts` mirrors `localAuthFormState.ts` exactly: an unexported `class EmailSettingsForm` with one method, exported as the singleton `export const emailSettingsForm = new EmailSettingsForm()`. `prepareSubmission({address, appPassword})` returns `{valid: false as const, error: string}` or `{valid: true as const, address: string, appPassword: string}` with the address trimmed+lowercased and the password whitespace-stripped. Rules, in order: empty address → "Email address is required."; address without a single `@` between non-empty parts → "Enter a valid email address."; stripped password length ≠ 16 → "A Gmail app password is exactly 16 characters."
   Verify: `cd frontend && npm test` — `emailSettingsForm.test.ts` pins `"abcd efgh ijkl mnop"` → valid with a 16-character stripped password, a 15-character rejection, a 17-character rejection, an empty-address rejection, an address-without-`@` rejection, and that a mixed-case address is lowercased. No `frontend/package.json` change is needed: the `test` script is a bare `node --experimental-strip-types --test`, which discovers `*.test.ts` automatically — the named per-file scripts already in that file are conveniences, not registrations, as `projectPreviewUrlService.test.ts` alongside the untargeted `pushSubscriptionOwnership.test.ts` demonstrates.

8. **Frontend: Email settings tab** — files: `frontend/src/state/hooks/server/useEmailSettingsController.ts`, `frontend/src/ui/settings/EmailSettings.tsx` (new); `frontend/src/ui/primitives/icons.tsx`, `frontend/src/ui/settings/SettingsPage.tsx` (edit).
   Change: add `export const Mail = (p: P) => (<svg {...base} {...p}><rect x="2" y="4" width="20" height="16" rx="2"/><path d="m2 7 10 6 10-6"/></svg>);` to `icons.tsx` (`:8-67`).
   `useEmailSettingsController()` mirrors `useGoogleOAuthSettingsController.ts`: local `settings`/`address`/`appPassword`/`loading`/`saving`/`error` state, a mount effect calling `emailApi.get()`, and a `save(event)` that runs `emailSettingsForm.prepareSubmission` first, calls `emailApi.save`, then clears `appPassword` on success. It additionally exposes `sendTest(to)`, `remove()`, `testing`, and `testMessage` with their own error handling. It calls no context and takes no arguments.
   `EmailSettings.tsx` takes one prop — `{ defaultRecipient }: { defaultRecipient: string }` — because the test-send field needs a sensible starting value and `SettingsPage` already has `currentEmail` in scope (`SettingsPage.tsx:99`); the *credential* state stays self-contained in the hook. It reuses the card markup of `GoogleOAuthSettings.tsx`: header with `Mail` icon and a `configured` badge; a note stating that the Google account needs 2-Step Verification enabled, linking `https://myaccount.google.com/apppasswords`; a text input for the Gmail address; a `type="password"` input placeholdered `xxxx xxxx xxxx xxxx`; a save button; a "Send test email" row seeded with `defaultRecipient`; and a remove button shown only when `settings?.configured`.
   In `SettingsPage.tsx`: add `Mail` to the icon import block (`:8-19`), `"email"` to the `SettingsTab` union (`:31-39`), `{ id: "email", label: "Email", description: "Send mail from this server through your Gmail account.", Icon: Mail }` to `tabs` (`:41-95`), the `EmailSettings` import beside its siblings (`:20-28`), and the branch `{activeTab === "email" && (isAdmin ? <EmailSettings defaultRecipient={currentEmail} /> : <SettingsNotice>Email delivery is managed by server administrators.</SettingsNotice>)}` beside the existing branches (`:217-306`). `SettingsContainer.tsx` needs **no** change — it already passes `currentEmail` and `isAdmin`, and the tab is plain local state (`SettingsContainer.tsx:26`).
   Verify: `cd frontend && npm run build` (`tsc -b` type-checks the new union member, the prop, the hook, and the card) — then, with `go run ./cmd/remote` and `npm run dev` both running, sign in as an admin and confirm Settings shows an **Email** tab whose card renders in the unconfigured state with the address, password, and save controls reachable.

9. **Docs** — files: `ARCHITECTURE.md`, `docs/03-platform/07-data-and-frontend-state.md`, `docs/03-platform/08-api-and-realtime.md`, `docs/threat-model.md`, `README.md` (edit).
   Change: add `| SMTP credentials | `DATA_DIR/smtp.json` | JSON | Gmail address + app password, plaintext, mode 0600 |` to the persistence table (`ARCHITECTURE.md:224-241`); add `smtp.json` to the `DATA_DIR` tree (`07-data-and-frontend-state.md:18`) and its file table (`:151`); add `| GET, PUT, DELETE | /api/admin/email | Read, replace, or clear the SMTP sender configuration; admin only |` and `| POST | /api/admin/email/test | Send a test email from the configured sender; admin only |` beside the Google row (`08-api-and-realtime.md:35`); add `smtp.json` to the named plaintext files in finding 18 (`threat-model.md:220`); add a step to `README.md`'s "Create your first project" list (`README.md:148-163`) pointing at **Settings → Email** and stating that the account needs 2-Step Verification and a 16-character app password.
   Verify: `grep -rn "smtp.json" ARCHITECTURE.md docs/` returns the three expected files, and `grep -n "/api/admin/email" docs/03-platform/08-api-and-realtime.md` returns both new rows.

10. **Full local verification** — files: none.
    Change: none.
    Verify: `gofmt -l backend` (empty), `cd backend && go vet ./... && go test ./...` (all pass), `cd frontend && npm run build && npm test` (both pass). Then run the manual acceptance checks in *Verification plan* against a local `go run ./cmd/remote` using a throwaway Gmail account whose credentials are typed into the UI and **never** written to any file in the repository.

11. **Branch, commits, and pull request** — files: none (VCS only).
    Change: this repository has **two** remotes — `origin` is the fork `AbdallahMohamedDotnet/remote.futrx`, `upstream` is `futrx-com/remote.futrx`, and a `qa` branch exists on both. The checkout starts on an unrelated branch (`feat/2fa-recovery-codes-download`) whose work is **not** part of this change; do not build on it. Confirm `git status --porcelain` is clean, then `git fetch upstream && git switch -c feat/smtp-gmail-mvp upstream/qa`.
    Commit in the step order above using Conventional Commits with the scope `email` — e.g. `feat(email): verify Gmail SMTP credentials before storing them`, `feat(email): admin SMTP settings endpoints`, `feat(email): SMTP settings tab`, `docs(email): document SMTP configuration` — each signed off with `git commit -s` per `CONTRIBUTING.md`, and each ending with the `Co-Authored-By` trailer this repository's recent history uses. Push to `origin`, then open the PR against `futrx-com/remote.futrx` with base branch **`qa`**: `gh pr create --repo futrx-com/remote.futrx --base qa --head AbdallahMohamedDotnet:feat/smtp-gmail-mvp`.
    Verify: `git log --format=%H upstream/qa..HEAD | wc -l` equals `git log --format=%B upstream/qa..HEAD | grep -c '^Signed-off-by:'`; `git merge-base --is-ancestor upstream/qa HEAD` exits 0; and `gh pr view --repo futrx-com/remote.futrx --json baseRefName,headRefName` reports `"baseRefName": "qa"`.

---

## Data / migrations                        <!-- Phase 6 -->

**New file:** `DATA_DIR/smtp.json`, mode 0600, written by `stores/fileemail` via temp-file + rename under an in-process mutex.

**Shape:** `{"address": "<sender address>", "appPassword": "<16 characters, whitespace already stripped>"}`.

**Forward migration:** none required. The file's absence is the valid "not configured" state, so an existing installation upgrades with no action and the Email tab simply shows as unconfigured until an admin fills it in.

**Rollback:** deleting `smtp.json` returns the server to the unconfigured state; `DELETE /api/admin/email` does exactly that. Downgrading the binary to a build without this feature leaves an inert file that nothing reads. No other store, and no existing file, is read or written by this change.

**Regeneration / reseed:** none. `backend/public/` is regenerated by `npm run build` as part of the normal build, not as a data migration.

**Ordering constraints:** steps 1–2 (the SMTP client) and step 3 (the service and its ports) must both land before step 5, because the adapter in step 5 references types from each. Step 4 (the store) must also precede step 5, which must precede step 6 (the handler). Steps 7–8 depend on step 6's routes existing. The three literals in step 5 — `Services{}`, `Dependencies{}`, and `main.go`'s `service.Dependencies{}` — must be updated in the same commit as each other, since any one alone leaves the feature either uncompilable or silently nil. No step is destructive and none needs a maintenance window.

---

## Risks & open questions

- **A verification round-trip blocks an HTTP handler for up to the dial timeout.** → `dialTimeout` is 15s, the context deadline propagates from the request, and the operation is admin-only and manual, so it cannot be triggered at volume. Ordinary users are unaffected.
- **The app password is stored in plaintext at 0600, matching `oauth.json` and `projectsecrets/*.json`.** → This is the repository's documented posture (`docs/threat-model.md:220`); introducing a bespoke encryption scheme for one file would be a silent deviation with a key-management problem attached. Step 9 adds `smtp.json` to finding 18 so the exposure is recorded where operators already look. Mitigating it properly means encrypting `DATA_DIR` as a whole — out of scope, already recommendation 9 in the threat model (`:285`).
- **A compromised agent runs as root and can read `smtp.json`, gaining the ability to send mail as the admin's Gmail account.** → Same blast radius as `oauth.json` and every project secret today (`docs/threat-model.md:138`). The step-9 threat-model edit names the new capability so it is not discovered later by surprise. Operators who consider mail-sending unacceptable simply leave the feature unconfigured; nothing else depends on it.
- **Gmail rejects app passwords when 2-Step Verification is off on the Google account, with an error that reads like a wrong password.** → The card text states the 2SV requirement and links `https://myaccount.google.com/apppasswords`; step 2 preserves the server's own message in the returned error, so the admin sees Gmail's wording rather than a generic failure.
- **Google may throttle or block repeated failed logins from a new server IP.** → Verification is manual and admin-only, so the natural rate is low; the error text is surfaced verbatim so an admin can recognise a Google-side block.
- **`net/smtp` is frozen upstream (no new features accepted).** → It is still supported and does exactly what this MVP needs. Isolating it behind `email.Sender` means replacing it later touches one package.
- [ ] *(non-blocking)* Which product event should send the first real email — turn completion, `AskUserQuestion`, scheduled-run outcome, or user invitation? Deliberately deferred: the requester scoped this edition to configure + verify + test. The `Sender` port and `email.Message` are the seam that consumer will use.
- [ ] *(non-blocking)* Should a verified configuration be re-checked periodically, so an admin learns that a revoked app password broke delivery before the first real send fails? Not built here — there is no real send path yet to break.

---

## Verification plan

Acceptance criteria for the QA step. Each is checkable as written.

**Automated**
1. `gofmt -l backend` prints nothing.
2. `cd backend && go vet ./...` exits 0.
3. `cd backend && go test ./...` passes, including the new `internal/integration/smtp`, `internal/service/email`, `internal/stores/fileemail`, and `internal/transport/http/handlers` tests.
4. `cd frontend && npm run build` succeeds (`tsc -b` clean).
5. `cd frontend && npm test` passes, including `emailSettingsForm.test.ts`.

**Authorization**
6. `GET /api/admin/email` with no session cookie returns `401`.
7. `GET /api/admin/email` as a signed-in non-admin returns `403`.
8. Signed in as a non-admin, the Settings → Email panel shows the administrator notice and no credential inputs.

**Validation (no network required)**
9. `PUT /api/admin/email` with a 15-character app password returns `400` and does not create `DATA_DIR/smtp.json`.
10. `PUT /api/admin/email` with `not-an-email` as the address returns `400`.
11. `PUT /api/admin/email` with a password entered as `abcd efgh ijkl mnop` passes length validation (it is normalised to 16 characters before the rule is applied) and proceeds to the verification step.
11a. `PUT /api/admin/email` with the address `Admin <admin@example.com>` returns `400` — the stored value is an envelope address, not a display-name header.
11b. `POST /api/admin/email/test` with `{"to": "not-an-email"}` returns `400`, distinct from the `409` unconfigured case.

**Verification before save — the core requirement**
12. With a valid address and a deliberately wrong 16-character password, `PUT /api/admin/email` returns `502`, the response body carries a message naming an authentication failure, and `DATA_DIR/smtp.json` is **not created**.
13. With a previously working configuration saved, a subsequent `PUT` carrying a wrong password returns an error and the **previously stored credentials remain intact** — a following `GET` still reports `configured: true` with the original address, and `POST /api/admin/email/test` still succeeds.
14. With a valid Gmail address and a correct 16-character app password, `PUT /api/admin/email` returns `200 {"configured": true, "address": "..."}`, and `stat -c '%a' "$DATA_DIR/smtp.json"` prints `600`.

**Secret containment**
15. No response body from `GET`, `PUT`, or `POST .../test` contains the app password. With `PASS` holding the app password used in criterion 14: `curl -s -b "$COOKIE" https://<host>/api/admin/email | grep -cF "$PASS"` prints `0`, and so does the same check against the `PUT` and test-send responses (including the `502` body from criterion 12).
16. Server logs do not contain the app password. Across a full configure + test-send + failed-verify cycle, `journalctl -u remote --since "-10 min" | grep -cF "$PASS"` prints `0` (locally: pipe the `go run ./cmd/remote` output through the same `grep -cF`).
17. No credential is committed. `git diff upstream/qa...HEAD | grep -cF "$PASS"` prints `0`; `git diff --stat upstream/qa...HEAD -- .qa.env` is empty; and `git log --oneline upstream/qa..HEAD -- '*smtp.json'` is empty. (`.qa.env` and `DATA_DIR` are already git-ignored — this criterion confirms nothing bypassed that.)

**Send path**
18. With a valid configuration, `POST /api/admin/email/test {"to": "<inbox you control>"}` returns `200` and the message arrives, with the configured address as the sender and a readable plain-text body.
19. With no configuration stored, `POST /api/admin/email/test` returns `409` naming the not-configured state.
20. `DELETE /api/admin/email` returns `204`, removes `smtp.json`, and a following `GET` reports `configured: false`. A second `DELETE` also returns `204` rather than an error.
20a. On a server that has never been configured, `GET /api/admin/email` returns `200 {"configured": false, "address": ""}` — not a 404 and not a 500 — proving the absent-file path is a valid state end to end.

**Regression**
21. Google OAuth settings still load and save (`GET`/`PUT /api/admin/auth/google`), confirming the shared `Handlers` and store wiring were not disturbed.
22. Web Push subscribe and test-notify still work — the two features share the composition root touched in step 5.
23. Sign-in, chat creation, and a project chat turn still work end to end after `npm run qa:deploy-app -- <ref>` against the QA box (`AGENTS.md`). `install.sh` and `update.sh` are **not** required: nothing under `infra/` changed.

---

## Verification gate                        <!-- Phase 8 -->

**Mechanical pass (8a):** clean after two fixes. Initially, step 11 (branch/PR) carried no `Verify:` line and the *Extension points* table contained a row justified by "a future second provider". Fixed by giving step 11 concrete `git`/`gh` assertions and deleting the speculative row — the two remaining extension points are each required by a test written in this plan. Re-checked: Context carries `path:line` anchors for all five modules touched (`integration`, `service`, `stores`, `transport`, `frontend`); all 11 steps carry `Verify:`; no table cell is empty; every pattern row has a force plus prior art plus a rejected alternative; no extension-point justification contains "might", "in case", "later", or "future-proof"; *Data / migrations* is non-empty; *Hard constraints* is non-empty; the tier is recorded as structural and all phases were run, including the check-in before this file was written.

**Cold-reader pass (8b)** was run by a subagent given this file path and nothing else. It returned 25 defects; each was independently re-verified against the repository before being accepted, and each was fixed. The material ones:

*Would not have compiled.* (1) The step-2 client signature (`Verify(ctx, address, appPassword string)`) did not satisfy the step-3 port (`Verify(ctx, Credentials)`), and step 5 wired them together directly — with **no adapter step at all**, despite the cited prior art (`webPushSender`, `services.go:333`) being exactly that. Fixed by adding the `emailSender` adapter as its own deliverable in step 5, and by splitting the two type families explicitly in *Approach*. (2) `main.go` was never edited, so `deps.Email` would have been nil and the feature dead on a real server; the `service.Dependencies` literal at `main.go:82-101` is now named in step 5, in the wiring table, and in the conformance ledger. (3) Step 4 added `email := fileemail.New(dataDir)` without adding `Email: email` to the returned `Stores{}` literal — an unused-variable compile error; both are now spelled out. (4) `package smtp` importing `net/smtp` needs the `netsmtp` alias; step 2 now states it. (5) `Message.From` had no stated source; the adapter now owns it.

*Wrong anchors.* (6) The plan cited `googleConfigHandler` as the model for "constructed in `transport.go`, slotted in `Handlers`" — it is none of those things; it is embedded in `AuthHandler` (`auth_handler.go:17,28,38`). Context and the conformance ledger now separate the *semantic* reference (Google config) from the *wiring* reference (`PushHandler`) and warn against attaching email to `AuthHandler`. (7) `webpush/transport.go` does not "inject the HTTP transport"; the real seam is `publicDialer`'s unexported fields, substituted at `transport_test.go:20-22`. Both the patterns and extension-points rows now cite that accurately. (8) `(nil, nil)` for a missing file contradicts `fileauth`, which returns a sentinel; the convention actually followed is `TwoFactorStore.Get` (`service/auth/ports.go:46-52`), now cited, with the divergence called out in step 4. (9) `ARCHITECTURE.md:290` was cited for two unrelated claims; the second now cites the grep that establishes it.

*Behaviour I would have had to invent.* Message-ID generation (10) — removed entirely; Gmail assigns one. `Settings()` signature and empty-store behaviour (11), `SendTest` recipient validation (12), display-name address handling (13), how the failure cause reaches the client (14), nil-store safety (15), the `defaultRecipient` prop versus "self-contained" (16), the `emailSettingsForm` export shape and success return (17), and `DELETE` on an unconfigured server (18) are each now specified, and (11)–(13) and (18) gained acceptance criteria 11a, 11b, 20 and 20a.

*Unrunnable verification.* (19) `git grep` searches a tree, not a diff, and `[a-z]{16}` matches ordinary identifiers; (20) "any 16-character fragment" was vacuous. Criteria 15–17 are now literal `grep -cF "$PASS"` checks against response bodies, logs, and `git diff upstream/qa...HEAD`. (21) The 401 case is enforced by middleware, not the handler; step 6 now says which layer its test covers and defers the end-to-end case to criterion 6. (22) Step 5's claim that a test proves the new struct fields was replaced with `go build` plus a three-file `grep -n "Email:"`.

*Smaller.* (23) `package.json` needs no new script (bare `node --test` auto-discovers) — now stated with the in-repo evidence rather than left as a contradiction. (24) The `Mail` icon import into `SettingsPage.tsx` was missing from the edit list; added. (25) Step 11 branched from `upstream/qa` while the checkout sits on unrelated feature work; the step now says explicitly not to build on it and gives the exact `fetch`/`switch` commands.

**Judgment items:** scope is one sentence with six explicit non-goals; orientation records framework versions from `go.mod` and `package.json` and states that no lockfile entry changes; the sources ledger covers every documentation category present and records the two that are absent (`docs/plans/`, ADRs); nothing contradicts the hard constraints (layering respected — reinforced by the adapter, which is what keeps `integration/smtp` free of `service/` imports — no dependency added, PATCH-class, DCO noted, no credential in this file); all five Phase 3 questions are answered with anchors, with the reference implementation now split into its semantic, wiring, and outbound-integration halves; the conformance ledger is present with no deviations; every abstraction traces to a force (F1–F9), including the adapter (F2 plus the layering constraint); the SOLID review points at named elements of this design; every step's verification uses `go test`, `go build`, `go vet`, `gofmt`, `npm test`, `npm run build`, `grep`, `git`, or `gh` — all tooling the project already has; risks carry mitigations and both open questions are marked non-blocking; the verification plan gives QA 26 checkable criteria.

**Gate:** both passes clean. The cold-reader guess-list is empty as of this revision; all 25 items were fixed rather than waived.

---

## Progress

- [x] 1. SMTP integration: message building — `verified`: `go test ./internal/integration/smtp/ -run TestBuild` passes all five cases.
- [x] 2. SMTP integration: client — `verified`: `go test ./internal/integration/smtp/` passes against a fake TCP server (auth success, 535 rejection without leaking the password, no-greeting deadline, and a full Send).
- [x] 3. Email service — `verified`: `go test ./internal/service/email/` passes, including nil-store safety and the failed-verify-does-not-save case.
- [x] 4. File store — `verified`: `go test ./internal/stores/... && go build ./...` passes; round-trip, `(nil, nil)` on absence, idempotent `Delete`, and mode 0600 all confirmed.
- [x] 5. Compose the service and adapt the sender — `verified`: `go build ./... && go vet ./... && go test ./internal/service/...` passes (aside from the pre-existing unrelated failure noted below); `grep -n "Email:"` returns a line in each of the three wiring files.
- [x] 6. HTTP handler — `verified`: `go test ./internal/transport/...` passes, including 401 (no session), 403 (non-admin), 400 (invalid password), 502 (verification failure), 204 (idempotent delete), 405 (unsupported method), and that the `GET` body omits the app password.
- [x] 7. Frontend: routes, model, API client, form rules — `verified`: `npm test` (256/256) and `npm run build` (`tsc -b` clean) both pass.
- [x] 8. Frontend: Email settings tab — `verified-indirectly`: `npm run build` type-checks the new tab/hook/card, and a live `go run ./cmd/remote` exercised through the real HTTP API (claim admin, `GET`/`PUT`/`DELETE /api/admin/email`) confirmed the wiring end-to-end. Sufficient because it proves the same code path the browser would exercise; clicking through the rendered tab in an actual browser was not done (no browser tooling available in this environment).
- [x] 9. Docs — `verified`: `grep -rn "smtp.json" ARCHITECTURE.md docs/` returns the three expected files, and `grep -n "/api/admin/email" docs/03-platform/08-api-and-realtime.md` returns both new rows.
- [x] 10. Full local verification — `verified`: `gofmt`, `go vet ./...`, `go test ./...` (backend), and `npm run build && npm test` (frontend) all pass except the pre-existing failure below. The manual acceptance checks against a real Gmail account (criteria 12–14, 18–19 in *Verification plan*) were **not** run — no throwaway Gmail credentials are available in this environment; see Outcome.
- [ ] 11. Branch, commits, and pull request — `blocked`: not started. This environment's operating rules prohibit committing, pushing, or opening a pull request without an explicit user request, which supersedes this step. All 10 preceding steps are implemented and verified in the working tree on branch `feat/2fa-recovery-codes-download`, uncommitted. See Outcome for the branch discrepancy this step was meant to resolve.

## Outcome                                  <!-- left empty; filled after implementation -->

**Deviations from the plan:** None in the code. Steps 1–10 were implemented exactly as specified, including the `emailSender` adapter, the three-literal wiring, and the `(nil, nil)`-on-absence convention. One procedural deviation: step 11 (branch creation, commits, PR) was not performed. The implementation happened on the checkout's starting branch, `feat/2fa-recovery-codes-download`, rather than a fresh branch off `upstream/qa` as step 11 specifies — the environment's own operating rules forbid creating branches, committing, or opening a PR without an explicit user request, and "start implementation" was not read as that request. All ten code/doc steps are complete and verified in the working tree; nothing is committed anywhere.

**Plan defects:** Step 11's branch instruction is sequenced too late. Its own text says "the checkout starts on an unrelated branch... do not build on it," which only makes sense as a Phase-2/pre-step-1 action, not something to run after nine other steps have already changed files in that same checkout. A future plan should place a branch-hygiene precondition in *Orientation* or as step 0, not folded into the final commit/PR step.

**Steps not fully verified:** Step 8 is `verified-indirectly` — the Email tab's type-checking and the underlying API were exercised for real (a live `go run ./cmd/remote`, a claimed admin session, and live `GET`/`PUT`/`DELETE /api/admin/email` calls all behaved as specified), but no browser was available in this environment to click through the rendered tab itself. Step 10's automated checks are `verified`; its manual acceptance criteria 12–14 and 18–19 (configuring against a real Gmail account, receiving a live test email, verifying a wrong-password rejection through the real handler) were not run because this environment has no throwaway Gmail app password to use, and none should be fabricated. Step 11 is `blocked` on an explicit user decision (see below).

**Left behind:** Step 11 is undone. Before this can ship: (1) decide how to handle the branch situation — the work is currently uncommitted on `feat/2fa-recovery-codes-download`, which already carries one unrelated commit (`62154c91`, the 2FA recovery-codes-download feature) not part of this change; moving the uncommitted files onto a fresh branch off `upstream/qa` (as the plan specifies) needs an explicit go-ahead since it involves branch/git operations; (2) once on the right branch, commit in the step order with DCO sign-off and open the PR against `futrx-com/remote.futrx` base `qa`; (3) run the manual Gmail-account acceptance criteria (12–14, 18–19) with real (throwaway) credentials, never committed.

**For the next plan in this area:** The reference-implementation split (semantic vs. wiring vs. outbound-integration) and the three-literal wiring table were both exactly right and caught real mistakes before they were made — keep that structure for the next store/service/handler addition. Also worth knowing: this repository currently has a pre-existing, unrelated defect at `backend/internal/service/auth/service_test.go:85` — a stray corrupted line (literal ` B N`) that breaks `go vet` and `go test` for the `internal/service/auth` package only (confirmed present on `upstream/qa` itself via `git show`, not introduced by any work here). It does not block `go build ./...` or any other package's tests, and no step in this plan touches that package, but it will surface as a red `go test ./...` for the next person too until someone deletes that line.
