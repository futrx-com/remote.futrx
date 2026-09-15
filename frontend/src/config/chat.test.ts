import assert from "node:assert/strict";
import test from "node:test";
import { providerDisplayLabel } from "./chat.ts";

test("providerDisplayLabel preserves known provider branding", () => {
  assert.equal(providerDisplayLabel("codex"), "Codex");
  assert.equal(providerDisplayLabel("minimax"), "MiniMax");
});

test("providerDisplayLabel formats future provider identifiers", () => {
  assert.equal(providerDisplayLabel("future-agent"), "Future Agent");
});
