import assert from "node:assert/strict";
import test from "node:test";
import { workspaceLocation } from "../scripts/api/applicationUrls.js";
import activate from "../scripts/main.js";

const cwd = "/var/lib/remote/projects/alpha/workspace/src";
test("only a project workspace maps to the container's /workspace", () => {
  assert.deepEqual(workspaceLocation(cwd), { slug: "alpha", path: "/workspace/src" });
  for (const invalid of ["/tmp/workspace", "/workspace", "", undefined])
    assert.equal(workspaceLocation(invalid), null);
});

test("registers a drawer that needs chat context and this project's backend", () => {
  const previous = globalThis.window;
  globalThis.window = { location: { origin: "https://remote.example.com" } };
  try {
    let drawer;
    activate({
      application: {},
      backend: { instances: [{ instanceId: "i1", scope: "project", projectId: "p1" }] },
      ui: {
        registerDrawer: (options) => {
          drawer = options;
        },
      },
    });
    assert.equal(drawer.id, "files");
    assert.equal(drawer.when({ projectId: "p1", chatId: "c1", cwd }), true);
    // Another project's chat must not reach this project's files.
    assert.equal(drawer.when({ projectId: "p2", chatId: "c2", cwd }), false);
    assert.equal(drawer.when({ projectId: "p1", chatId: "c1", cwd: "/tmp" }), false);
    assert.equal(drawer.when({ projectId: "p1", cwd }), false);
    assert.equal(drawer.when({ chatId: "c1", cwd }), false);
  } finally {
    globalThis.window = previous;
  }
});
