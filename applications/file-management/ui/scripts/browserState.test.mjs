import assert from "node:assert/strict";
import test from "node:test";

import { createInitialState, reduce } from "./browserState.js";

test("directory transitions keep independent loading, error, and truncation state", () => {
  let state = createInitialState();
  state = reduce(state, { type: "directory-load-started", path: "" });
  assert.equal(state.rootLoading, true);
  state = reduce(state, {
    type: "directory-load-succeeded",
    path: "",
    entries: [{ name: "src", path: "src", isDir: true }],
    truncated: true,
  });
  assert.equal(state.rootLoading, false);
  assert.equal(state.childrenByDir.get("").length, 1);
  assert.equal(state.truncatedDirs.has(""), true);

  state = reduce(state, { type: "directory-toggled", path: "src" });
  assert.equal(state.expanded.has("src"), true);
  state = reduce(state, { type: "directory-load-failed", path: "src", error: "gone" });
  assert.equal(state.errorByDir.get("src"), "gone");
});

test("search transitions switch cleanly between browse, results, and errors", () => {
  let state = reduce(createInitialState(), { type: "query-changed", query: "app" });
  state = reduce(state, { type: "search-started" });
  assert.equal(state.searching, true);
  state = reduce(state, {
    type: "search-succeeded",
    entries: [{ name: "app.go", path: "src/app.go", isDir: false }],
    truncated: true,
  });
  assert.equal(state.searchResults.length, 1);
  assert.equal(state.searchTruncated, true);
  state = reduce(state, { type: "search-idle" });
  assert.equal(state.searchResults, null);
  assert.equal(state.searchError, null);
});
