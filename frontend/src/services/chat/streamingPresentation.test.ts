import assert from "node:assert/strict";
import test from "node:test";
import type { AgentCapabilitiesCatalog } from "../../models/agentCapabilities.ts";
import { streamingPresentationFor } from "./streamingPresentation.ts";

test("uses provider policy and falls back to tokens when support is absent", () => {
  const catalog = { providers: [
    { provider: "claude", features: { streamingPresentation: "blocks" } },
    { provider: "future-agent", features: { streamingPresentation: "tokens" } },
  ] } as AgentCapabilitiesCatalog;

  assert.equal(streamingPresentationFor(catalog, "claude"), "blocks");
  assert.equal(streamingPresentationFor(catalog, "future-agent"), "tokens");
  assert.equal(streamingPresentationFor(catalog, "missing-agent"), "tokens");
  assert.equal(streamingPresentationFor(null, "claude"), "tokens");
});
