import assert from "node:assert/strict";
import test from "node:test";
import { fileService } from "./fileService.ts";

test("viewableMediaKind mirrors the backend inline media set", () => {
  assert.equal(fileService.viewableMediaKind("shot.PNG"), "image");
  assert.equal(fileService.viewableMediaKind("demo.mp4"), "video");
  assert.equal(fileService.viewableMediaKind("voice.m4a"), "audio");
  assert.equal(fileService.viewableMediaKind("report.pdf"), "pdf");
  assert.equal(fileService.viewableMediaKind("app.tsx"), null);
  assert.equal(fileService.viewableMediaKind("archive.zip"), null);
  assert.equal(fileService.viewableMediaKind("clip.mkv"), null);
  assert.equal(fileService.viewableMediaKind("noextension"), null);
});

test("openAction opens media in the viewer", () => {
  assert.deepEqual(fileService.openAction("shot.png"), { action: "media", kind: "image" });
  assert.deepEqual(fileService.openAction("report.pdf"), { action: "media", kind: "pdf" });
});

test("openAction opens source and text files in the IDE", () => {
  assert.deepEqual(fileService.openAction("main.go"), { action: "ide" });
  assert.deepEqual(fileService.openAction("data.json"), { action: "ide" });
  assert.deepEqual(fileService.openAction("README.md"), { action: "ide" });
  assert.deepEqual(fileService.openAction("Makefile"), { action: "ide" });
});

test("openAction downloads archives and unrenderable media", () => {
  assert.deepEqual(fileService.openAction("bundle.zip"), { action: "download" });
  assert.deepEqual(fileService.openAction("clip.mkv"), { action: "download" });
  assert.deepEqual(fileService.openAction("photo.heic"), { action: "download" });
});

// The two size formatters are deliberately different — the file tree has a
// column, an attachment chip has a corner. Keep them apart.
test("formatBytes and formatBytesCompact stay different on purpose", () => {
  assert.equal(fileService.formatBytes(980), "980 B");
  assert.equal(fileService.formatBytes(4300), "4.2 KB");
  assert.equal(fileService.formatBytes(132_120_576), "126 MB");

  assert.equal(fileService.formatBytesCompact(980), "980B");
  assert.equal(fileService.formatBytesCompact(4300), "4 KB");
  assert.equal(fileService.formatBytesCompact(132_120_576), "126.0 MB");
});
