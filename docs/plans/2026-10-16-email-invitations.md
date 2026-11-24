# Email invitations with temporary credentials

**Issue:** [#372](https://github.com/futrx-com/remote.futrx/issues/372)
**Status:** ready-for-implementation (requirements confirmed with the requester on 2026-10-16; see *Requirements record*)
**Depends on:** [#374](https://github.com/futrx-com/remote.futrx/issues/374) / PR #375, plan `docs/plans/2026-10-16-password-change.md`. This plan reuses its session revocation (`RevokeSessionsBefore`, `ErrSessionRevoked`), its change rules, and `ChangePassword`. Start implementing only after #375 merges into `qa`, and rebase this branch onto it.
**Tier:** structural. One new service package, one new `DATA_DIR` file, new routes, and a new first-login step. No dependency and no `infra/` change, so it is a PATCH-class release.


**Scope (one sentence):** An administrator creates an account from an email address and an existing role. Remote issues a temporary password, which is emailed when SMTP is configured and the admin leaves "email it" ticked, or shown with copy and PDF download otherwise. The account cannot get a session until its owner sets a new password.

**Out of scope:** forgot-password by email; new roles; changes to the local-admin claim, the setup token, the Google OAuth configuration, or 2FA enrolment; encrypting `DATA_DIR` at rest; magic-link sign-in; fixing the pre-existing Google re-invite session issue (see *Deliberately not done*).

---

## Requirements record

### Confirmed decisions (requester, 2026-10-16)

| # | Decision | Consequence |
| --- | --- | --- |
| D1 | **Credentials can be retrieved in the dialog, and later via Reset.** Right after an invite, the admin can view, copy or download the credentials while the card is open. After it is closed, **Reset temporary password** issues a new one and invalidates the old one. Nothing recoverable is stored. | It satisfies "retrieve without another invitation" and "never persist plaintext" together. |
| D2 | **Google sign-in is also forced through the set-password step** while the account has a pending temporary password. | The Google callback needs a new branch. |
| D3 | **Temporary passwords do not expire.** They end only when changed or reset. | No TTL option. See Risks R1. |
| D4 | **Self-service password change for everyone.** This ships separately as #374, which this plan depends on. | Invited accounts get Settings → Security → Change password for free once `ChangePassword` knows the second credential source (step 4). |
| D5 | **A password change or an admin reset signs out every other session** of the account. | Reuses #374's `RevokeSessionsBefore`. |
| D6 | **Change needs the current password, plus 2FA if it is on.** | Handled by #374. Not relevant to the first-login step, since the user just proved the temporary password. |
| D7 | **Admins may give a password to a Google-only user** through Reset. | Reset works whether or not the user already has a credential. Because of D2, their next Google sign-in also goes through the set-password step. |
| D8 | **Non-ASCII email addresses:** invite and email work, the PDF button is disabled with a reason, and copy still works. | The PDF writer must refuse rather than garble the text. |
| D9 | **Two stacked PRs**: #374 / #375 first, then this one (#373). | — |
| D10 | **"Email the credentials" checkbox**, ticked by default when SMTP is configured. Unticked, the result looks like the no-SMTP case. | Adds a fourth delivery state, `not_requested`. |
| D11 | **Reset scope:** any other member or admin. An admin cannot reset their own account (they use Change password) and nobody can reset the local administrator. | Three guards. |
| D12 | **Password rule unchanged:** 12–1024 characters, must differ from the temporary password. | Reuses `HashPassword`. |

### Evidence about existing behaviour (frozen unless a decision above changes it)

- `POST /api/admin/users` refuses with 409 when Google OAuth is not configured (`transport/http/handlers/users_handler.go:53`). Invites are Google-only today.
- `user.Service.Add` is atomic against duplicates. The file store re-checks under its mutex (`stores/fileusers/store.go:135-150`).
- Password login is local-admin-only (`service/auth/local_admin_authenticator.go:158`). Its constant-time miss path uses a dummy hash.
- The login handler maps every login error to 401 `invalid email or password` (`handlers/auth_local_handler.go:101-105`).
- The Google callback already hands a pending step to the SPA through `?twoFactorRequired=1` plus a pending cookie (`handlers/auth_google_login_handler.go:95-96`). The SPA reads it in `useLocalAuthController.ts:41-44`.
- **Caddy strips only named cookies** before proxying to project containers: `remote_session|remote_2fa_pending|remote_oauth_state|return_to` (`infra/templates/Caddyfile.tmpl:91`). A new cookie scoped to the base domain would be sent into every user's container. That would be a leak, and fixing it would need an `infra/` change and a MINOR release.
- The SPA treats any 401 as signed out (`frontend/src/api/apiRequest.ts:12`).
- The PDF writer replaces every non-ASCII character with `-` (`frontend/src/services/auth/recoveryCodeFileService.ts:135-141`).
- `Mailer.Mail().ToUser(..).Required().Send(ctx)` reports `ErrNotConfigured` and delivery errors synchronously. `Mailer.Enabled(ctx)` reports configuration (`service/email/mailer.go:69,106`).
- There are **no tests** for `users_handler.go` or `security_handler.go`.

---

## Refactor findings (code-refactorer pass)

These are behaviour-preserving moves done **before** the feature commits, each in its own `refactor(...)` commit and each verified by existing or step-0 characterization tests. Every one earns its place by giving one owner to a responsibility this feature touches a second time.

| # | Smell → principle | Evidence | Move |
| --- | --- | --- | --- |
| R1 | Account-creation **policy in transport** (SRP, layering): the handler decides Google is required. | `users_handler.go:53-56` | Move the check into the new `invitation.Service.Invite` for the `google` method. The handler only maps errors. Same status and text. |
| R2 | **Sign-in method inferred from a magic `Sub` string** in three places (one owner per domain decision). | `local_admin_authenticator.go:202` (`"local-admin"`), `service.go:365`, `security.go:201-203` (anything not local-admin is treated as Google) | Add named subject constants and one `signInMethodForSubject` in `auth`. Behaviour stays identical for the two existing subjects. This is needed before a third subject (`"password"`) exists, otherwise `ReissueTrackedSession` would record invited password users as Google. |
| R3 | **Password verification belongs to one credential type** and has no seam for a second (OCP, for a demonstrated second variant). | `local_admin_authenticator.go:158-185` | Split it into "resolve the password credential for this email" (local admin, or dummy on a miss) and one `VerifyPassword`. Timing and results are unchanged. Step 4 then adds the invited source to the resolver. |
| R4 | **PDF document assembly locked inside a recovery-code service** (SRP, reuse). | `recoveryCodeFileService.ts:59-141` | Extract `services/files/pdfDocument.ts` (lines, content stream, xref, escaping). The recovery-code service delegates to it. `recoveryCodeFileService.test.ts` must pass unchanged. |
| R5 | **Four components in one 290-line file** that is about to gain a fifth (cohesion). | `ui/account/UsersPanel.tsx:23,94,164,206` | Move `AddUserForm`, `UserList` and `UserRow` into their own files under `ui/account/`, with no JSX change. Checked by `npm run build`. |

**Rejected:** splitting the `auth.Service` facade, which is large but out of scope; a generic "credential provider" interface package, since an unexported resolver inside `auth` is enough for two sources; a client-side PDF library (D8).

---

## Design

### Responsibilities

| Responsibility | Owner |
| --- | --- |
| Invited-account password credentials (hash, `mustChangePassword`, `version`) | `auth`: new unexported `accountPasswordAuthenticator` behind a new `AccountCredentialStore` port, with an in-memory cache loaded at startup |
| The file `DATA_DIR/account-credentials.json` (0600) | `stores/fileaccountcredentials` (new) |
| Login, first-login change, self-service change, reset | `auth.Service` facade methods (`CompletePasswordLogin`, `CompleteGoogleLogin`, new `CompleteInitialPasswordChange`, #374's `ChangePassword`, new `IssueTemporaryPassword`) |
| Orchestration of invite, reset **and removal**: users, credentials, revocation, mail, compensation, per-email lock | `service/invitation` (new). `DELETE /api/admin/users/{email}` moves from `users.Remove` to `invitation.Remove` (same lock, then `users.Remove`; same responses), so invite, reset and removal of one email never interleave. It depends on narrow ports (user adder/remover/getter, credential issuer, session revoker, mailer, base URL) and is wired in `service/services.go` |
| Temporary password generation | `auth.GenerateTemporaryPassword` (crypto/rand) |
| Removing the credential when a user is removed | `service/user_removal_cleanup.go` (extend) |
| HTTP mapping, cookies, `no-store` | `UsersHandler`, `localAuthHandler`, Google login handler, `auth_cookie.go` |
| Credential card, PDF, invite form, reset action | frontend `ui/account/*`, `services/users/invitationCredentialFileService.ts`, `state/hooks/users/*` |
| First-login step | frontend `ui/auth/InitialPasswordStep.tsx`, `useLocalAuthController.ts` |

**Why a new `service/invitation` package instead of extending `user` or `auth`:** inviting composes the user directory, credentials, session revocation and email. Putting it in `user` would make the directory change whenever mail wording or credential policy changes, and `auth` must not import `user` (`service/auth/service.go:12-14`). The package exists because there are two real use cases (invite and reset) with the same compensation and locking needs.

### Persisted record

`account-credentials.json` maps a normalized email to `{passwordHash, mustChangePassword, version, updatedAt}`. The file is absent until the first password invite. The hash format is the existing argon2id string. `version` grows by one on every set (invite, reset, first change, self-change). Pending change tokens carry it, which makes them single-use.

### Cookies

New cookie `remote_password_change`:
- **host-only** (no `Domain`), `Path=/auth/local/password`, `HttpOnly`, `Secure`, `SameSite=Strict`, `Max-Age` 15 minutes;
- value: `signedPayload{email, sub, method, version, exp}` signed with the session key, like `pendingLogin`.

Because it has no `Domain`, the browser never sends it to `*.dev.<host>` or application hosts, so the Caddy strip list does not need to change. Because of the `Path`, only the two endpoints that use it ever receive it. *Rejected:* a base-domain cookie, which leaks into containers or else needs an `infra/` change.

### Flows

**Invite:** `POST /api/admin/users {email, role, method: "google"|"password", sendEmail?: bool}`. If `method` is missing it means `"google"`, which keeps today's frontend and scripts working.
1. The handler checks `requireAdmin` and parses the body (`sendEmail` defaults to `true`), then calls `invitation.Invite`.
2. `Invite` takes the per-email lock, then:
   - `google`: Google must be configured (R1, same 409) → `users.Add` → return `{user}`. This is today's behaviour. As defence in depth, any credential left behind for that email is deleted, so a Google re-invite can never inherit an old temporary password.
   - `password`:
     1. `users.Add`. A duplicate gives `ErrUserExists` → 409, and nothing else happens.
     2. `auth.IssueTemporaryPassword(email)`: generate, hash, save `{mustChange: true, version+1}`, revoke sessions before now, return the plaintext. On failure → compensate with `users.Remove(email)` → 500 "account was not created". If compensation also fails → 500 that names both failures (`errors.Join`) and is logged with a masked email, so the admin sees the half-created user and can remove it.
     3. Delivery:
        - `sendEmail=false` → `not_requested`;
        - `!mailer.Enabled` → `not_configured`;
        - otherwise `Mail().ToUser(email).Required()…Send(ctx)` with a 20-second timeout → `sent`, or `failed` with a fixed safe `deliveryError` ("The SMTP server rejected the message", "Could not reach the SMTP server", or a generic message). Never the raw error text.
     4. Return `{user, method: "password", delivery, deliveryError?, temporaryPassword}` with `Cache-Control: no-store`. The status is 201 in every delivery state.

**Reset:** `POST /api/admin/users/{email}/temporary-password {sendEmail?: bool}`. The guards (D11) give 404 for an unknown user, 409 for your own account ("use Change password") and 409 for the local admin. Then it takes the same per-email lock and does `IssueTemporaryPassword` plus delivery as above. A Google-only user gains a credential (D7). The response has the same shape.

**Mail content:** heading "You've been invited to Remote" (on reset: "Your Remote password was reset"); a short text; `KeyValues` with the sign-in URL (`auth.BaseURL()`) and the email; `Code(temporaryPassword)`; `Button("Sign in", baseURL)`; and a `Note` saying "You'll be asked to choose a new password the first time you sign in. If you didn't expect this, ignore this email."

**Password login:** `POST /auth/local/login`. The local admin is unchanged.
1. R3 resolver: local admin credential, else an invited credential **for a registered user** (an orphan credential is ignored), else the dummy. Then **one** `VerifyPassword`.
2. If the credential is invited and `mustChange` → set the change cookie and return `200 {passwordChangeRequired: true}`. Otherwise run the normal `completeLogin`, where 2FA works as before. A wrong password gives the same 401 text as today.

**Google login with a pending change (D2):** in `CompleteGoogleLogin`, after authorization, if the account has an invited credential with `mustChange` → return `LoginResult{PasswordChangeRequired, Token}`. The callback sets the change cookie and redirects to `/?passwordChangeRequired=1`, mirroring the 2FA redirect.

**First-login change:** `POST /auth/local/password/initial {newPassword}`. It needs the change cookie, and is rate-limited per IP with the local-login limiter.
1. Verify the token signature, expiry, and that `version` matches the stored version. Otherwise `ErrInvalidPasswordChange` → **400** "this sign-in step expired, sign in again" (not 401, see Evidence). Clear the cookie.
2. The new password must differ from the temporary one (`ErrPasswordReuse`) and pass `HashPassword`.
3. Revoke sessions before now, then save `{mustChange: false, version+1}`. This is the same order as #374.
4. `completeLogin(user from the token, its original method)`. If the account has 2FA (possible for a reset Google user) → the 2FA pending cookie and the `{twoFactorRequired: true}` response. Otherwise → `remote_session` and `Status`.

`POST /auth/local/password/cancel` clears the cookie. "Back to sign-in" in the UI calls it.

**Self-service change for invited users:** #374's `ChangePassword` uses the R3 resolver for "has a password credential" and gets its compare-and-swap from the source that owns the credential. Step 4 extends the resolver and adds an invited-credential replace.

**Removal:** `invitation.Remove` takes the per-email lock, then `users.Remove`, whose cleanup deletes the credential. Combined with the revocation that every `IssueTemporaryPassword` does, a re-invited password user cannot bring back sessions from before the removal.

**Users list:** `GET /api/admin/users` items gain `passwordSignIn: bool` and `passwordChangePending: bool`. The handler joins `users.List` with `auth.PasswordStates(ctx)` (a map read from the cache). That is a presentation join only, with no policy in it.

### API summary

| Method & path | Auth | Request → Response |
| --- | --- | --- |
| `POST /api/admin/users` | admin | `{email, role, method?, sendEmail?}` → 201 `User` (google) or `InviteResult` (password); 400 / 409 |
| `POST /api/admin/users/{email}/temporary-password` | admin | `{sendEmail?}` → 200 `InviteResult`; 404 / 409 |
| `GET /api/admin/users` | admin | `User & {passwordSignIn, passwordChangePending}` |
| `POST /auth/local/login` | public | adds 200 `{passwordChangeRequired: true}` |
| `GET /auth/google/callback` | public | adds a redirect to `/?passwordChangeRequired=1` |
| `POST /auth/local/password/initial` | change cookie | `{newPassword}` → 200 `Status`, 200 `{twoFactorRequired: true}`, 400, 429 |
| `POST /auth/local/password/cancel` | — | 204 |
| `POST /api/me/security/password` | session | from #374, now also for invited users |

### Security invariants (each has a test)

1. The plaintext temporary password exists only in: the generator's return value, the mail body, and one HTTP response body. It never appears in logs, in errors, in `account-credentials.json`, `users.json`, or the session registry.
2. No session for a `mustChange` account before the change. Revocation on issue plus no session issue until the change enforces it. There is no separate allow-list of routes to keep up to date.
3. The change cookie never leaves the main host and never reaches other paths.
4. A password login does exactly one argon2 verification on every path (local admin, invited, unknown email, orphan credential).
5. A change or reset invalidates earlier change tokens (`version`) and earlier sessions (revocation).
6. Responses that carry a password send `Cache-Control: no-store`. The SPA keeps the password only in component state and wipes it when the card closes or the component unmounts. Never in zustand, storage, or the URL.
7. Admin-only routes check `requireAdmin` themselves, because the middleware gate is not authorization.

### Frontend behaviour

- **Invite form:** email, role (`admin`/`member`), and sign-in method. "Password" is the default when Google is not configured; "Google" is offered only when it is. "Email the credentials to the user" is shown and ticked only when SMTP is configured; otherwise a hint reads "SMTP is not configured; you'll share the credentials yourself". SMTP state comes from the existing `GET /api/admin/email`.
- **Result card** (inline, like `TwoFactorRecoveryCodes`, not a new modal):
  - `sent`: "Invitation emailed to …", then `ConfirmDialog` asking "View or download the credentials too?". Yes reveals them.
  - `not_requested`: credentials shown.
  - `not_configured`: credentials shown, with a warning: "Not emailed: SMTP is not configured".
  - `failed`: credentials shown, with an error: "The email could not be sent: <deliveryError>. The account was created."
  - Every state where the credentials are visible has Copy and Download PDF (disabled with a reason for non-ASCII emails, D8), plus Done, which clears the state.
- **User rows:** a "Password change pending" badge, and a **Reset temporary password** action (hidden for self and the local admin) with an email checkbox, using the same result card.
- **Login:** the copy at `LoginScreen.tsx:55` changes to "Sign in with your email and password, or with Google if your administrator set that up." The password form shows whenever the local admin is configured. After `passwordChangeRequired` (from the JSON reply or the `?passwordChangeRequired=1` redirect), `InitialPasswordStep` shows new password + confirm, with a back link that calls cancel. It has no skip.
- Loading, error and disabled states follow the existing `AddUserForm` and `TwoFactor*` patterns. No new UI library.

---

## Steps

Each backend step ends with `gofmt -l backend` (empty), then `cd backend && go vet ./... && go test ./internal/...`. Each frontend step ends with `cd frontend && npm test && npm run build`. Commit after each step with `git commit -s`, with **no** co-author trailer. Use `refactor(...)` for steps 1–2 and `feat(...)` or `test(...)` for the rest.

0. **Rebase and characterize.** After #375 merges: `git fetch upstream && git rebase upstream/qa`. Add `handlers/users_handler_test.go` pinning current behaviour: 401, 403, 409 without Google, 409 duplicate, 201, role change, last-admin and local-admin guards.
   Verify: the new tests pass on the unchanged code.

1. **Backend refactors R1–R3** (no behaviour change).
   Verify: step-0 tests, `service/auth` tests (`local_admin_claim_test.go`, `service_security_features_test.go`, `security_deep_test.go`) and `go vet` all pass. `git diff --stat` touches only the files named in R1–R3.

2. **Frontend refactors R4–R5** (no behaviour change).
   Verify: `recoveryCodeFileService.test.ts` is unchanged and passes; `npm run build`.

3. **Credential store + port.** Files: `service/auth/model.go` and `ports.go` (`AccountCredential`, `AccountCredentialStore{Get, Save, Delete, List}`), `stores/fileaccountcredentials/` + test, and the wiring in `stores/stores.go`, `service/services.go` and `cmd/remote/main.go`.
   Verify: round trip; `(nil, nil)` on absence; idempotent delete; mode 0600; `grep -n AccountCredentials` hits all three wiring files.

4. **Invited credentials in auth.** Files: `temporary_password.go` + test (20 symbols from a 31-character set with no look-alike characters, grouped `XXXXX-XXXXX-XXXXX-XXXXX`, about 99 bits, rejection sampling), `account_password_authenticator.go` + test, the R3 resolver extension, `IssueTemporaryPassword`, `PasswordStates`, and `ChangePassword` (#374) extended to invited credentials.
   Verify: invariant 4 with a counting-hasher seam; an orphan credential is ignored; `version` increments; issuing revokes earlier sessions; a self-change works for an invited user.

5. **Forced first-login change in auth.** Files: `password_change_initial.go` + test, `service.go` (`LoginResult.PasswordChangeRequired`, the `CompleteGoogleLogin` branch).
   Verify: a token is invalid after a reset or a change; it is rejected when expired or forged; reusing the temporary password is rejected; the 2FA hand-off works; for both password and Google sign-in, no `remote_session` exists before completion.

6. **Invitation service.** Files: `service/invitation/{model,service,service_test}.go`, `service/services.go`, `service/user_removal_cleanup.go`.
   Verify with fakes:
   - every delivery state;
   - a duplicate writes no credential;
   - a credential failure leads to `users.Remove`, and a double failure is reported;
   - the per-email lock serializes invite, reset and removal (a reset racing a removal leaves no credential);
   - a Google invite deletes a leftover credential;
   - the reset guards (D11);
   - a Google-only user gains a credential;
   - the mail body contains the URL, email and password and no expiry text;
   - a capturing `slog` handler and every returned error contain no password (invariant 1).

7. **HTTP.** Files: `users_handler.go` (method, `sendEmail`, reset subroute, DELETE through `invitation.Remove`, `no-store`, list join), `auth_local_handler.go` (new outcome, `/auth/local/password/initial`, `/cancel`), `auth_google_login_handler.go` (redirect), `auth_cookie.go` (set and clear the host-only, path-scoped cookie), and route registration.
   Verify through `httptest`:
   - every status in the API summary;
   - the cookie has no `Domain` and has `Path=/auth/local/password`;
   - with only the change cookie, `/api/projects`, `/ws/...`, `/auth/verify` and an application-gateway request are all rejected;
   - after the change, `/auth/me` shows the right `isAdmin`;
   - a request without `method` behaves exactly as before (step-0 tests stay green).

8. **Frontend API, models, state.** Files: `config/routes.ts`, `api/userApi.ts`, `api/authApi.ts`, `models/users.ts` (`InviteResult` discriminated on `method` and `delivery`), `models/auth.ts`, `state/hooks/users/useUserDirectory.ts`, `state/hooks/auth/useLocalAuthController.ts`, pure `initialPasswordFormState.ts` + test, and `invitationDeliveryCopy.ts` (delivery → message) + test.
   Verify: `npm test` covers the form rules and the copy for all four delivery states.

9. **Frontend UI and PDF.** Files: `ui/account/AddUserForm.tsx`, `UserRow.tsx`, `IssuedCredentialsCard.tsx` (new), `ui/auth/InitialPasswordStep.tsx` (new), `ui/auth/LoginScreen.tsx`, `services/users/invitationCredentialFileService.ts` + test.
   Verify:
   - the PDF test checks `%PDF-1.4`, `%%EOF`, a consistent xref, and that the email and password are present unescaped for an ASCII input;
   - the builder **refuses** non-ASCII input and the UI disables the button;
   - `npm run build`;
   - screenshots of the four card states, the reset flow and the first-login step for the PR.

10. **Docs.** `ARCHITECTURE.md` "Platform identity" (invited users may use email + password; `account-credentials.json`; the change cookie), `docs/02-workspaces/02-auth-users-and-access.md`, `docs/03-platform/08-api-and-realtime.md`, `docs/threat-model.md` (a temporary password in email; no expiry, D3; host-only cookie; orphan credential handling), and the README user guide.
    Verify: `grep -rn "account-credentials.json\|passwordChangeRequired" ARCHITECTURE.md docs/ README.md` hits each file.

11. **Full verification + QA.** All checks in the step header plus `go test ./...` at the repo root; `bash infra/qa/deploy-app.sh <sha>`; the acceptance criteria below on QA with a throwaway SMTP account that is never committed. Update the PR body with the output and screenshots.

## Data / migrations

- New file `DATA_DIR/account-credentials.json`, mode 0600. When it is absent there are no invited password accounts. No migration.
- `users.json` and `local-admin.json` are unchanged. Session registry records get `sessionsValidAfter` from #374.
- **Rollback:** an older binary ignores the new file. Invited password users cannot sign in until it is upgraded again. Google users and the local admin are unaffected. Say so in the release note.

## Risks

| # | Risk | Mitigation |
| --- | --- | --- |
| R1 | A non-expiring temporary password (D3) sitting in a mailbox. | The requester accepted it. Mitigations: the forced change on first use, Reset invalidates it, and the users list shows "Password change pending" so admins can see stale invites. Recorded in the threat model. |
| R2 | Emailing a password is weaker than a link. | Required by the issue. Revisit with a magic-link follow-up issue. |
| R3 | The synchronous SMTP send holds the request for up to 20 seconds. | A bounded context and a spinner. The account already exists, so a timeout becomes `failed`, not lost. |
| R4 | Compensation (`users.Remove`) runs the full removal cleanup on a brand-new user. | Harmless (it is idempotent and the user has no projects). Double failure is reported. |
| R5 | A token's `version` check and the save could race with a concurrent reset. | Both go through the per-account lock in `accountPasswordAuthenticator`, and the save re-checks `version`. |

## Deliberately not done

- **A pre-existing bug, reported rather than fixed:** user removal deletes the session-registry record (`service/user_removal_cleanup.go:64-68`). Re-inviting the same email as a **Google** user makes that user's old cookies (up to 30 days) valid again. Password invites are covered by the revocation in `IssueTemporaryPassword`, but changing the Google path is a separate behaviour change. Proposed follow-up issue: "revoke sessions when a user is removed".
- The `auth.Service` facade size (R-rejected).
- The recovery-code alert on password change (see #374 R4).

## Acceptance criteria (Given / When / Then)

1. **Given** SMTP is configured, **when** an admin invites `a@example.com` as member with "email" ticked, **then** the mail arrives with the sign-in URL, email, password and instructions; the UI says "emailed" and offers to view the credentials.
2. **Given** SMTP is configured, **when** "email" is unticked, **then** no mail is sent and the credentials are shown.
3. **Given** SMTP is not configured, **then** the credentials are shown with the not-emailed warning, and the PDF downloads and opens with the right email and password.
4. **Given** a broken SMTP configuration, **then** the account exists, the UI says the email was **not** sent and why, and the credentials are shown.
5. **Given** an existing email, **when** invited again, **then** 409, the existing user is unchanged, and no mail is sent.
6. **Given** a password invite, **when** the user signs in with the temporary password, **then** the set-password step shows, and `curl` with that browser's cookies to `/api/projects` and to a preview host gets 401 or a redirect to login.
7. **Given** the same user signs in with Google first, **then** they also land on the set-password step.
8. **Given** a new password under 12 characters or equal to the temporary one, **then** it is rejected; a valid one lands the user in the workspace with their role's rights only.
9. **Given** a completed change, **then** the temporary password fails and the new one works.
10. **When** an admin resets a user, **then** the old temporary password, any open set-password step, and the user's existing sessions all stop working; the reset is refused for self and for the local admin.
11. **Given** a Google-only member, **when** reset, **then** they can sign in with the temporary password and must change it.
12. **When** a user is removed and re-invited with a password, **then** they get a new credential and their old sessions don't come back.
13. **Given** `josé@exemple.fr`, **then** the invite and the mail work, and the PDF button is disabled with a reason.
14. **Given** any of the above, **then** `grep -rF "$TEMP_PW" /opt/remote.futrx/data` and `journalctl -u remote.futrx | grep -F "$TEMP_PW"` find nothing.
15. **Given** a Google invite without `method`, and a local-admin login, **then** both behave exactly as before.

## Self-review log

The plan was reviewed adversarially before it was marked ready. Defects found in earlier drafts and fixed here:

1. A base-domain change cookie would have leaked into every project container, because Caddy strips only named cookies. Fixed with a host-only, path-scoped cookie and no `infra/` change.
2. The PDF writer silently turned non-ASCII into `-`, so a downloaded credential could be wrong. Fixed with refusal plus a disabled button (D8).
3. `ReissueTrackedSession` would have recorded invited password users as Google sign-ins (magic `Sub`). Fixed by R2.
4. A login falling through two verifiers would have spent different amounts of time depending on the account type. Fixed by R3 (one resolver, one verification).
5. An expired first-login token would have returned 401 and dropped the user to the login screen. Changed to 400.
6. The first draft's extra `mustChange` check in `CurrentSession` is redundant once issuing a password revokes sessions and no session exists before the change. Removed (YAGNI).
7. Re-inviting a removed user could have brought back their old sessions. Fixed for password invites. The Google case is reported, not folded in.
8. An orphan credential (a reset racing a removal) could have let someone through password login, including after a later Google re-invite. Fixed on three layers: removal now goes through the same per-email lock as invite and reset; the resolver ignores credentials for unregistered emails; and a Google invite deletes any leftover credential. The second review pass caught that the first fix claimed a lock removal never took.
9. The first draft offered expiry and "Google allowed while pending". Replaced by the confirmed D2 and D3.

## Progress

- [ ] 0–11 not started (blocked on #375)

## Outcome

<!-- filled after implementation -->
