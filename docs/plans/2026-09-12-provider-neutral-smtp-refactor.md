# Provider-neutral SMTP configuration refactor

**Status:** Ready to implement.

**Target branch:** Refactor the current `feat/smtp-gmail-mvp` work before it is merged upstream.

**Tier:** Structural, full-stack feature refactor. This changes the persisted SMTP record, the service and integration contracts, the admin HTTP API, and the Settings UI while preserving the existing outbound `Mailer` behavior.

**Goal:** Replace the Gmail-specific SMTP implementation with one provider-neutral SMTP configuration. Gmail remains available only as a frontend-owned, recommended preset that fills the generic fields. The backend must not contain Gmail host, port, password-length, provider-name, or help-text constants.

**Scope:**

- Keep one server-wide SMTP configuration managed by administrators.
- Support required STARTTLS, implicit TLS, and an unauthenticated plaintext relay mode.
- Support SMTP PLAIN, LOGIN, and no authentication.
- Separate the sender address from the authentication username.
- Verify a complete candidate configuration before replacing a working configuration.
- Preserve the write-only treatment of the password.
- Preserve the existing mail builder, templates, recipient handling, synchronous and asynchronous delivery behavior, and test-email route.
- Add a Gmail preset marked **Recommended** in the frontend.
- Add a fully editable Custom SMTP mode in the frontend.

**Non-goals:**

- OAuth2/XOAUTH2 authentication.
- Client-certificate authentication.
- Certificate-verification bypasses.
- Multiple SMTP accounts, provider failover, or routing by message type.
- Per-user sender configuration or email preferences.
- Delivery queues, persistence, retry policy, bounce handling, or rate limiting.
- Changing message templates, MIME layout, recipients, delivery concurrency, or background-send semantics.
- Moving unrelated repository models or ports merely to make the whole repository match the new layout.

---

## Instructions and sources read

The implementation must follow the supplied `code-refactorer` skill and the configuration, modeling, and commit instructions supplied with this request.

Read for this plan:

- Supplied `code-refactorer` skill, in full.
- Supplied backend refactoring reference, in full.
- Supplied configuration rules.
- Supplied data-modeling rules.
- Supplied commit rules.
- `AGENTS.md` QA workflow supplied for this repository.
- `CONTRIBUTING.md`, especially DCO sign-off and repository build/test commands.
- `ARCHITECTURE.md`, especially backend layering, email delivery, and persistence.
- `docs/plans/2026-09-05-smtp-gmail-mvp.md`, including its implementation outcome.
- Current backend SMTP integration, email service, file store, handler, composition, and tests.
- Current frontend email model, API adapter, controller, form validation, settings component, routes, and tests.
- Current API and global-settings documentation.

The `code-refactorer` skill refers to `references/frontend.md`, but that file was not included in either attachment and was not available in the local skill directories. This plan therefore applies the skill's shared frontend rules directly: presentation, state, boundary DTOs, transport, and provider catalogs have distinct owners; the frontend does not import backend implementation models; and provider-specific public defaults live in frontend configuration rather than business or transport code.

---

## Repository state precondition

Planning was performed without modifying or staging existing user work. At planning time:

- `.gitignore` is in an unmerged `UU` state.
- `backend/internal/stores/fileauth/store.go` has unrelated modifications.
- `docs/dev/2fa-global-update-lifecycle-plan.md` is untracked and unrelated.
- Local Git identity is configured as `Abdallah Mohamed <abdoooomohamed88@gmail.com>`.

Before implementation:

1. The user must resolve the existing `.gitignore` conflict; SMTP work must not choose a resolution for it.
2. Preserve the unrelated fileauth change and untracked 2FA plan.
3. Re-run `git status --short` and record all pre-existing paths.
4. Work on `feat/smtp-gmail-mvp` only after confirming its relationship to the intended PR base.
5. Never use broad staging. Stage only exact SMTP task paths or exact hunks.
6. Use the existing Git identity and `git commit -s`. Do not add a `Co-Authored-By` trailer or any agent identity.
7. Do not push or open/update a PR unless separately authorized.

---

## Current behavior to preserve

The following behavior is frozen unless this plan explicitly replaces the Gmail-specific configuration contract:

