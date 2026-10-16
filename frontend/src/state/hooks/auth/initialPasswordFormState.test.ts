import assert from "node:assert/strict";
import test from "node:test";
import { initialPasswordFormState } from "./initialPasswordFormState.ts";

function submit(newPassword: string, confirmation = newPassword) {
  return initialPasswordFormState.prepareSubmission({ newPassword, confirmation });
}

test("minimum counts Unicode characters, not UTF-16 units", () => {
  for (const short of ["a".repeat(11), "😀".repeat(11)]) {
    assert.deepEqual(submit(short), { valid: false, error: "Use at least 12 characters." });
  }
  for (const accepted of ["a".repeat(12), "😀".repeat(12)]) {
    assert.deepEqual(submit(accepted), { valid: true, newPassword: accepted });
  }
});

test("maximum is 1024 UTF-8 bytes, boundary included", () => {
  assert.equal(submit("a".repeat(1024)).valid, true);
  assert.equal(submit("é".repeat(512)).valid, true);
  assert.deepEqual(submit("a".repeat(1025)), { valid: false, error: "Use at most 1024 bytes." });
  assert.deepEqual(submit("é".repeat(513)), { valid: false, error: "Use at most 1024 bytes." });
});

test("the confirmation must match exactly and whitespace is kept", () => {
  assert.deepEqual(submit("a long enough pass", "a long enough pasS"), {
    valid: false, error: "Passwords do not match.",
  });
  assert.deepEqual(submit("  spaced out pass  "), { valid: true, newPassword: "  spaced out pass  " });
});

test("the length rule is reported before the mismatch", () => {
  assert.deepEqual(submit("short", "other"), { valid: false, error: "Use at least 12 characters." });
});

test("an expired step sends the user back to sign-in", () => {
  assert.deepEqual(
    initialPasswordFormState.classifyFailure(400, "this sign-in step expired, sign in again"),
    { kind: "expired", message: "That sign-in step expired. Sign in again." },
  );
});

test("a password rule failure keeps the form, with the server's words", () => {
  for (const message of [
    "new password must be different from the temporary password",
    "password must be at least 12 characters",
  ]) {
    assert.deepEqual(initialPasswordFormState.classifyFailure(400, message), { kind: "rejected", message });
  }
});

test("rate limits and server errors keep the form", () => {
  assert.deepEqual(
    initialPasswordFormState.classifyFailure(429, "too many attempts; try again in a few minutes"),
    { kind: "rejected", message: "too many attempts; try again in a few minutes" },
  );
  assert.equal(initialPasswordFormState.classifyFailure(500, "password not changed").kind, "rejected");
});

test("the expiry text on another status, or no usable text, is not an expiry", () => {
  assert.equal(initialPasswordFormState.classifyFailure(500, "this sign-in step expired, sign in again").kind, "rejected");
  assert.deepEqual(initialPasswordFormState.classifyFailure(undefined, "500"), {
    kind: "rejected", message: "Could not set the new password. Try again.",
  });
  assert.deepEqual(initialPasswordFormState.classifyFailure(undefined, ""), {
    kind: "rejected", message: "Could not set the new password. Try again.",
  });
});
