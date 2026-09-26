import assert from "node:assert/strict";
import test from "node:test";
import { isMermaidLanguage } from "./mermaidBlock.ts";

test("recognizes mermaid language tag case-insensitively", () => {
  assert.equal(isMermaidLanguage("mermaid"), true);
  assert.equal(isMermaidLanguage("Mermaid"), true);
  assert.equal(isMermaidLanguage("mmd"), true);
  assert.equal(isMermaidLanguage(" mermaid "), true);
});

test("rejects non-mermaid language tags", () => {
  assert.equal(isMermaidLanguage("ts"), false);
  assert.equal(isMermaidLanguage(""), false);
  assert.equal(isMermaidLanguage(undefined), false);
  assert.equal(isMermaidLanguage("flowchart"), false);
});
