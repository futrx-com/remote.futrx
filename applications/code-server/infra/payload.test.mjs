import assert from "node:assert/strict";
import fs from "node:fs";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

test("shipped installer payload matches the reviewable source files", () => {
  const payload = fileURLToPath(new URL("./payload.tar.gz", import.meta.url));
  const entries = execFileSync("tar", ["-tzf", payload], { encoding: "utf8" }).trim().split("\n");
  assert.deepEqual(entries, ["infra/remote-open.html", "infra/migrate-settings.cjs"]);
  for (const entry of entries) {
    const packed = execFileSync("tar", ["-xOzf", payload, entry]);
    const source = fs.readFileSync(new URL(`../${entry}`, import.meta.url));
    assert.deepEqual(packed, source, `${entry}: run bash applications/code-server/infra/build-payload.sh`);
  }
});
