import assert from "node:assert/strict";
import test from "node:test";
import { localAuthFormState } from "./localAuthFormState.ts";

test("identifies both account-setup modes", () => {
  assert.equal(localAuthFormState.isSetup("claim"), true);
  assert.equal(localAuthFormState.isSetup("legacy-setup"), true);
  assert.equal(localAuthFormState.isSetup("login"), false);
});

test("normalizes a valid email for submission", () => {
  assert.deepEqual(
    localAuthFormState.prepareSubmission({
      mode: "login",
      email: "  Admin@Example.COM ",
      password: "password",
      confirmation: "",
    }),
    { valid: true, email: "admin@example.com" }
  );
});

test("preserves the credential validation order and messages", () => {
  assert.deepEqual(
    localAuthFormState.prepareSubmission({
      mode: "claim",
      email: "   ",
      password: "short",
      confirmation: "different",
    }),
    { valid: false, error: "Email is required." }
  );
  assert.deepEqual(
    localAuthFormState.prepareSubmission({
      mode: "claim",
      email: "admin@example.com",
      password: "short",
      confirmation: "different",
    }),
    { valid: false, error: "Passwords do not match." }
  );
  assert.deepEqual(
    localAuthFormState.prepareSubmission({
      mode: "legacy-setup",
      email: "admin@example.com",
      password: "short",
      confirmation: "short",
    }),
    { valid: false, error: "Use at least 12 characters." }
  );
});

test("login does not apply account-setup password rules", () => {
  assert.deepEqual(
    localAuthFormState.prepareSubmission({
      mode: "login",
      email: "admin@example.com",
      password: "short",
      confirmation: "different",
    }),
    { valid: true, email: "admin@example.com" }
  );
});
