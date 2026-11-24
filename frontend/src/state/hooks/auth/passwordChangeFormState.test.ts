import assert from "node:assert/strict";
import test from "node:test";
import { passwordChangeFormState } from "./passwordChangeFormState.ts";

const valid = {
  currentPassword: "old password exact",
  newPassword: "new password exact",
  confirmation: "new password exact",
  twoFactorEnabled: false,
};

function resultForPassword(newPassword: string) {
  return passwordChangeFormState.prepareSubmission({ ...valid, newPassword, confirmation: newPassword });
}

test("requires the current password before validating new credentials", () => {
  assert.deepEqual(passwordChangeFormState.prepareSubmission({ ...valid, currentPassword: "" }), {
    valid: false, error: "Current password is required.",
  });
});

test("minimum uses Unicode code points, including supplementary characters", () => {
  for (const short of ["a".repeat(11), "😀".repeat(6), "😀".repeat(11), "é".repeat(11)]) {
    assert.deepEqual(resultForPassword(short), { valid: false, error: "Use at least 12 characters." });
  }
  for (const accepted of ["a".repeat(12), "😀".repeat(12), "é".repeat(12)]) {
    assert.equal(resultForPassword(accepted).valid, true);
  }
});

test("maximum applies to UTF-8 bytes and accepts the exact boundary", () => {
  for (const accepted of ["a".repeat(1024), "é".repeat(512), "😀".repeat(256)]) {
    assert.equal(resultForPassword(accepted).valid, true);
  }
  for (const tooLong of ["a".repeat(1025), "é".repeat(513), "😀".repeat(257)]) {
    assert.deepEqual(resultForPassword(tooLong), { valid: false, error: "Use at most 1024 bytes." });
  }
});

test("requires exact confirmation and a different password", () => {
  assert.deepEqual(passwordChangeFormState.prepareSubmission({ ...valid, confirmation: "different" }), {
    valid: false, error: "Passwords do not match.",
  });
  assert.deepEqual(passwordChangeFormState.prepareSubmission({ ...valid, currentPassword: valid.newPassword }), {
    valid: false, error: "New password must differ from your current password.",
  });
});

test("preserves password whitespace and only trims the optional second factor", () => {
  const currentPassword = "  old password  ";
  const newPassword = "  new password  ";
  assert.deepEqual(passwordChangeFormState.prepareSubmission({
    currentPassword, newPassword, confirmation: newPassword, twoFactorEnabled: true, code: "  recovery-code  ",
  }), { valid: true, input: { currentPassword, newPassword, code: "recovery-code" } });
  assert.equal(resultForPassword(" ".repeat(12)).valid, true);
});

test("requires a nonblank code only when two factor is enabled", () => {
  for (const code of [undefined, "", " \t "]) {
    assert.deepEqual(passwordChangeFormState.prepareSubmission({ ...valid, twoFactorEnabled: true, code }), {
      valid: false, error: "Enter an authenticator code or recovery code.",
    });
  }
  for (const code of ["123456", "recovery-code"]) {
    assert.deepEqual(passwordChangeFormState.prepareSubmission({ ...valid, twoFactorEnabled: true, code }), {
      valid: true, input: { currentPassword: valid.currentPassword, newPassword: valid.newPassword, code },
    });
  }
  assert.deepEqual(passwordChangeFormState.prepareSubmission({ ...valid, code: "ignore-hidden-code" }), {
    valid: true, input: { currentPassword: valid.currentPassword, newPassword: valid.newPassword },
  });
});
