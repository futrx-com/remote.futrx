import { resolveFileOpener } from "./resolveFileOpener.ts";
import assert from "node:assert/strict";
import test from "node:test";
import { internalPathOpenUrl } from "./workspaceLinkService.ts";
import { fileOpenerStore } from "../../state/stores/files/fileOpenerStore.ts";

test("file links use an installed application's opener", () => {
  const dispose = fileOpenerStore.getState().register("editor", ["p1"], ({ path, line }) =>
    `/apps/project/editor/?file=${encodeURIComponent(path)}&line=${line ?? 0}`);
  try {
    assert.equal(internalPathOpenUrl("/workspace/src/App.tsx:12", {
      chatId: "chat-1", cwd: "/var/lib/remote/projects/project/workspace",
      openFileUrl: (request) => resolveFileOpener(fileOpenerStore.getState().forProject("p1"), request),
    }), "/apps/project/editor/?file=%2Fworkspace%2Fsrc%2FApp.tsx&line=12");
  } finally {
    dispose();
  }
});

test("file links download when no project opener is installed", () => {
  assert.equal(internalPathOpenUrl("/workspace/README.md", {
    chatId: "chat-1", cwd: "/var/lib/remote/projects/project/workspace",
  }), "/api/chats/chat-1/files/download?path=README.md");
});