- `Services.Mailer` remains the only email entry point for other application services.
- Callers compose mail without knowing SMTP credentials, endpoints, authentication, MIME, or provider details.
- A message is sent separately to each deduplicated recipient.
- `SendAsync` validates composition and recipient resolution synchronously, then performs only delivery in the background.
- Background delivery uses its own bounded timeout and the existing concurrent-send limit.
- Optional mail is a logged no-op when SMTP is unconfigured.
- `.Required()` mail returns `ErrNotConfigured` when SMTP is unconfigured.
- Test mail is required and reports an unconfigured server as an error.
- A failed configure/verify attempt does not overwrite the last working configuration.
- SMTP secrets never appear in a GET response, successful response, error, or log message.
- Admin authorization remains enforced by the HTTP handler for every settings/test operation.
- `DELETE /api/admin/email` remains idempotent.
- The configuration file remains under `DATA_DIR`, uses an in-process mutex, is written through a temporary file plus rename, and has mode `0600` under a `0700` directory.
- Existing plain-text and multipart/alternative message generation remains byte-compatible except for the configured `From` address already supplied at the SMTP boundary.

Baseline checks before the first structural change:

```bash
cd backend
go test ./internal/integration/smtp/...
go test ./internal/service/email/...
go test ./internal/stores/fileemail/...
go test ./internal/transport/http/handlers/... -run Email

cd ../frontend
npm test -- src/state/hooks/server/emailSettingsForm.test.ts
npm run build
```

If a frozen behavior is not covered strongly enough to survive a move, add only the missing characterization test in a dedicated first commit. Do not add broad speculative test suites.

---

## Concrete diagnosis

### 1. SMTP protocol and Gmail policy are one object

**Evidence:** `backend/internal/integration/smtp/client.go:12-47` owns Gmail host/port constants and constructs a Gmail endpoint; `:77-81` assumes STARTTLS; `:96` and `:110` always use PLAIN authentication with the sender address as the username.

**Smell:** The integration owns both generic SMTP mechanics and one provider's configuration policy.

**Principles:** Single Responsibility and Open/Closed.

**Fix:** Make the SMTP client consume a validated provider-neutral configuration per operation. Isolate TCP/TLS connection mechanics and authentication selection inside the integration. Put no provider preset in the backend.

### 2. Gmail credentials are modeled as the application contract

**Evidence:** `backend/internal/service/email/model.go:5-22` exposes `AppPasswordLength`, `Credentials{Address, AppPassword}`, and settings containing only one address; `credentials.go:24-47` strips whitespace and requires exactly 16 characters.

**Smell:** Provider-specific input rules leak into application models and constrain every future SMTP server.

**Principles:** Dependency Inversion and precise modeling.

**Fix:** Replace `Credentials` with a provider-neutral `SMTPConfiguration`; distinguish secret-bearing internal configuration from the public settings projection; move Gmail app-password normalization into the frontend Gmail preset workflow only.

### 3. Public models and behavioral interfaces are mixed into the service package

**Evidence:** `backend/internal/service/email/model.go` holds data definitions and `backend/internal/service/email/ports.go` holds store, sender, and directory contracts.

**Smell:** Data categories and dependency capabilities do not have the canonical `model/<context>/<category>` and `port/<context>/<direction>` ownership required by the supplied modeling rules.

**Principles:** Interface Segregation and explicit boundary ownership.

**Fix:** Move email data models into bounded email model packages and consumer-owned behavioral contracts into an email outbound-port package. Keep services, stores, handlers, and integrations out of `model/`.

### 4. Persistence cannot describe a custom SMTP server

**Evidence:** `backend/internal/stores/fileemail/store.go:30-68` persists only `address` and `appPassword`.

**Smell:** The storage record encodes the Gmail MVP rather than the application's SMTP configuration.

**Principles:** Single Responsibility and boundary mapping.

**Fix:** Persist a versioned provider-neutral record. Keep the record private beside the store because it is implementation-only, and map it to the application model at the store boundary.

### 5. The HTTP boundary is Gmail-specific and uses anonymous DTOs

**Evidence:** `backend/internal/transport/http/handlers/email_settings_handler.go:41-83` exposes only `address` and `appPassword` through handler-local structs.

**Smell:** The public API contract is not modeled explicitly and cannot express connection or authentication choices.

**Principles:** Explicit API boundaries and precise types.

**Fix:** Add email API DTOs under `internal/model/email/api`, validate/decode once in the handler, map to an application command, and never include the password in a response DTO.

### 6. The frontend form treats Gmail rules as universal

**Evidence:** `frontend/src/state/hooks/server/emailSettingsForm.ts:1-26` requires a 16-character app password for every submission; `frontend/src/ui/settings/EmailSettings.tsx:42-80` contains Gmail-only text and fields.

**Smell:** Provider catalog, form policy, controller state, and presentation are coupled around one provider.

**Principles:** Single Responsibility and Open/Closed.

**Fix:** Add a frontend-owned provider preset catalog, a provider-neutral form model, and distinct Gmail guidance versus Custom SMTP controls. The backend receives only resolved generic values, not a provider ID.

### 7. SMTP runtime constants live inside the integration

