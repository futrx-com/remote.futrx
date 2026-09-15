import assert from "node:assert/strict";
import test from "node:test";
import { DEFAULT_SORT } from "../../config/search.ts";
import { searchFilterService } from "./searchFilterService.ts";
import {
  ephemeralSearchPreferenceService,
  searchPreferenceService,
} from "./searchPreferenceService.ts";

test("the palette's filters never reach the sidebar's stored selection", () => {
  const store = new Map<string, string>();
  (globalThis as { localStorage?: unknown }).localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => void store.set(key, value),
  };

  // The sidebar sets up a project scope and it survives a reload.
  const scoped = searchFilterService.defaults();
  scoped.facets.project = ["p-remote"];
  searchPreferenceService.writeFilters(scoped);
  searchPreferenceService.writeSort("recent");
  assert.deepEqual(searchPreferenceService.readFilters().facets.project, ["p-remote"]);

  // The palette narrows to something else. Sharing one state, this used to
  // re-scope the sidebar behind the user's back, and outlive the session.
  const inPalette = searchFilterService.defaults();
  inPalette.facets.project = ["p-docs"];
  ephemeralSearchPreferenceService.writeFilters(inPalette);
  ephemeralSearchPreferenceService.writeSort("oldest");

  assert.deepEqual(searchPreferenceService.readFilters().facets.project, ["p-remote"]);
  assert.equal(searchPreferenceService.readSort(), "recent");
  // And the palette itself opens clean rather than inheriting either one.
  assert.deepEqual(ephemeralSearchPreferenceService.readFilters(), searchFilterService.defaults());
  assert.equal(ephemeralSearchPreferenceService.readSort(), DEFAULT_SORT);
});

