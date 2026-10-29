import assert from "node:assert/strict";
import test from "node:test";
import { fileManagerUrl } from "../scripts/api/applicationUrls.js";
import activate from "../scripts/main.js";

const cwd = "/var/lib/remote/projects/alpha/workspace/src";
test("routes only a project workspace to its named file-manager origin", () => {
  assert.equal(
    fileManagerUrl(cwd, "chat", "files", "https://remote.example.com/"),
    "https://files--alpha.remote.example.com/",
  );
  for (const invalid of [
    "/tmp/workspace",
    "/workspace",
    "/var/lib/remote/projects/a--b/workspace",
  ])
    assert.equal(
      fileManagerUrl(invalid, "chat", "files", "https://remote.example.com/"),
      null,
    );
  assert.equal(
    fileManagerUrl(cwd, "chat", "bad.name", "https://remote.example.com/"),
    null,
  );
});
test("registers a drawer whose predicate requires project and chat context", () => {
  const previous = globalThis.window;
  globalThis.window = { location: { origin: "https://remote.example.com" } };
  try {
    let drawer;
    activate({
      application: { web: { subdomain: "files" } },
      ui: {
        registerDrawer: (options) => {
          drawer = options;
        },
      },
    });
    assert.equal(drawer.id, "files");
    assert.equal(drawer.when({ projectId: "p1", chatId: "c1", cwd }), true);
    assert.equal(
      drawer.when({ projectId: "p2", chatId: "c2", cwd: "/tmp" }),
      false,
    );
    assert.equal(drawer.when({ chatId: "c1", cwd }), false);
  } finally {
    globalThis.window = previous;
  }
});
