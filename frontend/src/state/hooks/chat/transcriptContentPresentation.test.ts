import assert from "node:assert/strict";
import test from "node:test";
import { fullResponseErrorMessage, fullResponseLabel } from "./transcriptContentPresentation.ts";

test("full response errors always produce a useful message", () => {
  assert.equal(fullResponseErrorMessage(new Error("network failed")), "network failed");
  assert.equal(fullResponseErrorMessage("request rejected"), "request rejected");
  assert.equal(fullResponseErrorMessage({ status: 500 }), "Failed to load the full response.");
  assert.equal(fullResponseErrorMessage(null), "Failed to load the full response.");
});

test("full response labels preserve optional sizes and their rounding", () => {
  assert.equal(fullResponseLabel(false), "Load full response");
  assert.equal(fullResponseLabel(false, 0), "Load full response");
  assert.equal(fullResponseLabel(false, 1023), "Load full response (1023 B)");
  assert.equal(fullResponseLabel(false, 1024), "Load full response (1 KB)");
  assert.equal(fullResponseLabel(false, 1025), "Load full response (2 KB)");
  assert.equal(fullResponseLabel(false, 1024 * 1024), "Load full response (1.0 MB)");
  assert.equal(fullResponseLabel(true, 1024), "Loading full response…");
});