**Evidence:** `backend/internal/integration/smtp/client.go:19` owns `dialTimeout`.

**Smell:** An application runtime constant is outside the centralized configuration owner.

**Principle:** Configuration ownership.

**Fix:** Move generic SMTP timeout values into `backend/internal/config/constants/email.go`, pass them through the composition root, and keep dependency construction out of `config/`.

---

## Target model and ownership

Use the email bounded context and singular `model/` and `port/` roots.

```text
backend/internal/model/email/
  domain/
    address.go
    message.go
  application/
    smtp_configuration.go
    smtp_settings.go
  api/
    smtp_settings.go

backend/internal/port/email/
  outbound/
    configuration_store.go
    sender.go
    directory.go
```

Suggested Go package names are `emaildomain`, `emailapplication`, `emailapi`, and `emailoutbound` so imports remain unambiguous.

### Domain models

`emaildomain.Message` preserves the existing fields:

```go
type Message struct {
    To       string
    Subject  string
    Body     string
    HTMLBody string
}
```

Address parsing/normalization used by message composition belongs with the email domain. Do not mix SMTP host or authentication validation into it.

### Application models

```go
type TLSMode string

const (
    TLSModeSTARTTLS TLSMode = "starttls"
    TLSModeImplicit TLSMode = "implicit"
    TLSModeNone     TLSMode = "none"
)

type AuthenticationMode string

const (
    AuthenticationPlain AuthenticationMode = "plain"
    AuthenticationLogin AuthenticationMode = "login"
    AuthenticationNone  AuthenticationMode = "none"
)

type SMTPConfiguration struct {
    Host           string
    Port           uint16
    TLSMode        TLSMode
    Authentication AuthenticationMode
    Username       string
    Password       string
    FromAddress    string
}

type PublicSMTPConfiguration struct {
    Host               string
    Port               uint16
    TLSMode            TLSMode
    Authentication     AuthenticationMode
    Username           string
    FromAddress        string
    PasswordConfigured bool
}

type SMTPSettings struct {
    Configuration *PublicSMTPConfiguration
}
```

`nil` configuration represents the valid unconfigured state. A public model cannot contain a password field.

### Outbound ports

```go
type ConfigurationStore interface {
    Configuration(context.Context) (*emailapplication.SMTPConfiguration, error)
    Save(context.Context, emailapplication.SMTPConfiguration) error
    Delete(context.Context) error
}

type Sender interface {
    Verify(context.Context, emailapplication.SMTPConfiguration) error
    Send(context.Context, emailapplication.SMTPConfiguration, emaildomain.Message) error
}
```

Keep the directory port separate because user-address resolution is independent from SMTP delivery and configuration persistence.

### API models

Use explicit request/response DTOs rather than handler-local anonymous structs.

```go
type SaveSMTPSettingsRequest struct {
    Host           string  `json:"host"`
    Port           uint16  `json:"port"`
    TLSMode        string  `json:"tlsMode"`
    Authentication string  `json:"authentication"`
    Username       string  `json:"username"`
    Password       *string `json:"password,omitempty"`
    FromAddress    string  `json:"fromAddress"`
}

type SMTPSettingsResponse struct {
    Configured    bool                         `json:"configured"`
    Configuration *PublicSMTPConfigurationDTO `json:"configuration,omitempty"`
}
```

The pointer password distinguishes an omitted password from a supplied empty password. An omitted value may preserve an existing secret during an update; an explicitly empty value is invalid whenever authentication is enabled.

The backend API contract is the canonical cross-stack owner for enum wire values and field names. Frontend DTO definitions mirror that process boundary only; Gmail provider defaults are not part of the backend contract.

---

## Validation and security decisions

The service validates before any network operation:

- Trim `host`, `username`, and `fromAddress`.
- Require a host without a URL scheme, path, query, whitespace, or embedded port.
- Accept a valid DNS name, IPv4 address, or IPv6 literal as the host.
- Require port `1..65535`.
- Require a recognized TLS mode and authentication mode.
- Parse `fromAddress` as one bare envelope address; reject display-name input and header line breaks.
- Require non-empty username and password for PLAIN or LOGIN authentication.
- Allow an omitted password on update only when a stored authenticated configuration already has one; merge it before verification.
- Permit `tlsMode: none` only with `authentication: none` so credentials are never sent over plaintext.
- Clear username/password from the persisted candidate when authentication is `none`.
- Keep TLS certificate and hostname verification mandatory. Do not add `InsecureSkipVerify` to any public or runtime setting.
- Require STARTTLS advertisement and a successful upgrade when `tlsMode` is `starttls`; never continue in plaintext when it is absent or rejected.
- Use implicit TLS from the first byte when `tlsMode` is `implicit`.
- Never include the password in wrapped errors. Tests must search the complete error chain and serialized response for the exact supplied secret.

