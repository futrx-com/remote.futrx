import assert from "node:assert/strict";
import test from "node:test";
import { createFileOpeningService } from "../scripts/services/fileOpeningService.js";
import { createRemoteFilesApi } from "../scripts/api/remoteFilesApi.js";
import { createFileUrls } from "../scripts/api/fileUrls.js";

const context = {
  chatId: "c1",
  projectId: "p1",
  cwd: "/var/lib/remote/projects/alpha/workspace",
};
const urls = createFileUrls("https://remote.example.com");

test("the core API handles the file and supplies the blocked-tab link without a status request", () => {
  const requests = [];
  const blocked = [];
  let checks = 0;
  const remote = {
    files: {
      open(request) {
        requests.push(request);
        return "blocked";
      },
      editorUrl: () => "https://editor.example.com/open",
    },
  };
  const open = createFileOpeningService({
    remoteFiles: createRemoteFilesApi(remote),
    codeServerRunning: async () => { checks++; return true; },
    urls,
    navigation: {},
  }, context, (path, url) => blocked.push({ path, url }));

  open("src/app.go");
  assert.deepEqual(requests, [{ ...context, path: "/workspace/src/app.go" }]);
  assert.deepEqual(blocked, [{ path: "src/app.go", url: "https://editor.example.com/open" }]);
  assert.equal(checks, 0);
});

test("legacy clicks use the cached status, notify blocked tabs, then refresh for the next click", async () => {
  const detections = [];
  const events = [];
  const open = createFileOpeningService({
    remoteFiles: createRemoteFilesApi({}),
    codeServerRunning(projectId) {
      assert.equal(projectId, "p1");
      events.push(["check"]);
      return new Promise((resolve) => detections.push(resolve));
    },
    urls,
    navigation: {
      claimLink(url) { events.push(["link", url]); return false; },
      openTab(url) { events.push(["tab", url]); return false; },
      download(url) { events.push(["download", url]); },
    },
  }, context, (path, url) => events.push(["blocked", path, url]));

  assert.deepEqual(events, [["check"]]);
  events.length = 0;
  // The mount check is still pending: offer Scratch Tab, then download.
  open("src/app.go");
  assert.deepEqual(events, [
    ["link", urls.scratch("c1", "/workspace/src/app.go")],
    ["download", urls.download("c1", "src/app.go")],
    ["check"],
  ]);

  detections[0](false);
  detections[1](true);
  await Promise.resolve();
  events.length = 0;
  const editor = urls.editor(context.cwd, "/workspace/src/app.go");
  open("src/app.go");
  assert.deepEqual(events, [
    ["link", editor],
    ["tab", editor],
    ["blocked", "src/app.go", editor],
    ["check"],
  ]);

  detections[2](false);
  await Promise.resolve();
  events.length = 0;
  open("src/app.go");
  assert.deepEqual(events, [
    ["link", urls.scratch("c1", "/workspace/src/app.go")],
    ["download", urls.download("c1", "src/app.go")],
    ["check"],
  ]);
  detections[3](false);
});
