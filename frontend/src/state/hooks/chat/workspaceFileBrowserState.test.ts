import assert from "node:assert/strict";
import test from "node:test";
import type { FileNode } from "../../../models/files.ts";
import { workspaceFileBrowserState } from "./workspaceFileBrowserState.ts";

test("preserves independent directory loading and search state", () => {
  const entry: FileNode = { name: "src", path: "src", isDir: true, size: 0, modTime: 1 };
  const loading = workspaceFileBrowserState.reduce(workspaceFileBrowserState.createInitial(), {
    type: "directory-load-started",
    path: "",
  });
  const loaded = workspaceFileBrowserState.reduce(loading, {
    type: "directory-load-succeeded",
    path: "",
    entries: [entry],
    truncated: true,
  });
  const searching = workspaceFileBrowserState.reduce(loaded, { type: "search-started" });
  const failed = workspaceFileBrowserState.reduce(searching, {
    type: "search-failed",
    error: "unavailable",
  });

  assert.equal(failed.rootLoading, false);
  assert.deepEqual(failed.childrenByDir.get(""), [entry]);
  assert.equal(failed.truncatedDirs.has(""), true);
  assert.deepEqual(failed.searchResults, []);
  assert.equal(failed.searchError, "unavailable");
  assert.equal(failed.searching, false);
});