Arbitrary SMTP hosts create an intentional admin-only outbound-network capability. This is acceptable within the current trust model because only a server administrator can configure it, but it must be documented in the threat model as an SSRF/network-pivot surface. Do not weaken the existing per-request admin check.

---

## SMTP integration design

`backend/internal/integration/smtp` remains the sole owner of SMTP wire mechanics and imports no service package.

Refactor `Client` into a stateless adapter configured with generic runtime dependencies:

```go
type Client struct {
    dialTCP   DialTCP
    dialTLS   DialTLS
    timeout   time.Duration
    localName string
}
```

The production constructor receives timeout settings from the composition root. Private test constructors may inject dialers and a trusted test TLS configuration, but production configuration must not expose certificate bypasses.

Connection flow:

1. Construct `host:port` with `net.JoinHostPort`.
2. Apply the caller deadline, capped by the configured SMTP operation timeout.
3. For `implicit`, establish TLS first and then create the SMTP client over that connection.
4. For `starttls`, establish TCP, issue EHLO, require the STARTTLS extension, upgrade with `ServerName: host`, and issue SMTP commands only over the secured session.
5. For `none`, establish TCP and proceed only when authentication is also `none`.
6. Apply the selected authentication implementation: PLAIN, LOGIN, or none.
7. For verification, complete connection/authentication and quit without sending a message.
8. For delivery, build the existing RFC 5322/MIME message, then issue MAIL FROM, RCPT TO, DATA, and QUIT.
9. Close the underlying connection on every failure path.

PLAIN may use `net/smtp.PlainAuth` after the service has guaranteed a secure transport. LOGIN requires a small integration-private `smtp.Auth` implementation with tests for challenge ordering. Do not retry one authentication mechanism after another: the selected mode is explicit, avoids duplicate failed-login attempts, and gives deterministic error reporting.

---

## Service workflow

The email service remains the policy owner. Its configure operation becomes:

```text
decode request
  -> validate public fields
  -> load current configuration when password is omitted
  -> build a complete secret-bearing candidate
  -> verify candidate through Sender
  -> persist candidate only after verification succeeds
  -> return a secret-free public projection
```

Important cases:

- First authenticated configuration with no password: validation error, no network call, no write.
- Existing authenticated configuration with an omitted password: reuse the stored password, verify the complete edited candidate, then save.
- Existing configuration with an explicitly empty password: validation error.
- Change from authenticated to unauthenticated: clear username and password, verify connection, then save.
- Change from unauthenticated to authenticated: require a new password.
- Failed verification: retain the complete previous record unchanged.
- Missing stored record: return an unconfigured settings result rather than an error.
- Store/integration unavailable: preserve current nil-safe `ErrNotConfigured` behavior.

The mail builder, `Mailer`, templates, and all service callers must depend only on domain `Message`, `Service.send`, and the existing facade. They must not gain host, port, TLS, authentication, or provider branches.

---

## Persistence and migration

Persist a private versioned record in `DATA_DIR/smtp.json`:

```json
{
  "version": 2,
  "host": "smtp.example.com",
  "port": 587,
  "tlsMode": "starttls",
  "authentication": "plain",
  "username": "mailer@example.com",
  "password": "secret",
  "fromAddress": "mailer@example.com"
}
```

The password remains plaintext at rest under mode `0600`, matching the repository's existing documented secret-storage posture. It is never exposed by the public settings projection.

The current Gmail SMTP work is present on `origin/feat/smtp-gmail-mvp` and is not contained by an upstream branch. Therefore the implementation should replace the unreleased record format before merge and should not add permanent Gmail-aware compatibility logic to the backend.

Consequences:

- No automatic v1-to-v2 Gmail migration is implemented.
- A developer/QA installation created from the feature branch must delete its experimental `smtp.json` and configure SMTP again.
- If maintainers confirm that the Gmail-only branch was deployed as supported production state, stop before implementation: migration policy needs explicit authorization because inferring `smtp.gmail.com:587` in backend migration code contradicts the no-backend-Gmail requirement.
- Unknown versions and incomplete records return a clear storage/configuration error without logging secret fields.
- Rollback before release is removal of `smtp.json`; after v2 is released, older Gmail-only binaries cannot consume the v2 record and must not be presented as a supported rollback path.

---

## HTTP contract

Keep the existing paths and authorization behavior:

```text
GET    /api/admin/email
PUT    /api/admin/email
DELETE /api/admin/email
POST   /api/admin/email/test
```

Unconfigured GET:

```json
{
  "configured": false
}
```

Configured GET/PUT response:

