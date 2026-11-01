import assert from "node:assert/strict";
import test from "node:test";
import { codeServerFileUrl, scratchFileUrl } from "../scripts/api/fileUrls.js";
import { codeServerRunning } from "../scripts/api/projectApplicationsApi.js";
import { fileAction } from "../scripts/services/fileAction.js";
import { fileIdeUrl } from "../../../code-server/ui/scripts/editorUrls.js";

const origin = "https://remote.example.com";

test("Code Server file links match the ones Code Server builds", () => {
  for (const cwd of [
    "/var/lib/remote/projects/alpha/workspace",
    "/var/lib/remote/projects/alpha/workspace/src",
  ]) {
    const request = { cwd, path: "/workspace/src/app.go" };
    assert.equal(
      codeServerFileUrl(cwd, request.path, origin),
      fileIdeUrl(request, true, "code", origin),
    );
  }
  assert.equal(codeServerFileUrl("/tmp", "/workspace/a.go", origin), null);
});

test("files are viewed, edited or downloaded as Remote decides", () => {
  assert.equal(fileAction("/workspace/photo.PNG"), "media");
  assert.equal(fileAction("/workspace/doc.pdf"), "media");
  assert.equal(fileAction("/workspace/src/app.go"), "editor");
  assert.equal(fileAction("/workspace/README"), "editor");
  assert.equal(fileAction("/workspace/build.tar"), "download");
  assert.equal(fileAction("/workspace/clip.mkv"), "download");
});

test("Scratch Tab links name the chat and the container path", () => {
  assert.equal(
    scratchFileUrl("c 1", "/workspace/src/app.go"),
    "/api/chats/c%201/ide-open?scratch=1&path=%2Fworkspace%2Fsrc%2Fapp.go",
  );
});

test("Code Server counts only while it runs in the project", async () => {
  const reply = (instances, ok = true) => async (url) => {
    assert.equal(url, "/api/projects/p1/applications");
    return { ok, json: async () => instances };
  };
  assert.equal(
    await codeServerRunning("p1", reply([{ applicationId: "code-server", status: "running" }])),
    true,
  );
  assert.equal(
    await codeServerRunning("p1", reply([{ applicationId: "code-server", status: "stopped" }])),
    false,
  );
  assert.equal(await codeServerRunning("p1", reply([], false)), false);
  assert.equal(
    await codeServerRunning("p1", async () => {
      throw new Error("offline");
    }),
    false,
  );
  assert.equal(await codeServerRunning(undefined, reply([])), false);
});
