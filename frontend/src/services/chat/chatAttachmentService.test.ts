import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { chatAttachmentService } from "./chatAttachmentService.ts";

const chat: ChatMeta = {
  id: "chat-1",
  title: "Chat",
  cwd: "/workspace/project/",
  createdAt: 1,
  lastMessageAt: 1,
};

const project: ProjectMeta = {
  id: "project-1",
  name: "Project",
  slug: "project",
  cwd: "/workspace",
  containerName: "project",
  status: "running",
  createdAt: 1,
  updatedAt: 1,
};

test("preserves attachment storage paths and collision-safe names", () => {
  assert.equal(chatAttachmentService.basePath(chat, []), "/workspace/project/.uploads");
  assert.equal(
    chatAttachmentService.basePath({ ...chat, projectId: project.id }, [project]),
    "/workspace/.uploads"
  );
  assert.equal(chatAttachmentService.uniqueUploadName("folder/image.png", "abc"), "image-abc.png");
  assert.equal(
    chatAttachmentService.absoluteUploadPath("/workspace/.uploads/", "folder/image-abc.png"),
    "/workspace/.uploads/image-abc.png"
  );
});

test("promptWithAttachments prefixes the user message with the path list", () => {
  assert.equal(
    chatAttachmentService.promptWithAttachments("hello", ["/a.png", "/b.txt"]),
    "hello\n\nAttached files:\n- /a.png\n- /b.txt",
  );
});

test("promptWithAttachments returns the user message verbatim with no paths", () => {
  assert.equal(chatAttachmentService.promptWithAttachments("hello", []), "hello");
  assert.equal(chatAttachmentService.promptWithAttachments("", []), "");
});

test("promptWithAttachments renders the attachment block alone when the user message is empty", () => {
  assert.equal(
    chatAttachmentService.promptWithAttachments("", ["/a.png"]),
    "Attached files:\n- /a.png",
  );
});

test("parseAttachedPaths returns the message and the paths it carries", () => {
  const result = chatAttachmentService.parseAttachedPaths(
    "describe this picture\n\nAttached files:\n- /tmp/a.png\n- /tmp/b.txt",
  );
  assert.equal(result.message, "describe this picture");
  assert.deepEqual(result.paths, ["/tmp/a.png", "/tmp/b.txt"]);
});

test("parseAttachedPaths handles an attachment-only message", () => {
  const result = chatAttachmentService.parseAttachedPaths(
    "Attached files:\n- /tmp/a.png",
  );
  assert.equal(result.message, "");
  assert.deepEqual(result.paths, ["/tmp/a.png"]);
});

test("parseAttachedPaths leaves user text intact when no header is present", () => {
  const text = "describe /tmp/a.png without the marker";
  const result = chatAttachmentService.parseAttachedPaths(text);
  assert.equal(result.message, text);
  assert.deepEqual(result.paths, []);
});

test("parseAttachedPaths ignores a header that is not followed by bullets", () => {
  const text = "Attached files:\nare stored under .uploads";
  const result = chatAttachmentService.parseAttachedPaths(text);
  assert.equal(result.message, text);
  assert.deepEqual(result.paths, []);
});