```json
{
  "configured": true,
  "configuration": {
    "host": "smtp.example.com",
    "port": 587,
    "tlsMode": "starttls",
    "authentication": "plain",
    "username": "mailer@example.com",
    "fromAddress": "mailer@example.com",
    "passwordConfigured": true
  }
}
```

PUT request:

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "tlsMode": "starttls",
  "authentication": "plain",
  "username": "mailer@example.com",
  "password": "write-only value",
  "fromAddress": "mailer@example.com"
}
```

Error mapping:

- `400`: malformed JSON, unknown enum, invalid host/port/address, invalid security/authentication combination, or missing required username/password.
- `401`: no valid session.
- `403`: authenticated non-admin.
- `409`: an operation requiring configured SMTP when no configuration exists.
- `502`: SMTP verification or delivery failure.
- `500`: persistence or unexpected internal failure.
- `204`: idempotent delete.

Retain strict JSON decoding behavior already provided by the repository helper. Provider errors may be summarized for an administrator but must not echo submitted credentials.

---

## Frontend design

### Canonical public preset catalog

Create:

```text
frontend/src/config/constants/email-providers.ts
```

It contains values only and performs no I/O or state mutation:

```ts
export const EMAIL_PROVIDER_PRESETS = {
  gmail: {
    id: "gmail",
    label: "Gmail",
    recommended: true,
    host: "smtp.gmail.com",
    port: 587,
    tlsMode: "starttls",
    authentication: "plain",
    passwordKind: "gmail-app-password",
  },
} as const;
```

This is the one canonical owner of Gmail public configuration. No other frontend file copies the host, port, or preset choices. The backend contains none of them.

Generic UI option labels for TLS and authentication belong in the same email configuration domain, in a separate constant if needed. Runtime behavior and field validation do not belong in the constants file.

### Frontend models and outbound port

Use singular roots for new or moved public definitions:

```text
frontend/src/model/email/api/
  smtpSettings.ts
frontend/src/model/email/application/
  smtpSettingsForm.ts
frontend/src/port/email/outbound/
  emailSettingsGateway.ts
```

Move the existing `frontend/src/models/email.ts` boundary definition into the new API model and delete the old file after all imports move.

`EmailSettingsGateway` is justified because the controller performs a real HTTP boundary workflow and needs focused tests without global transport mocking. `frontend/src/api/emailApi.ts` implements that port. The hook/controller receives the gateway through a small factory/default parameter wired by the UI composition site; do not add a global dependency registry.

### UI state and presentation

The form has two presentation choices:

1. **Gmail — Recommended**
2. **Custom SMTP**

The provider selection is frontend presentation state only. It is not sent to or persisted by the backend.

Gmail behavior:

- Selecting Gmail fills host `smtp.gmail.com`, port `587`, STARTTLS, and PLAIN authentication from the preset catalog.
- Sender address and username are initially linked; editing the Gmail address updates both until the user explicitly edits the username.
- Show the existing 2-Step Verification guidance and Google app-password link only in Gmail mode.
- Strip whitespace from a pasted Gmail app password and require 16 characters before submission.
- Mark the provider as **Recommended**.

Custom SMTP behavior:

- Show editable host, port, TLS mode, authentication mode, username, password, and sender-address fields.
- Do not impose Gmail's 16-character rule or whitespace removal on a custom password.
- Hide/disable username and password when authentication is none.
- If TLS is none, authentication must be none and the UI explains that the mode is intended for a trusted relay.
- Default a fresh custom form to STARTTLS and PLAIN without silently copying Gmail host values.

Editing an existing configuration:

- The backend does not return a provider ID.
- The frontend classifies a configuration as Gmail only when its public host, port, TLS mode, and authentication exactly match the Gmail preset; otherwise it selects Custom SMTP.
- Password input is always blank after load/save.
- When `passwordConfigured` is true, explain that leaving the password blank retains the stored value.
- Switching authentication to none omits the password and lets the backend clear it.

Split presentation only where responsibilities are real. Suggested components:

```text
frontend/src/ui/settings/email/
  EmailSettings.tsx
  EmailProviderSelector.tsx
  SMTPConnectionFields.tsx
  SMTPTestPanel.tsx
