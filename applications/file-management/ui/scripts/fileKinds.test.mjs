import assert from "node:assert/strict";
import test from "node:test";

import {
  category,
  formatBytes,
  openAction,
  parentDir,
  viewableMediaKind,
} from "./fileKinds.js";

test("file policy preserves media, IDE, and download routing", () => {
  assert.deepEqual(openAction("photo.PNG"), { action: "media", kind: "image" });
  assert.deepEqual(openAction("song.ogg"), { action: "media", kind: "audio" });
  assert.deepEqual(openAction("source.ts"), { action: "ide" });
  assert.deepEqual(openAction("bundle.zip"), { action: "download" });
  assert.deepEqual(openAction("unsupported.mkv"), { action: "download" });
  assert.equal(viewableMediaKind("report.pdf"), "pdf");
  assert.equal(viewableMediaKind("archive.zip"), null);
  assert.equal(category("config.yaml"), "data");
});

test("file labels preserve compact tree formatting", () => {
  assert.equal(formatBytes(980), "980 B");
  assert.equal(formatBytes(4300), "4.2 KB");
  assert.equal(formatBytes(126 * 1024 * 1024), "126 MB");
  assert.equal(parentDir("src/components/app.js"), "src/components");
  assert.equal(parentDir("README.md"), "");
});
