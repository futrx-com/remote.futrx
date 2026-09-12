import assert from "node:assert/strict";
import test from "node:test";
import { smtpSettingsForm } from "./smtpSettingsForm.ts";

function gmailInput(overrides: Partial<Parameters<typeof smtpSettingsForm.prepareSubmission>[0]> = {}) {
  return {
    ...smtpSettingsForm.blankGmailInput(),
    fromAddress: "user@example.com",
    username: "user@example.com",
    password: "abcd efgh ijkl mnop",
    ...overrides,
  };
}

test("a Gmail app password pasted with its display spacing is valid once stripped", () => {
  const result = smtpSettingsForm.prepareSubmission(gmailInput());
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.request.password, "abcdefghijklmnop");
  }
});

test("a 15-character Gmail password is rejected", () => {
  const result = smtpSettingsForm.prepareSubmission(gmailInput({ password: "abcd efgh ijkl mno" }));
  assert.equal(result.valid, false);
});

test("Custom SMTP does not strip whitespace or require 16 characters", () => {
  const result = smtpSettingsForm.prepareSubmission({
    ...smtpSettingsForm.blankCustomInput(),
    host: "smtp.example.com",
    port: "587",
    username: "mailer",
    password: "not gmail shaped",
    fromAddress: "mailer@example.com",
  });
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.request.password, "not gmail shaped");
  }
});

test("an out-of-range port is rejected", () => {
  const result = smtpSettingsForm.prepareSubmission(gmailInput({ port: "70000" }));
  assert.equal(result.valid, false);
});

test("an invalid sender address is rejected", () => {
  const result = smtpSettingsForm.prepareSubmission(gmailInput({ fromAddress: "not-an-email" }));
  assert.equal(result.valid, false);
});

test("a mixed-case sender address is lowercased", () => {
  const result = smtpSettingsForm.prepareSubmission(gmailInput({ fromAddress: "User@Example.com" }));
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.request.fromAddress, "user@example.com");
  }
});

test("no authentication hides credentials and submits no username or password", () => {
  const result = smtpSettingsForm.prepareSubmission({
    ...smtpSettingsForm.blankCustomInput(),
    host: "relay.internal",
    port: "25",
    tlsMode: "none",
    authentication: "none",
    fromAddress: "noreply@example.com",
  });
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.request.username, "");
    assert.equal(result.request.password, undefined);
  }
});

test("plaintext plus a non-none authentication is rejected client-side", () => {
  const result = smtpSettingsForm.prepareSubmission({
    ...smtpSettingsForm.blankCustomInput(),
    host: "relay.internal",
    port: "25",
    tlsMode: "none",
    authentication: "plain",
    username: "mailer",
    password: "secret",
    fromAddress: "noreply@example.com",
  });
  assert.equal(result.valid, false);
});

test("an empty password on edit is omitted so the backend retains it", () => {
  const result = smtpSettingsForm.prepareSubmission(
    gmailInput({ password: "", passwordConfigured: true })
  );
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.request.password, undefined);
  }
});

test("an empty password on first save is rejected", () => {
  const result = smtpSettingsForm.prepareSubmission(
    gmailInput({ password: "", passwordConfigured: false })
  );
  assert.equal(result.valid, false);
});

test("classify selects gmail only on an exact preset match", () => {
  assert.equal(
    smtpSettingsForm.classify({
      host: "smtp.gmail.com",
      port: 587,
      tlsMode: "starttls",
      authentication: "plain",
      username: "a@example.com",
      fromAddress: "a@example.com",
      passwordConfigured: true,
    }),
    "gmail"
  );
  assert.equal(
    smtpSettingsForm.classify({
      host: "smtp.gmail.com",
      port: 465,
      tlsMode: "implicit",
      authentication: "plain",
      username: "a@example.com",
      fromAddress: "a@example.com",
      passwordConfigured: true,
    }),
    "custom"
  );
});