```

`EmailSettings` owns layout/composition; the selector renders provider choice; connection fields render configuration inputs; the test panel owns only test-recipient presentation. The controller remains the single state owner. Do not duplicate form state inside child components.

---

## Implementation steps and commits

Every commit must compile, preserve unrelated work, use exact staging, and pass its named check. If a contract and its callers cannot compile independently, keep them in the same commit.

### Commit 1 — characterize missing behavior, only if required

**Subject:** `test(email): characterize SMTP configuration behavior`

**Files:** Existing SMTP integration/service/store/handler/frontend test files only.

Add only tests needed to freeze behavior that will be moved but is not already covered: failed verification preserves a working configuration, secret omission, delete idempotency, Mailer unconfigured/required behavior, and asynchronous semantics. Do not add new generic SMTP behavior in this commit.

**Verify:**

```bash
cd backend
go test ./internal/integration/smtp/... ./internal/service/email/... ./internal/stores/fileemail/...
go test ./internal/transport/http/handlers/... -run Email
cd ../frontend
npm test -- src/state/hooks/server/emailSettingsForm.test.ts
```

If existing tests already cover all movement risks, omit this commit rather than creating redundant tests.

### Commit 2 — move models and ports without changing behavior

**Subject:** `refactor(email): establish canonical models and outbound ports`

**Files:**

- Add `backend/internal/model/email/{domain,application,api}/` as required by the currently existing Gmail contract.
- Add `backend/internal/port/email/outbound/`.
- Update email service, SMTP adapter, file store, handler, tests, and composition imports.
- Move `frontend/src/models/email.ts` to `frontend/src/model/email/api/` and update imports.
- Delete superseded service-local model/port definitions only after all callers move.

This commit is a mechanical ownership move. Preserve Gmail fields and runtime behavior temporarily so the compiler and existing tests prove the move.

**Verify:**

```bash
cd backend
go test ./internal/integration/smtp/... ./internal/service/email/... ./internal/stores/fileemail/... ./internal/transport/http/handlers/...
go build ./...
cd ../frontend
npm run build
npm test
```

### Commit 3 — generalize SMTP connection mechanics

**Subject:** `refactor(smtp): support configurable transport and authentication`

**Files:**

- `backend/internal/integration/smtp/client.go` and focused collaborators/tests.
- `backend/internal/config/constants/email.go` for generic timeouts.
- Composition wiring required to pass the timeout.

Introduce host/port inputs, STARTTLS/implicit/plain connection strategies, PLAIN/LOGIN/none authentication, strict TLS behavior, and injected test dialers. Keep a temporary Gmail-shaped call site only inside the existing composition adapter so observable application/API behavior remains unchanged in this structural commit. The temporary adapter must be deleted in Commit 4.

**Verify:**

```bash
cd backend
gofmt -l internal/integration/smtp internal/config/constants
go test ./internal/integration/smtp/...
go test ./internal/service/email/...
go build ./...
```

### Commit 4 — replace the full-stack SMTP configuration contract

**Subject:** `feat(email): support provider-neutral SMTP settings`

**Files:**

- Application and API email models.
- Email service validation/workflow and tests.
- Email outbound ports.
- `integration/smtp` final signatures.
- `stores/fileemail` record/mapping/tests.
- Email HTTP handler/tests.
- Service/store/transport composition files.
- `frontend/src/config/constants/email-providers.ts`.
- Frontend email API/application models and outbound port.
- `frontend/src/api/emailApi.ts`.
- Email form policy/controller and tests.
- Email settings components and Settings tab copy.

Replace Gmail credentials with the final provider-neutral model, add update-with-existing-password behavior, change `smtp.json` to version 2, update both sides of the API contract, add the frontend gateway and Gmail preset, implement the Custom SMTP form, and remove the temporary Gmail adapter/constants from the backend completely.

This is one commit because the service, store, integration, handler, frontend API adapter, controller, and UI are inseparable callers of the same changed process-boundary contract. Splitting the backend and frontend would leave a buildable but unusable application revision. Do not leave compatibility shims after this commit.

**Verify:**

```bash
cd backend
gofmt -l .
go test ./internal/integration/smtp/...
go test ./internal/service/email/...
go test ./internal/stores/fileemail/...
go test ./internal/transport/http/handlers/... -run Email
go build ./...
cd ../frontend
npm test
npm run build
```

Also run:

```bash
rg -n 'GmailHost|GmailPort|AppPasswordLength|smtp\.gmail\.com|Gmail credential|app password' \
  backend/internal backend/cmd
