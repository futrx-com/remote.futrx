import assert from "node:assert/strict";
import test from "node:test";
import { searchFilterService } from "../../../services/workspace/searchFilterService.ts";
import { createWorkspaceSearchStore } from "./workspaceSearchStore.ts";

test("counts stay on until the last menu releases them", () => {
  const store = createWorkspaceSearchStore({ filters: searchFilterService.defaults(), sort: "relevance" });

  const releaseSidebar = store.getState().retainCounts();
  const releasePalette = store.getState().retainCounts();
  assert.equal(store.getState().countsRetained, 2);

  releaseSidebar();
  assert.ok(store.getState().countsRetained > 0, "one menu is still open");

  releaseSidebar();
  assert.equal(store.getState().countsRetained, 1, "a repeated release does nothing");

  releasePalette();
  assert.equal(store.getState().countsRetained, 0);
});
