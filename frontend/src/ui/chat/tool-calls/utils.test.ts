import assert from "node:assert/strict";
import test from "node:test";
import {
  DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS,
  READ_TOOL_OUTPUT_PREVIEW_CHARS,
} from "../../../config/chat.ts";
import { toolOutputPreviewLimit } from "./utils.ts";

test("tool output expansion follows each renderer preview limit", () => {
  assert.equal(toolOutputPreviewLimit("Read"), READ_TOOL_OUTPUT_PREVIEW_CHARS);
  assert.equal(toolOutputPreviewLimit("Bash"), DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS);
  assert.equal(toolOutputPreviewLimit("Grep"), DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS);
  assert.equal(toolOutputPreviewLimit("future-tool"), DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS);
  assert.equal(toolOutputPreviewLimit("Edit"), null);
  assert.equal(toolOutputPreviewLimit("MultiEdit"), null);
  assert.equal(toolOutputPreviewLimit("Write"), null);
});