```

The command must return no backend Gmail configuration or copy. Generic comments about tests/docs are not an excuse for provider policy remaining in runtime code.

Also run:

```bash
rg -n 'smtp\.gmail\.com|587|16' frontend/src
```

Any Gmail SMTP preset match must resolve to `config/constants/email-providers.ts`; tests may contain expected literal values but runtime code must not duplicate them.

### Commit 5 — update architecture, user, API, and security documentation

**Subject:** `docs(email): document provider-neutral SMTP settings`

**Files:**

- `ARCHITECTURE.md`.
- `docs/02-user-guide/10-global-settings-users-providers.md`.
- `docs/03-platform/07-data-and-frontend-state.md`.
- `docs/03-platform/08-api-and-realtime.md`.
- `docs/threat-model.md`.
- The historical Gmail MVP plan only to mark it superseded and link here; do not rewrite its historical decisions/outcome.

Document generic SMTP settings, Gmail as a UI preset, the v2 persistence fields without example secrets, TLS/authentication modes, password retention, and the admin-controlled outbound-network risk.

**Verify:**

```bash
rg -n 'Gmail credential|Gmail address \+ app password|through your Gmail account' ARCHITECTURE.md docs frontend/src
rg -n 'SMTP|smtp.json|/api/admin/email' ARCHITECTURE.md docs
```

The first search must contain only explicitly historical Gmail MVP material or Gmail preset instructions, not claims that the backend is Gmail-only.

### Commit 6 — final verification fixes, only if necessary

Do not create a cleanup commit by default. If final validation exposes task-owned formatting, test, or documentation defects, fix them in the owning commit before push when history has not been published. If history is already published, use one narrow commit with a subject naming the actual correction; do not rewrite published history.

---

## Automated test matrix

### SMTP integration

- Configured host and port are used for both verify and send.
- Context cancellation and deadlines close the connection promptly.
- STARTTLS succeeds when advertised.
- STARTTLS mode fails when the extension is absent.
- STARTTLS rejection never falls back to plaintext.
- Implicit TLS starts with a TLS handshake and verifies `ServerName`.
- A bad or wrong-host certificate fails.
- Plaintext/no-auth sends no AUTH command.
- PLAIN carries exactly the configured username/password after TLS.
- LOGIN responds to the username/password challenges in order after TLS.
- Authentication rejection includes the SMTP reply but not the password.
- Every failure path closes the connection.
- Existing message building, CRLF protection, multipart boundaries, and envelope behavior remain covered.

### Email service

- Host required and syntactically valid.
- Port below 1 or above 65535 rejected.
- Unknown TLS/authentication values rejected.
- Plaintext plus authentication rejected before dialing.
- Authenticated mode requires username and a complete password.
- Sender address remains independent from username.
- First save verifies then persists.
- Failed verification performs zero writes.
- Failed edit retains the old working record.
- Omitted password on edit reuses the saved password for verification and persistence.
- Explicit empty password is rejected for authenticated mode.
- Switching to no authentication clears credentials.
- Public settings expose `passwordConfigured`, never the password.
- Missing record remains a valid unconfigured state.
- Test mail and Mailer use the saved endpoint/configuration without exposing it to callers.

### File store

- Absence returns `(nil, nil)`.
- Version 2 round-trip preserves every field.
- File mode remains `0600`; directory remains `0700`.
- Writes remain temp-file plus rename under the mutex.
- Delete remains idempotent.
- Unknown/malformed record returns an error without secret text.
- Canceled context performs no write or delete.

### HTTP handler

- 401 without a session and 403 for a non-admin.
- GET unconfigured response has no configuration object.
- GET configured response contains every public field and no password.
- PUT accepts all generic fields and optional password.
- Invalid combinations return 400 and never call verification/storage.
- Verification errors return 502 and preserve prior state.
- Test send keeps current 400/409/502 distinctions.
- DELETE returns 204 when configured and already absent.
- Unsupported methods return 405.

### Frontend

- Gmail is marked Recommended.
- Selecting Gmail populates values exclusively from the preset catalog.
- Gmail app-password spacing is removed and 16 characters are required.
- Custom SMTP does not strip password whitespace or require 16 characters.
- Custom host, port, TLS, authentication, username, and sender address are submitted unchanged after generic normalization.
- No-auth hides credentials and submits no password.
- Plaintext forces/no-auth and displays the security explanation.
- Existing public values matching Gmail select Gmail on load; any mismatch selects Custom.
- A configured password is represented only by `passwordConfigured` and an empty input.
- Empty password on edit is omitted so the backend retains it.
- Save success clears the in-memory password.
- API/controller failures preserve editable values and show the error.
- Test-send and remove flows continue to work.
- Non-admin users still see only the administrator notice.

---

## Full verification gate

Run from the repository root after all task commits:

```bash
test -z "$(gofmt -l backend)"

cd backend
go vet ./...
go test ./...
go build ./...

cd ../frontend
npm test
npm run build

