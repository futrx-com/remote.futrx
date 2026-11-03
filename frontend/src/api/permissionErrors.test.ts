import assert from "node:assert/strict";
import test from "node:test";
import { ApiError } from "./apiError.ts";
import { permissionErrors } from "./permissionErrors.ts";

test("a 409 on delete offers the unbind option; other errors pass through", () => {
  const conflict = new ApiError("role is bound to users", 409);
  assert.equal(permissionErrors.isRoleStillBound(conflict), true);
  assert.match(permissionErrors.deleteRoleErrorMessage(conflict), /Also remove its bindings/);
  const other = new ApiError("boom", 500);
  assert.equal(permissionErrors.isRoleStillBound(other), false);
  assert.equal(permissionErrors.deleteRoleErrorMessage(other), "boom");
});
