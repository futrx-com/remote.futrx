import assert from "node:assert/strict";
import test from "node:test";
import type { SearchFilters, SearchPreferences, SortId } from "../../../models/search.ts";
import { searchFilterService } from "../../../services/workspace/searchFilterService.ts";
import { createWorkspaceSearchStore } from "./workspaceSearchStore.ts";

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
  const store = createWorkspaceSearchStore(preferences);

  assert.equal(store.getState().query, "");
  assert.equal(store.getState().sort, "recent");
  assert.deepEqual(store.getState().filters, searchFilterService.defaults());
  assert.equal(preferences.written.length, 0, "hydrating must not save");
});

test("saves a filter change, and does not save the keyword", () => {
  const preferences = recordingPreferences();
  const store = createWorkspaceSearchStore(preferences);

  store.getState().setQuery("deploy");
  assert.equal(store.getState().query, "deploy");
  assert.equal(preferences.written.length, 0);

  store.getState().toggleFacetValue("provider", "codex");
  assert.deepEqual(store.getState().filters.facets.provider, ["codex"]);
  assert.deepEqual(preferences.written.at(-1), store.getState().filters);

  store.getState().setSort("title");
  assert.deepEqual(preferences.sorts, ["title"]);
});

test("clearAll drops the keyword and every filter", () => {
  const store = createWorkspaceSearchStore(recordingPreferences());

  store.getState().setQuery("deploy");
  store.getState().toggleFacetValue("provider", "codex");
  store.getState().setDateFilter({ preset: "7d", field: "lastMessageAt" });

  let notifications = 0;
  const unsubscribe = store.subscribe(() => {
    notifications += 1;
  });

  store.getState().clearAll();

  assert.equal(store.getState().query, "");
  assert.deepEqual(store.getState().filters, searchFilterService.defaults());
  assert.equal(notifications, 1, "one change, so subscribers see no half-cleared selection");
  unsubscribe();
});

test("counts stay on until the last menu releases them", () => {
  const store = createWorkspaceSearchStore(recordingPreferences());

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

test("each surface owns its selection", () => {
  const sidebar = createWorkspaceSearchStore(recordingPreferences());
  const palette = createWorkspaceSearchStore(recordingPreferences());

  palette.getState().toggleFacetValue("project", "alpha");

  assert.deepEqual(palette.getState().filters.facets.project, ["alpha"]);
  assert.deepEqual(sidebar.getState().filters.facets.project, []);
});
