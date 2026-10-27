import assert from "node:assert/strict";
import test from "node:test";

import { ApiError, isDefinitiveRejection } from "./apiError.ts";

test("a server rejection carries its message and status", () => {
  const rejection = new ApiError("too many push subscriptions for this user", 409);

  assert.equal(rejection.message, "too many push subscriptions for this user");
  assert.equal(rejection.status, 409);
  assert.ok(rejection instanceof Error);
});

test("definitive rejections are the deliberate client-error answers", () => {
  assert.equal(isDefinitiveRejection(new ApiError("no", 400)), true);
  assert.equal(isDefinitiveRejection(new ApiError("cap", 409)), true);
  assert.equal(isDefinitiveRejection(new ApiError("restarting", 502)), false);
  assert.equal(isDefinitiveRejection(new ApiError("busy", 503)), false);
  assert.equal(isDefinitiveRejection(new Error("network down")), false);
  assert.equal(isDefinitiveRejection("string"), false);
});
