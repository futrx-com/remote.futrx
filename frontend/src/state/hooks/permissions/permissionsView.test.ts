import assert from "node:assert/strict";
import test from "node:test";
import { PERMISSIONS_EMPTY_COPY, permissionsViewState } from "./permissionsView.ts";

test("a failed load renders the error state, never empty lists", () => {
  assert.equal(
    permissionsViewState({ loading: false, error: "500", loaded: false }),
    "error"
  );
  assert.equal(permissionsViewState({ loading: true, error: "404", loaded: true }), "error");
});

test("first load shows loading; later refreshes keep the lists", () => {
  assert.equal(permissionsViewState({ loading: true, error: null, loaded: false }), "loading");
  assert.equal(permissionsViewState({ loading: true, error: null, loaded: true }), "ready");
});

test("a settled load with no error is ready, and empty copy states reality", () => {
  assert.equal(permissionsViewState({ loading: false, error: null, loaded: true }), "ready");
  assert.equal(PERMISSIONS_EMPTY_COPY.roles, "No roles yet.");
  assert.equal(PERMISSIONS_EMPTY_COPY.assignments, "No assignments yet.");
});
