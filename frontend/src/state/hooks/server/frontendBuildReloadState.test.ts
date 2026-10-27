import assert from "node:assert/strict";
import test from "node:test";
import type { FrontendBuildPage } from "../../../models/frontendBuild.ts";
import { frontendBuildReloadState } from "./frontendBuildReloadState.ts";

function page(overrides: Partial<FrontendBuildPage> = {}): FrontendBuildPage {
  return {
    running: "old",
    served: "new",
    reloadedFor: null,
    ...overrides,
  };
}

test("stays on a build the server still serves or cannot name", () => {
  assert.equal(frontendBuildReloadState.shouldReload(page({ served: "old" })), false);
  assert.equal(frontendBuildReloadState.shouldReload(page({ served: null })), false);
  assert.equal(frontendBuildReloadState.shouldReload(page({ running: null })), false);
});

test("reloads onto a newer build", () => {
  assert.equal(frontendBuildReloadState.shouldReload(page()), true);
});

test("reloads once per served build, so a stale cache cannot loop the page", () => {
  assert.equal(frontendBuildReloadState.shouldReload(page({ reloadedFor: "new" })), false);
  assert.equal(frontendBuildReloadState.shouldReload(page({ reloadedFor: "older" })), true);
});
