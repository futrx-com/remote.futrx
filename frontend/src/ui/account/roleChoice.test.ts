import { strict as assert } from "node:assert";
import { describe, it } from "node:test";

import type { RbacRole } from "../../models/rbac.ts";
import type { User } from "../../models/user.ts";
import { appliesMemberRole } from "./roleChoice.ts";

function role(id: string): RbacRole {
  return { id, name: id, description: "", rules: [], createdBy: "system", createdAt: 0, updatedAt: 0 };
}

const member = role("member");
const viewer = role("viewer");

function user(role: User["role"]): User {
  return { email: "someone@example.com", role } as User;
}

describe("appliesMemberRole", () => {
  it("applies to a non-admin without another platform role", () => {
    assert.equal(appliesMemberRole(user(""), [], [member, viewer]), true);
  });

  it("does not apply to admins or users with a custom role", () => {
    assert.equal(appliesMemberRole(user("admin"), [], [member]), false);
    assert.equal(appliesMemberRole(user(""), [viewer], [member, viewer]), false);
  });

  it("does not apply once the member role is deleted", () => {
    assert.equal(appliesMemberRole(user(""), [], [viewer]), false);
  });
});
