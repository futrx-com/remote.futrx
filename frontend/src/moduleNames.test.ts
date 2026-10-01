// Two modules whose import paths differ only in case are one file to a
// case-insensitive filesystem (Windows, default macOS). There `./MermaidBlock`
// resolved to `mermaidBlock.ts` and the build failed, while Linux CI never
// noticed. Every module's extensionless path must stay unique ignoring case.
import assert from "node:assert/strict";
import { readdirSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const SOURCE_ROOT = fileURLToPath(new URL(".", import.meta.url));
const MODULE_EXTENSION = /\.(?:tsx?|jsx?|mts|mjs|cts|cjs)$/;

test("no two source modules differ only in case", () => {
  const byFoldedPath = new Map<string, string[]>();
  for (const entry of readdirSync(SOURCE_ROOT, { recursive: true, encoding: "utf8" })) {
    const path = entry.replaceAll("\\", "/");
    if (!MODULE_EXTENSION.test(path)) continue;
    const modulePath = path.replace(MODULE_EXTENSION, "");
    const folded = modulePath.toLowerCase();
    byFoldedPath.set(folded, [...(byFoldedPath.get(folded) ?? []), path]);
  }

  const clashes = [...byFoldedPath.values()].filter((paths) => {
    const spellings = new Set(paths.map((path) => path.replace(MODULE_EXTENSION, "")));
    return spellings.size > 1;
  });
  assert.deepEqual(clashes, [], `modules that differ only in case: ${JSON.stringify(clashes)}`);
});
