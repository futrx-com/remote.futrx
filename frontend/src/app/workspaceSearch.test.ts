import assert from "node:assert/strict";
import test from "node:test";
import type { SearchFilters, SortId } from "../models/search.ts";
import { searchFilterService } from "../services/workspace/searchFilterService.ts";
import { createWorkspaceSearchSurface } from "./workspaceSearch.ts";
import type { SearchPreferences } from "../port/workspaceSearch.ts";

/** A preferences boundary that records what it was asked to save. */
function recordingPreferences(initial?: Partial<SearchFilters>): SearchPreferences & {
  written: SearchFilters[];
  sorts: SortId[];
} {
  const written: SearchFilters[] = [];
  const sorts: SortId[] = [];
  return {
    written,
    sorts,
    readFilters: () => ({ ...searchFilterService.defaults(), ...initial }),
    writeFilters: (filters) => void written.push(filters),
    readSort: () => "relevance",
    writeSort: (sort) => void sorts.push(sort),
  };
}

test("hydrates its selection from the preferences boundary", () => {
  const preferences = recordingPreferences();
  preferences.readSort = () => "recent";
  const { store } = createWorkspaceSearchSurface(preferences);

  assert.equal(store.getState().query, "");
  assert.equal(store.getState().sort, "recent");
  assert.deepEqual(store.getState().filters, searchFilterService.defaults());
  assert.equal(preferences.written.length, 0, "hydrating must not save");
});

test("saves a filter change, and does not save the keyword", () => {
  const preferences = recordingPreferences();
  const { store, selection } = createWorkspaceSearchSurface(preferences);

  store.getState().setQuery("deploy");
  assert.equal(store.getState().query, "deploy");
  assert.equal(preferences.written.length, 0);

  selection.toggleFacetValue("provider", "codex");
  assert.deepEqual(store.getState().filters.facets.provider, ["codex"]);
  assert.deepEqual(preferences.written.at(-1), store.getState().filters);

  selection.setSort("title");
  assert.deepEqual(preferences.sorts, ["title"]);
});

test("clearAll drops the keyword and every filter", () => {
  const { store, selection } = createWorkspaceSearchSurface(recordingPreferences());

  store.getState().setQuery("deploy");
  selection.toggleFacetValue("provider", "codex");
  selection.setDateFilter({ preset: "7d", field: "lastMessageAt" });

  let notifications = 0;
  const unsubscribe = store.subscribe(() => {
    notifications += 1;
  });

  selection.clearAll();

  assert.equal(store.getState().query, "");
  assert.deepEqual(store.getState().filters, searchFilterService.defaults());
  assert.equal(notifications, 1, "one change, so subscribers see no half-cleared selection");
  unsubscribe();
});

test("each surface owns its selection", () => {
  const sidebar = createWorkspaceSearchSurface(recordingPreferences());
  const palette = createWorkspaceSearchSurface(recordingPreferences());

  palette.selection.toggleFacetValue("project", "alpha");

  assert.deepEqual(palette.store.getState().filters.facets.project, ["alpha"]);
  assert.deepEqual(sidebar.store.getState().filters.facets.project, []);
});

test("hydrates in read order and publishes before each synchronous preference write", () => {
  const events: string[] = [];
  const preferences = recordingPreferences();
  preferences.readFilters = () => {
    events.push("read filters");
    return searchFilterService.defaults();
  };
  preferences.readSort = () => {
    events.push("read sort");
    return "relevance";
  };
  preferences.writeFilters = () => events.push("write filters");
  preferences.writeSort = () => events.push("write sort");
  const { store, selection } = createWorkspaceSearchSurface(preferences);
  assert.deepEqual(events, ["read filters", "read sort"]);

  const unsubscribe = store.subscribe(() => events.push("publish"));
  events.length = 0;
  selection.toggleFacetValue("provider", "codex");
  selection.setSort("title");
  selection.clearAll();
  assert.deepEqual(events, [
    "publish", "write filters", "publish", "write sort", "publish", "write filters",
  ]);
  unsubscribe();
});

test("a preference write error propagates after the selection has been published", () => {
  const preferences = recordingPreferences();
  const error = new Error("preference write failed");
  preferences.writeFilters = () => { throw error; };
  const { store, selection } = createWorkspaceSearchSurface(preferences);
  let notifications = 0;
  const unsubscribe = store.subscribe(() => notifications += 1);

  assert.throws(() => selection.toggleFacetValue("provider", "codex"), (thrown) => thrown === error);
  assert.deepEqual(store.getState().filters.facets.provider, ["codex"]);
  assert.equal(notifications, 1);
  unsubscribe();
});
