import assert from "node:assert/strict";
import test from "node:test";
import activate, { workspaceIdeUrl } from "./main.js";

test("Code Server URL uses the project route and selected chat directory", () => {
  assert.equal(
    workspaceIdeUrl("/var/lib/remote/projects/example/workspace/src", "https://remote.example.test"),
    "https://remote.example.test/example/code/?folder=%2Fworkspace%2Fsrc",
  );
  assert.equal(workspaceIdeUrl("/opt/remote.futrx", "https://remote.example.test"), null);
});

test("editor icon is contributed only for a project workspace", () => {
  let button;
  activate({
    application: { id: "code-server" },
    slots: { chatHeaderActions: "chat.header.actions", applicationCardActions: "applications.card.actions" },
    ui: {
      addIconButton: (_slot, contribution) => { button = contribution; },
      addButton: () => {},
    },
  });
  globalThis.window = { location: { origin: "https://remote.example.test" }, open: (...args) => { globalThis.opened = args; } };
  try {
    assert.equal(button.when({ projectId: "p1", cwd: "/var/lib/remote/projects/example/workspace" }), true);
    assert.equal(button.when({ projectId: "p1", cwd: "/opt/remote.futrx" }), false);
    assert.equal(button.when({ cwd: "/var/lib/remote/projects/example/workspace" }), false);
    button.onClick({ cwd: "/var/lib/remote/projects/example/workspace" });
    assert.equal(globalThis.opened[0], "https://remote.example.test/example/code/?folder=%2Fworkspace");
  } finally {
    delete globalThis.window;
    delete globalThis.opened;
  }
});

test("settings action belongs only to a running Code Server installation", () => {
  let action;
  activate({
    application: { id: "code-server" },
    slots: { chatHeaderActions: "chat.header.actions", applicationCardActions: "applications.card.actions" },
    ui: { addIconButton: () => {}, addButton: (_slot, contribution) => { action = contribution; } },
  });
  assert.equal(action.when({ instance: { applicationId: "code-server", status: "running" } }), true);
  assert.equal(action.when({ instance: { applicationId: "code-server", status: "stopped" } }), false);
  assert.equal(action.when({ instance: { applicationId: "other", status: "running" } }), false);
});
