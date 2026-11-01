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

test("openAction offers source and text files to an application", () => {
  assert.deepEqual(fileService.openAction("main.go"), { action: "application" });
  assert.deepEqual(fileService.openAction("data.json"), { action: "application" });
  assert.deepEqual(fileService.openAction("README.md"), { action: "application" });
  assert.deepEqual(fileService.openAction("Makefile"), { action: "application" });
});

test("openAction downloads archives and unrenderable media", () => {
  assert.deepEqual(fileService.openAction("bundle.zip"), { action: "download" });
  assert.deepEqual(fileService.openAction("clip.mkv"), { action: "download" });
  assert.deepEqual(fileService.openAction("photo.heic"), { action: "download" });
});

test("formats attachment sizes compactly", () => {
  assert.equal(fileService.formatBytesCompact(980), "980B");
  assert.equal(fileService.formatBytesCompact(4300), "4 KB");
  assert.equal(fileService.formatBytesCompact(132_120_576), "126.0 MB");
});
