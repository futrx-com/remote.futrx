import assert from "node:assert/strict";
import test from "node:test";
import type { DirectoryUser } from "../../../models/user.ts";
import { userDirectoryState } from "./userDirectoryState.ts";

function entry(email: string, extra: Partial<DirectoryUser> = {}): DirectoryUser {
  return { email, role: "member", addedAt: 1, passwordSignIn: false, passwordChangePending: false, ...extra };
}

const base = [entry("b@example.com"), entry("d@example.com")];

test("a password invite is listed in email order with a pending change", () => {
  const next = userDirectoryState.withInvited(base, {
    method: "password",
    delivery: "sent",
    temporaryPassword: "AAAAA-BBBBB-CCCCC-DDDDD",
    user: { email: "c@example.com", role: "admin", addedAt: 2 },
  });
  assert.deepEqual(next.map((user) => user.email), ["b@example.com", "c@example.com", "d@example.com"]);
  assert.equal(next[1].passwordChangePending, true);
  assert.equal(next[1].passwordSignIn, true);
  assert.equal(next[1].role, "admin");
});

test("the temporary password never reaches the stored directory entry", () => {
  const next = userDirectoryState.withInvited(null, {
    method: "password",
    delivery: "not_requested",
    temporaryPassword: "SECRET-SECRET-SECRET-1234",
    user: { email: "c@example.com", role: "member", addedAt: 2 },
  });
  assert.equal(JSON.stringify(next).includes("SECRET"), false);
});

test("a Google invite has no password state", () => {
  const next = userDirectoryState.withInvited(null, {
    method: "google",
    user: { email: "a@example.com", role: "member", addedAt: 2 },
  });
  assert.equal(next[0].passwordSignIn, false);
  assert.equal(next[0].passwordChangePending, false);
});

test("a reset marks only that account as pending, including a Google-only one", () => {
  const next = userDirectoryState.withTemporaryPassword(base, "d@example.com");
  assert.deepEqual(next.map((user) => user.passwordChangePending), [false, true]);
  assert.equal(next[1].passwordSignIn, true);
});

test("role changes and removal touch only the named account", () => {
  assert.deepEqual(userDirectoryState.withRole(base, "b@example.com", "admin").map((user) => user.role), ["admin", "member"]);
  assert.deepEqual(userDirectoryState.without(base, "b@example.com").map((user) => user.email), ["d@example.com"]);
  assert.deepEqual(userDirectoryState.without(null, "x@example.com"), []);
});

test("only a password invite carries credentials to show", () => {
  const user = { email: "a@example.com", role: "member" as const, addedAt: 1 };
  const result = { method: "password" as const, delivery: "sent" as const, temporaryPassword: "pw", user };
  assert.equal(userDirectoryState.issuedFrom(result), result);
  assert.equal(userDirectoryState.issuedFrom({ method: "google", user }), null);
});
