import assert from "node:assert/strict";
import test from "node:test";
import { emailSettingsForm } from "./emailSettingsForm.ts";

test("a Gmail app password pasted with its display spacing is valid once stripped", () => {
  const result = emailSettingsForm.prepareSubmission({
    address: "user@example.com",
    appPassword: "abcd efgh ijkl mnop",
  });
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.appPassword, "abcdefghijklmnop");
  }
});

test("a 15-character password is rejected", () => {
  const result = emailSettingsForm.prepareSubmission({
    address: "user@example.com",
    appPassword: "abcd efgh ijkl mno",
  });
  assert.equal(result.valid, false);
});

test("a 17-character password is rejected", () => {
  const result = emailSettingsForm.prepareSubmission({
    address: "user@example.com",
    appPassword: "abcd efgh ijkl mnopq",
  });
  assert.equal(result.valid, false);
});

test("an empty address is rejected", () => {
  const result = emailSettingsForm.prepareSubmission({
    address: "",
    appPassword: "abcd efgh ijkl mnop",
  });
  assert.equal(result.valid, false);
});

test("an address without @ is rejected", () => {
  const result = emailSettingsForm.prepareSubmission({
    address: "not-an-email",
    appPassword: "abcd efgh ijkl mnop",
  });
  assert.equal(result.valid, false);
});

test("a mixed-case address is lowercased", () => {
  const result = emailSettingsForm.prepareSubmission({
    address: "User@Example.com",
    appPassword: "abcd efgh ijkl mnop",
  });
  assert.equal(result.valid, true);
  if (result.valid) {
    assert.equal(result.address, "user@example.com");
  }
});
