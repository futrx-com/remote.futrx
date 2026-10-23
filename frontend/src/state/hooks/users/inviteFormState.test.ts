import assert from "node:assert/strict";
import test from "node:test";
import { inviteFormState, SMTP_NOT_CONFIGURED_HINT } from "./inviteFormState.ts";

const none = { method: null, sendEmail: null };

test("without Google only the password method is offered, and it is the default", () => {
  const view = inviteFormState.view({ googleConfigured: false, smtpConfigured: false }, none);
  assert.deepEqual(view.methods, ["password"]);
  assert.equal(view.method, "password");
});

test("Google is offered only when configured, yet Password stays the default", () => {
  const view = inviteFormState.view({ googleConfigured: true, smtpConfigured: false }, none);
  assert.deepEqual(view.methods, ["google", "password"]);
  assert.equal(view.method, "password");
  assert.equal(inviteFormState.view({ googleConfigured: true, smtpConfigured: false }, { method: "google", sendEmail: null }).method, "google");
});

test("a Google choice is ignored once Google is no longer configured", () => {
  assert.equal(inviteFormState.view({ googleConfigured: false, smtpConfigured: false }, { method: "google", sendEmail: null }).method, "password");
});

test("the email option is ticked by default only when SMTP is configured", () => {
  const configured = inviteFormState.view({ googleConfigured: false, smtpConfigured: true }, none);
  assert.deepEqual([configured.showEmailOption, configured.sendEmail, configured.emailHint], [true, true, null]);

  const missing = inviteFormState.view({ googleConfigured: false, smtpConfigured: false }, none);
  assert.deepEqual([missing.showEmailOption, missing.sendEmail, missing.emailHint], [false, false, SMTP_NOT_CONFIGURED_HINT]);
});

test("unticking is remembered, and no stale tick survives missing SMTP", () => {
  assert.equal(inviteFormState.view({ googleConfigured: false, smtpConfigured: true }, { method: null, sendEmail: false }).sendEmail, false);
  assert.equal(inviteFormState.view({ googleConfigured: false, smtpConfigured: false }, { method: null, sendEmail: true }).sendEmail, false);
});

test("the Google method has neither the email option nor the hint", () => {
  const view = inviteFormState.view({ googleConfigured: true, smtpConfigured: true }, { method: "google", sendEmail: null });
  assert.deepEqual([view.showEmailOption, view.sendEmail, view.emailHint], [false, false, null]);
});
