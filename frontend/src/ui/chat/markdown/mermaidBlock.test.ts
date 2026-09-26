import assert from "node:assert/strict";
import test from "node:test";
import { isChunkLoadError, isMermaidLanguage } from "./mermaidBlock.ts";

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

test("recognizes failed lazy chunk loads across browsers", () => {
  assert.equal(
    isChunkLoadError(new TypeError("Failed to fetch dynamically imported module: https://x/assets/elk-1.js")),
    true,
  );
  assert.equal(isChunkLoadError(new TypeError("Importing a module script failed.")), true);
  assert.equal(isChunkLoadError(new TypeError("error loading dynamically imported module: https://x/a.js")), true);
});

test("does not treat mermaid parse errors as chunk load failures", () => {
  assert.equal(isChunkLoadError(new Error("Parse error on line 2: ...")), false);
  assert.equal(isChunkLoadError(undefined), false);
});
