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

test("attachment sizes use the compact format", () => {
  assert.equal(fileService.formatBytesCompact(980), "980B");
  assert.equal(fileService.formatBytesCompact(4300), "4 KB");
  assert.equal(fileService.formatBytesCompact(132_120_576), "126.0 MB");
});