cd ..
git status --short
git diff --check
```

Review requirements:

- Inspect every changed file and staged diff as a maintainer.
- Confirm the original unrelated work is unchanged.
- Confirm no temporary adapter, old Gmail model, duplicate DTO, dead field, or obsolete comment remains.
- Confirm `integration/smtp` imports no service package.
- Confirm services import no `net/smtp`, `crypto/tls`, HTTP request, or persistence-record type.
- Confirm the handler contains delivery mapping only.
- Confirm frontend components contain no duplicated Gmail host/port constants.
- Confirm no real SMTP address, username, password, QA credential, or `.qa.env` content is tracked.
- Confirm every commit has the configured user's DCO sign-off and no co-author trailer.

---

## Manual acceptance

Use credentials only from the operator's environment or interactive entry. Never write them to tracked files or this plan.

1. Open Settings → Email as an administrator; Gmail is selected for a fresh form and visibly marked Recommended.
2. Enter a Gmail/Google Workspace sender and a spaced app password; save succeeds and test mail arrives.
3. Confirm GET and browser state never contain the app password after save.
4. Edit a non-secret Gmail field with the password left blank; the existing secret is retained and reverified.
5. Enter a wrong replacement password; the save fails and the previous working configuration still sends test mail.
6. Select Custom SMTP and configure a STARTTLS server with a non-16-character password; save and test succeed.
7. Configure an implicit-TLS server; save and test succeed.
8. Configure a trusted no-auth relay; save and test succeed without an AUTH command.
9. Attempt plaintext plus credentials; the UI prevents it and a direct API request returns 400.
10. Attempt STARTTLS against a server that does not advertise it; save returns a verification failure and nothing is persisted.
11. Attempt TLS with an invalid certificate; save fails without a bypass option.
12. Verify sender address and SMTP username may differ.
13. Delete settings twice; both calls return 204 and GET reports unconfigured.
14. Sign in as a member; no SMTP fields or secrets are exposed.
15. Exercise one existing real Mailer consumer or the test-mail path and confirm message rendering and delivery behavior are unchanged.

For secret containment, place the test password in a shell variable and search exact values only:

```bash
journalctl -u remote.futrx.service --since '-15 min' | grep -cF "$SMTP_TEST_PASSWORD"
git diff -- . ':!docs/plans/2026-09-12-provider-neutral-smtp-refactor.md' | grep -cF "$SMTP_TEST_PASSWORD"
```

Both counts must be zero. Do not print the variable itself.

---

## QA and deployment verification

This changes application backend/frontend code and persisted application data, but not host dependencies, Caddy, systemd, LXD, or workspace images. Use the app deployment workflow rather than installer/update QA unless implementation expands into `infra/`.

Before QA:

1. Commit with the configured user identity and DCO sign-off.
2. Push the exact candidate ref only when authorized.
3. Confirm the ref is checked out locally.
4. Confirm the tracked working tree is clean.
5. Confirm the QA VM provider identity before changing any stale `known_hosts` entry.

Run:

```bash
bash infra/qa/deploy-app.sh <ref>
```

Then repeat the Gmail, custom STARTTLS, implicit TLS, failed-verification-preserves-old-config, secret-containment, test-send, and delete acceptance checks against QA.

If any installer/updater/runtime-service template is changed during implementation, stop and expand QA scope according to `AGENTS.md`: installer changes require a rebuilt VM and `install.sh <ref>`; a major/minor release or infrastructure/base-image change requires `update.sh <ref>` against an existing installation.

---

## Acceptance criteria

The PR is ready only when all of the following are true:

- The backend contains no Gmail host, port, password-length, provider ID, or Gmail setup text.
- Gmail SMTP defaults have exactly one owner: `frontend/src/config/constants/email-providers.ts`.
- An administrator can save and test Gmail through the recommended frontend preset.
- An administrator can save and test a custom STARTTLS server.
- An administrator can save and test a custom implicit-TLS server.
- A trusted unauthenticated relay is supported without allowing plaintext credentials.
- Sender address and authentication username are independently configurable.
- Verification always happens before persistence.
- A failed edit cannot replace a working configuration.
- Update without a password safely retains the stored password.
- No API response or log contains the SMTP password.
- Public models and behavioral ports use the required singular bounded-context roots.
- Backend runtime constants are owned by `config/constants/`; dependency construction remains in composition.
- Existing Mailer behavior and all existing email/message tests remain green.
- Backend vet/test/build and frontend test/build pass.
- QA `deploy-app.sh <ref>` and manual Gmail/custom SMTP acceptance pass on the pushed immutable ref.
- The final status report lists every commit hash, subject, verification performed, and any remaining uncommitted paths.

---

## Deliberately not done

- No Gmail constant is retained in the backend for convenience or migration.
- No provider registry is added to the backend; the backend supports SMTP capabilities, not branded providers.
- No generic plugin/provider framework is introduced for one transport.
- No certificate verification bypass is offered.
- No unrelated models, stores, handlers, or settings screens are reorganized.
- No existing dirty-worktree changes are staged, edited, committed, or discarded.
- No push, PR mutation, history rewrite, or hook bypass is authorized by this plan alone.
