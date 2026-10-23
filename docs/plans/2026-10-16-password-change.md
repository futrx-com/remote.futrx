# Change password and sign out other sessions

**Issue:** [#374](https://github.com/futrx-com/remote.futrx/issues/374)
**Status:** in-progress — implementation complete; required live QA deployment/acceptance blocked by SSH timeout (2026-10-16)
**Tier:** structural, small. It adds one API route, one field on an existing persisted record, and one store method. New files within existing packages only; no new package, dependency or `infra/` change, so it is a PATCH-class release.
**Blocks:** [#372](https://github.com/futrx-com/remote.futrx/issues/372). Plan: `docs/plans/2026-10-16-email-invitations.md`. It reuses the change rules and session revocation built here.

**Scope (one sentence):** An account that signs in with a password can change it from Settings → Security after proving the current password, plus 2FA if enabled. Every other session of that account then stops working, while the browser that made the change stays signed in.

**Out of scope:** forgot-password by email; administrator-initiated reset (#372); invited password accounts (#372, which extends this flow to them); changes to Google OAuth; a "sign out everywhere" button that doesn't change the password; encrypting `DATA_DIR` at rest.

---

## Requirements record

### Confirmed decisions (requester, 2026-10-16)

| # | Decision |
| --- | --- |
| D1 | Self-service password change applies to **every account that has a password**: the local administrator now, and invited accounts once #372 lands. |
| D2 | The user must enter the **current password**. If 2FA is enabled, they must **also** enter an authenticator code or recovery code, the same as disabling 2FA today. |
| D3 | A password change **signs out every other session** of the account. The browser that made the change gets a fresh session. |
| D4 | The password rule is unchanged: at least 12 Unicode characters and at most 1024 bytes, no composition rules. The new password must differ from the current one. |
| D5 | It ships as its own PR, ahead of #372. |

### Evidence about existing behaviour (unchanged unless listed above)

- Sessions are stateless HMAC tokens `{email, sub, iat, exp, sid}` (`service/auth/model.go` `Session`). `iat` is in **seconds**.
- `CurrentSession` (`service/auth/service.go:354`) is the single place every gate checks a session: the API middleware, `/auth/verify` and the WebSocket upgrades all go through `PrincipalResolver`. It already calls `sessionRegistry.IsActive` (`session_registry.go:164`), which reads a lazily cached per-account record. So adding a check there costs no extra disk read per request.
- `IsActive` **fails open** on a store read error (`session_registry.go:166`). This plan keeps that behaviour (see Risks R3).
- The local admin credential is created with `CreateLocalAdmin`, which refuses to overwrite, and removed with a compare-and-delete `DeleteLocalAdmin` (`stores/fileauth/store.go:255-300`). Nothing can replace it today.
- 2FA proof of possession with replay protection and recovery-code consumption is `twoFactorAuthenticator.VerifyChallenge` (`service/auth/twofactor.go:164`).
- The local-login limiter (`handlers/auth_local_handler.go:113-160`) allows 5 failures per key per 5 minutes.
- There are **no** handler tests for `security_handler.go`.

---

## Design

### Where each responsibility goes

| Responsibility | Owner | Why there |
| --- | --- | --- |
| "Is this session still valid?" incl. revoke-before-time | `sessionRegistry` (extend) | It already owns per-account session validity (`IsActive`, `Revoke`), its cache and its per-account lock. A new store would duplicate the cache and lock. |
| Verifying and replacing the local admin password | `LocalAdminAuthenticator` (extend) | It owns the credential, its in-memory copy and its claim lock. The change must update the store and the in-memory copy together. |
| Sequencing (verify → 2FA → revoke → save → reissue) | `auth.Service.ChangePassword` (new facade method) | It follows the facade pattern of `CompleteTwoFactorDisable` (`security.go:87`), which composes the authenticators. |
| HTTP: body, rate limit, cookie, error mapping | `SecurityHandler` (`/api/me/security/*`) | It already owns the signed-in user's own security routes. |
| Atomic replace on disk | `fileauth.Store.ReplaceLocalAdmin` (new) | Compare-and-swap, mirroring `DeleteLocalAdmin`. |
| Form rules (length, confirmation, differs) | `frontend/src/state/hooks/auth/passwordChangeFormState.ts` (new, pure) | Same shape as `localAuthFormState.ts`. Keeps components free of policy and can be tested with `node --test`. |

No new interfaces, packages or layers. `LocalAdminStore` gains one method. That is the smallest contract change that lets the change be atomic.

### Behaviour

`POST /api/me/security/password` with body `{currentPassword, newPassword, code?}`, behind a session:

1. The handler resolves the session (401 if none) and rate-limits on the key `<clientIP>|<email>|password-change`. It allows 5 failures per 5 minutes and returns 429 with `Retry-After` when exceeded, using the same limiter type as login.
2. `auth.Service.ChangePassword(ctx, user, PasswordChange{Current, New, Code}, ip, ua)` holds the local authenticator claim lock across verification, revocation, replacement and reissue, so a concurrent loser cannot revoke the winner’s fresh session:
   1. The account has no password credential (a Google-only account) → `ErrPasswordSignInUnavailable` → **409**.
   2. Verify `Current` against the stored hash. A mismatch gives `ErrCurrentPasswordIncorrect` → **403**, which counts as a failure. It must not be 401, because `frontend/src/api/apiRequest.ts:12` treats any 401 as "signed out" and would drop the user to the login screen. The password is checked before the 2FA code, so the 2FA state of the account is revealed only to someone who already knows the password.
   3. Load 2FA state with error propagation (a read failure returns 500 `password not changed`, never treating 2FA as disabled). If 2FA is enabled: an empty `Code` gives `ErrTwoFactorCodeRequired` → **400**. Otherwise call `VerifyChallenge`. An invalid or replayed proof gives **403** and counts as a failure; persistence errors give **500** `password not changed`. A recovery code used here is consumed, the same as when disabling 2FA.
   4. `New == Current` → `ErrPasswordReuse` → **400**. `HashPassword(New)` returns the existing too-short or too-long errors → **400**.
   5. **Revoke first:** `registry.RevokeSessionsBefore(email, now)` sets `SessionsValidAfter = now.Unix()`. On failure, return **500** "password not changed". Nothing else has happened yet.
   6. **Then save:** `local.replacePassword(expected, newHash)` does a compare-and-swap on disk, then updates the in-memory copy. If a concurrent change won, return `ErrLocalAdminCredentialChanged` → **409**. On another failure → **500** "password not changed; your other sessions were signed out". That is true and safe: the old password still works and the user can sign in again.
   7. **Then reissue:** `IssueSession(user, SignInMethodPassword, ip, ua)`. Its `iat` is at least the cut-off, so it passes. If reissue fails after replacement, return **500** `password changed; sign in again with your new password`, never claiming the password is unchanged.
3. The handler sets `remote_session` and returns **200** with `Status`.

`CurrentSession` → `registry.IsActive(ctx, *session) error` (nil means active; separate sentinels distinguish revoked and superseded sessions). Cold registry loads must recheck the cache under its write lock before publishing their result, so a stale load cannot overwrite a newer revocation. The order is: record missing → active; `session.Iat < record.SessionsValidAfter` → `ErrSessionRevoked`; then the existing single-session rule. Accounts whose record has `SessionsValidAfter == 0` behave exactly as today.

`SecuritySummary` gains `passwordSignIn bool`, which is true when the account has a password credential. The frontend shows the card only when it is true.

### Error contract

| Condition | Status | Message (safe, no secrets) |
| --- | --- | --- |
| No session | 401 | `authentication required` |
| Google-only account | 409 | `this account does not sign in with a password` |
| Wrong current password | 403 | `current password is incorrect` |
| 2FA code missing | 400 | `two-factor code required` |
| 2FA code wrong | 403 | `invalid two-factor code` |
| Same as current / too short / too long | 400 | existing or new sentinel text |
| Rate limited | 429 | `too many attempts; try again in a few minutes` |
| Concurrent change | 409 | `password was changed elsewhere; reload and try again` |
| Revoke failed | 500 | `password not changed` |
| Save failed after revoke | 500 | `password not changed; other sessions were signed out` |
| 2FA load/save failed before revoke | 500 | `password not changed` |
| Reissue failed after save | 500 | `password changed; sign in again with your new password` |

---

## Steps

Every backend step ends with `gofmt -l backend` (empty), then `cd backend && go vet ./... && go test ./internal/...`. Frontend steps end with `cd frontend && npm test && npm run build`. Commit after each step with `git commit -s`, and with **no** co-author trailer.

0. **Characterization tests (no behaviour change).** Add `handlers/security_handler_test.go` covering what the change flow relies on: 401 without a session on `/api/me/security`, and the 2FA disable happy and wrong-code paths. Add `session_registry_test.go` cases pinning today's `IsActive` behaviour: no record → true; single-session off → true; superseded sid → false; store error → true.
   Verify: the new tests pass on the unchanged code.

1. **Session registry: revoke-before.** Files: `service/auth/model.go` (`SessionRegistryRecord.SessionsValidAfter int64 \`json:"sessionsValidAfter,omitempty"\``, `ErrSessionRevoked`), `session_registry.go` (`RevokeSessionsBefore`, `IsActive(ctx, Session)`), `service.go` (`CurrentSession` call site).
   `RevokeSessionsBefore` takes the account lock, loads or creates the record (keeping preferences, history and alert), sets the field only if the new value is larger, saves, then updates the cache.
   Verify: step-0 tests still pass. New cases: a session with `iat` before the cut-off gives `ErrSessionRevoked`; `iat` equal to it is accepted; a record created only by revocation leaves `Preferences` all false, so `IssueSession` still skips the registry; logout `Revoke` and `SetPreferences` keep `SessionsValidAfter`; a controlled cold-load/revoke race cannot restore stale cached data.

2. **Store: replace the local admin credential.** Files: `service/auth/ports.go` (`ReplaceLocalAdmin(ctx, expected, next LocalAdminCredential) error` on `LocalAdminStore`), `stores/fileauth/store.go` + test, and every fake implementing `LocalAdminStore` (`grep -rn "DeleteLocalAdmin(" backend --include=*_test.go`).
   Use the same normalization and incomplete-credential checks as `DeleteLocalAdmin`. If the file is missing or doesn't match `expected`, return `ErrLocalAdminCredentialChanged`. Write with `writeJSONLocked` (0600, temp + rename).
   Verify: success; mismatch → sentinel and the file is unchanged byte for byte; missing → sentinel; mode stays 0600.

3. **Authenticator + service.** Files: `twofactor.go` (protect cold-load publication against concurrently completed enrollment), `local_admin_authenticator.go` (`verifyPassword(email, password) (credential, error)` and `replacePassword(ctx, expected, hash)` with the workflow holding `claimMu`), `password_change.go` (new: `PasswordChange`, `ChangePassword`, sentinels `ErrPasswordSignInUnavailable`, `ErrCurrentPasswordIncorrect`, `ErrTwoFactorCodeRequired`, `ErrPasswordReuse`) + `password_change_test.go`, and `security.go` (`SecuritySummary.PasswordSignIn`).
   Verify, table-driven, with fakes that record call order:
   - each error row above;
   - the exact order revoke → save → reissue;
   - a failed revoke means **no** save;
   - a failed save after revoke returns the specific sentinel;
   - the old password fails and the new one works through `CompletePasswordLogin`;
   - a session issued before the change gets `ErrSessionRevoked` from `CurrentSession`, and the reissued one passes;
   - a recovery code is consumed;
   - 2FA load/save failures never skip proof or revoke sessions;
   - failed reissue returns the changed-password sentinel; concurrent changes with the same current password have exactly one winner and keep its fresh session valid;
   - a capturing `slog` handler and every error string contain neither password.

4. **HTTP.** Files: `handlers/security_handler.go` (route, limiter field, error mapping), `security_handler_test.go`, `NewSecurityHandler` (construct a dedicated `newLocalLoginLimiter()` internally, matching existing handler ownership; the helper is unexported).
   Verify: each status in the error contract through `httptest`; a successful change sets `remote_session` with the same attributes as `setSessionCookie`; the old cookie then gets 401 on `/api/projects` through the real middleware; `/api/me/security` returns `passwordSignIn: true` for the local admin and `false` for a Google user.

5. **Frontend.** Files: `config/auth.ts` (central maximum-byte rule alongside existing minimum), `config/routes.ts`, `api/securityApi.ts` (`changePassword`), `models/security` type (`passwordSignIn`), `state/hooks/auth/passwordChangeFormState.ts` + `.test.ts`, `state/hooks/auth/useSecuritySettings.ts` (expose `changePassword`), `ui/settings/security/PasswordChangeForm.tsx` (new), `ui/settings/SecuritySettings.tsx` (render when `passwordSignIn`).
   The form has current password, new password, confirmation, and a code field shown only when `twoFactorEnabled`. Use `autocomplete` values `current-password`, `new-password`, `one-time-code`. Show success text after the change. Errors show the server message. Clear the fields on success and on unmount.
   Verify: form-state tests cover length < 12, mismatch, same-as-current, and the code required when 2FA is on; `npm run build`.

6. **Docs.** `ARCHITECTURE.md` "Sessions" paragraph (`per-request IsRegistered checks are the only way a session is invalidated early` → add password change), `docs/02-workspaces/02-auth-users-and-access.md`, `docs/03-platform/08-api-and-realtime.md` (new route), `docs/threat-model.md` (stolen-session window now closes on password change), and the README user guide (Settings → Security).
   Verify: `grep -rn "sessionsValidAfter\|me/security/password" ARCHITECTURE.md docs/` hits each file.

7. **Full verification.** `gofmt -l backend`, `cd backend && go vet ./... && go test ./...`, `go test ./...` at the repo root, `cd frontend && npm test && npm run build`. Then `bash infra/qa/deploy-app.sh <sha>` (no infra change, per `AGENTS.md`) and the acceptance criteria below on QA.

## Data / migrations

- The session registry record (`DATA_DIR/sessions/`, `stores/filesessions`) gains an optional `sessionsValidAfter`. A missing value means 0, which means no revocation. Old records stay valid. An older binary ignores the field. On rollback, revoked sessions become valid again until they expire. Record this in the release note.
- `local-admin.json` keeps its shape. It is replaced atomically.

## Risks

| # | Risk | Mitigation |
| --- | --- | --- |
| R1 | `iat` has 1-second resolution. A session issued in the same second as the change, before it, survives. | Accepted and documented. This includes stolen existing cookies issued in that same second, regardless of how the attacker obtained them. |
| R2 | A revoke is written but the save fails, so the user is signed out everywhere and the password is unchanged. | Safe failure, and the message says so. |
| R3 | `IsActive` fails open on a registry read error, so a revoked session could pass while the disk errors. | This existing behaviour is frozen here and noted in the threat model. Changing it would affect every request and needs its own decision. |
| R4 | Using a recovery code to change the password doesn't raise the recovery-code alert. | Out of scope. Noted as a follow-up. |

## Acceptance criteria (Given / When / Then)

1. **Given** the local admin is signed in on browsers A and B, **when** they change the password on A, **then** A stays signed in and B's next request returns to the login screen.
2. **Given** 2FA is on, **when** the code is missing or wrong, **then** nothing changes and the error is shown.
3. **Given** a wrong current password five times, **then** the sixth attempt returns 429.
4. **Given** a successful change, **then** the old password fails at login and the new one works.
5. **Given** a Google-only member, **then** Settings → Security shows no password card, and the API returns 409.
6. **Given** any attempt, **then** `journalctl -u remote.futrx` and `DATA_DIR` contain neither password in plain text.
7. **Given** an account that never changed its password, **then** the sign-in flow, single-session and history behave as before.

## Verification gate

Read-only delegated preflight checked the reference implementations and all step verification commands at base `37672a53`. Requester approved the following design amendments: error-propagating 2FA loading, truthful post-save errors, serialized password-change workflows, and protection against stale registry cache publication. The IsActive error result is necessary to express the specified sentinels. No dependency or infrastructure changes are needed. Preserve existing fail-open registry reads, the accepted same-second window, and current login behavior.

**Baseline:** backend `go vet ./... && go test ./...`, root `go test ./...`, frontend `npm test` (101 test-file results under the initial runner; independently comparable Node 24 baseline: 555 cases) and `npm run build` passed. Backend tests required local sockets unavailable under the initial sandbox; rerunning outside it passed. Existing Vite configuration/chunk-size warnings remain.

**Additional follow-up:** pending 2FA login tokens issued before a change can still complete afterward; invalidating pending login proofs is outside this session-cookie scope.

## Progress

- [x] 0. Characterization tests — `verified`: new HTTP and registry tests pass unchanged; gofmt empty, backend vet and internal suite pass.
- [x] 1. Session registry — `verified`: cutoff boundaries, monotonic preservation, failure paths, untracked issuance and controlled stale-load races pass; focused race tests ×10, backend vet/internal suite, gofmt pass.
- [x] 2. Credential store — `verified`: normalized CAS success, missing/mismatch rejection with byte preservation, 0600 mode, cancellation and incomplete input tests pass; backend vet/internal suite and formatting pass.
- [x] 3. Authenticator and service — `verified`: ordered success/error matrix, durable/cache login consistency, revocation, recovery/TOTP/replay, sanitized logs/errors and concurrent winner tests pass; backend vet/internal suite and focused race tests ×2 pass. Independent review found and verified the fix for stale 2FA cold-load publication.
- [x] 4. HTTP — `verified`: full status matrix, limiter/reset/isolation, cookie attributes, middleware old401/fresh200, Google409, recovery use, real-store restart and plaintext scans pass; backend vet/internal suite and HTTP race check pass.
- [x] 5. Frontend — `verified`: 561 frontend cases and production build pass; real isolated Chrome checks validation/proof/exact payload, double submission, success/error, detached-node unmount clearing, visibility gate and mobile width. Screenshot inspected; independent source review passed.
- [x] 6. Documentation — `verified`: architecture/auth/API/threat-model references contain sessionsValidAfter or the new route; README user guide updated, rollback and residual limits documented, diff check clean.
- [ ] 7. Full verification and QA deployment — `blocked`: all local full suites, focused race checks, browser checks and independent adversarial checks passed. The pushed, locally checked-out, tracked-clean full candidate SHA passed deploy ref validation, then SSH timed out before remote mutation; live acceptance/journal/public health remain unverified. See QA Round 1.

## Outcome

**Deviations from the plan:** The requester approved error-propagating 2FA loading, truthful post-save session errors, serialization of the password-change workflow and safe cold-cache publication. Independent review found the same stale cold-load race in 2FA enrollment state; its cache now uses the same recheck, with a deterministic regression test. The dedicated HTTP limiter is constructed inside `NewSecurityHandler` because `newLocalLoginLimiter` is private to that package, so transport composition needed no edit. Focused backend tests use additional test files within existing packages; the frontend maximum-byte constant lives alongside its existing minimum in `config/auth.ts`.

**Plan defects:** Initial preflight lacked a recorded verification gate, omitted post-save renewal failure, assumed a boolean session check could express separate error sentinels, and described the password maximum as characters rather than the existing 1024-byte rule. Those details were corrected before the affected implementation. No new dependencies, interfaces, packages, generated/lock edits or infrastructure changes were introduced.

**Steps not fully verified:** Step 7 is locally verified but live QA is blocked. `bash infra/qa/deploy-app.sh d4dd36533c114820b4a4b84d57ecd8e186bd6fac` validated the pushed clean candidate then exited 255 because the configured QA host's SSH port timed out. Independent read-only strict SSH verification confirmed the timeout. No deployment was made and no host-key protections were weakened. Restore QA connectivity and run the exact pushed candidate plus acceptance 1–7 before release. The final report commit changes this plan only; the independently tested source is candidate `d4dd36533c114820b4a4b84d57ecd8e186bd6fac`.

**Left behind:** Accepted same-second token survival, fail-open registry reads, pending 2FA proof survival, existing WebSocket connections and recovery-code alert behavior are documented follow-ups. Unrelated pre-existing untracked `.agents/` content was preserved and excluded from commits. No release/tag or infrastructure update was performed.

**For the next plan in this area:** Keep cache publication safe against concurrent mutations, distinguish every partial-write outcome, and test persisted credentials and cutoffs after service restart. The deployment wrapper fetches remote refs and requires the full SHA rather than an abbreviated commit ID. Keep live QA status separate from passing local tests.

## QA

### Round 1 — 2026-10-16

**Verdict:** blocked — all local checks passed and no feature defect found; the required deployment and live acceptance remain blocked by QA connectivity.
**Independence:** delegated — this reviewer did not implement the feature or fixes.
**Tested at tier:** structural, small.
**Candidate:** `d4dd36533c114820b4a4b84d57ecd8e186bd6fac`; reviewed the entire diff from base `37672a53e3594b50dce269a12aff07faffad1278`.
**Baseline:** independently verified in `/tmp/password-change-qa-base-37672a53`: backend `go vet ./... && go test ./...`, root `go test ./...`, frontend `npm test` and `npm run build` all exited 0. Node `v24.20.0`, npm `11.19.1`, Go `1.26.8`; baseline frontend reported 555 test cases, candidate 561, all passed. The earlier 101 figure used a different Node runner/reporting environment and is not the comparable case count. Existing Vite configuration and chunk-size warnings occur at baseline and candidate.

**Ran:** independently repeated the same full suites on the candidate (all exit 0), `gofmt -l backend` (empty), `git diff --check` (empty), and `go test -race ./internal/service/auth ./internal/transport/http/handlers ./internal/stores/fileauth -run 'PasswordChange|ChangePassword|Revocation|ColdLoad|ReplaceLocalAdmin|SecurityDisableTwoFactor|IsActiveExistingBehavior' -count=2 -v` (pass). Independently ran `/tmp/password-change-ui-harness.mjs` against the actual components in headless Chrome; every assertion passed and the mobile screenshot was inspected. Additional QA-only tests ran in `/tmp/password-change-qa-candidate`, leaving repository source/tests untouched.

#### Acceptance results

1. **Local pass; live blocked.** `TestPasswordChangeHTTPSuccessAndDurability` exercises the real file stores and auth middleware: historical cookie accepted before change, old cookie 401 afterward, replacement cookie 200, same result after service restart. The Chrome harness verifies success and form clearing. Existing `requestJson` reloads on 401; two actual browser sessions against QA could not be exercised.
2. **Local pass; live blocked.** Service and HTTP missing/wrong/replayed 2FA cases return the specified 400/403 without replacing the password or revoking sessions; recovery-code and TOTP success/replay cases pass. Chrome verifies the required code field and visible server error.
3. **Local pass; live blocked.** `TestPasswordChangeHTTPRateLimit` observes five 403 failures followed by 429 and `Retry-After: 300`. Separate tests pass for invalid-2FA counting, independent IP buckets, validation not counting, successful reset, and separation from login limiting.
4. **Local pass; live blocked.** Service and real-store HTTP/restart tests reject the old password and complete login with the replacement password.
5. **Local pass; live blocked.** Google-only summary reports `passwordSignIn: false`, API returns 409, and Chrome asserts the Password card is absent while other Security settings render.
6. **Partial local pass; live blocked.** Capturing-log/error tests contain neither fabricated password; real-store success recursively scans temporary DATA_DIR files and finds neither plaintext. No new flow logs password values. Live service journal and deployed DATA_DIR were inaccessible; this criterion is not claimed passed on QA.
7. **Local pass; live blocked.** Full regression suites and existing no-record/single-session/history tests pass against the verified base. Revocation-only state leaves tracking off; preference/logout updates preserve the cutoff.

#### Adversarial and seam coverage

- Empty/missing/null/wrong-type bodies, malformed JSON, absent session and wrong HTTP method pass. Independent tests also assert invalid input performs no registry write and preserves durable login.
- Independent HTTP tests reject 11 supplementary Unicode characters, accept 12, accept exactly 1024 ASCII/UTF-8 bytes, reject 1028 bytes, and preserve a 12-space password through durable login. Frontend tests cover corresponding Unicode/byte and confirmation boundaries.
- Authorization/account isolation passes for no session, Google-only accounts, ignored body identity and another account's unaffected cookie. Independent forward-auth test observes old-cookie 200 before change, redirect 302 after change, and replacement-cookie 200.
- Repetition/concurrency, proof replay, one-winner changes, cold-cache races, dependency read/write failures, revoke-before-save ordering, failed save and failed reissue all pass under the focused race run. UI double-submit and detached-node credential clearing pass.
- Pre-existing missing-cutoff records retain prior behavior. The persisted change is additive with no migration/down command; independent legacy-shape decoding accepts the field and demonstrates the documented loss of revocation on older readers. An actual older-binary deployment rollback was not run.
- Volume: no bulk query/fetch or per-session scan was introduced; the changed request handles one credential and one bounded account record. Existing bounded-history tests pass. Load benchmarking was not performed.
- Caller search checked the changed `IsActive` interface and `LocalAdminStore` implementation/fake coverage. `CurrentSession` remains the shared gate for API, forward-auth and new WebSocket authentication; an actual WebSocket upgrade after a change was not separately exercised.
- All changed hunks map to the plan or its recorded amendments; no new dependency, infrastructure change, migration, debug scaffolding, secret, or unexplained generated-file change was found. Existing fail-open reads and same-second survival are preserved and documented.

#### Findings

No blocker, major or minor feature defect found locally. The deployment gate remains open: the implementer reports `deploy-app.sh` against the full pushed candidate SHA exited 255 at SSH before mutation. This reviewer independently ran a read-only SSH `true` using the configured QA connection and strict host checking: exit 255, `Connection timed out`. Credentials, host details and `.qa.env` values were not added to this report.

#### Not tested

- Required QA deployment, live two-browser acceptance, public health, `journalctl -u remote.futrx`, and deployed DATA_DIR plaintext checks. Restore connectivity to the configured QA host, deploy the exact pushed clean candidate using `infra/qa/deploy-app.sh`, then run acceptance 1–7 there before release.
- Existing WebSocket connections remaining open, pending 2FA login proof survival, fail-open registry errors and same-second token survival remain the accepted documented limitations, not newly claimed protections.

#### Evidence and next check

Independent logs: `/tmp/password-change-qa-base-{backend,root,frontend,build}.log`, `/tmp/password-change-qa-candidate-{backend,root,frontend,build}.log`, `/tmp/password-change-qa-focused.log`, `/tmp/password-change-qa-adversarial.log`, `/tmp/password-change-qa-ui.log`; screenshot `/tmp/password-change-ui.png`. QA-only adversarial tests are in `/tmp/password-change-qa-candidate/backend/internal/transport/http/handlers/password_change_independent_qa_test.go`.

Before shipping, prioritize the required live two-browser session-revocation and journal/DATA_DIR checks after QA connectivity is restored. A future plan should retain the distinction between local evidence and a completed deployment gate.
