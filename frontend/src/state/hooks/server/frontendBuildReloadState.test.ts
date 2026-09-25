import assert from "node:assert/strict";
import test from "node:test";
import type { FrontendBuildPage } from "../../../models/frontendBuild.ts";
import { frontendBuildReloadState } from "./frontendBuildReloadState.ts";

function page(overrides: Partial<FrontendBuildPage> = {}): FrontendBuildPage {
  return {
    running: "old",
    served: "new",
    reloadedFor: null,
    opening: false,
    hidden: false,
    editing: false,
    held: false,
    ...overrides,
  };
}

test("stays on a build the server still serves or cannot name", () => {
  assert.equal(frontendBuildReloadState.decide(page({ served: "old" })), "stay");
  assert.equal(frontendBuildReloadState.decide(page({ served: null })), "stay");
  assert.equal(frontendBuildReloadState.decide(page({ running: null })), "stay");
});

test("reloads an idle page onto a newer build", () => {
  assert.equal(frontendBuildReloadState.decide(page()), "reload");
});

test("waits while the user is typing, then reloads once they leave or return", () => {
  assert.equal(frontendBuildReloadState.decide(page({ editing: true })), "wait");
  assert.equal(frontendBuildReloadState.decide(page({ editing: true, hidden: true })), "reload");
  assert.equal(frontendBuildReloadState.decide(page({ editing: true, opening: true })), "reload");
});

test("never drops held work, even when the page is out of sight", () => {
  assert.equal(frontendBuildReloadState.decide(page({ held: true })), "wait");
  assert.equal(frontendBuildReloadState.decide(page({ held: true, hidden: true })), "wait");
  assert.equal(frontendBuildReloadState.decide(page({ held: true, opening: true })), "wait");
});

test("reloads once per served build, so a stale cache cannot loop the page", () => {
  assert.equal(frontendBuildReloadState.decide(page({ reloadedFor: "new" })), "stay");
  assert.equal(frontendBuildReloadState.decide(page({ reloadedFor: "new", opening: true })), "stay");
  assert.equal(frontendBuildReloadState.decide(page({ reloadedFor: "older" })), "reload");
});
