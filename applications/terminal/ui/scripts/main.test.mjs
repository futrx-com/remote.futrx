import assert from "node:assert/strict";
import test from "node:test";
import activate from "./main.js";
import { mountTerminal } from "./controllers/terminalDrawerController.js";
import { workspaceLocation } from "./workspacePath.js";

const PROJECT_CWD = "/var/lib/remote/projects/example/workspace";

test("the host workspace path is shown as the container's /workspace", () => {
  assert.deepEqual(workspaceLocation(PROJECT_CWD), { slug: "example", path: "/workspace" });
  assert.deepEqual(
    workspaceLocation(`${PROJECT_CWD}/services/api`),
    { slug: "example", path: "/workspace/services/api" },
  );
});

test("a directory outside a project workspace has no terminal", () => {
  for (const cwd of [undefined, "", "~", "/opt/remote.futrx", "/workspace",
    "/var/lib/remote/projects/example", "/var/lib/remote/projects/Example/workspace",
    "/var/lib/remote/projects/example/workspaces"]) {
    assert.equal(workspaceLocation(cwd), null, String(cwd));
  }
});

function register(remoteOverrides = {}) {
  let drawer;
  activate({
    application: { id: "terminal" },
    backend: { instances: [{ instanceId: "i1", scope: "project", projectId: "p1" }] },
    ui: { registerDrawer: (options) => { drawer = options; return () => {}; } },
    ...remoteOverrides,
  });
  return drawer;
}

test("the drawer is offered only for a chat in a project with a running terminal", () => {
  const drawer = register();
  assert.equal(drawer.id, "shell");
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p1", cwd: PROJECT_CWD }), true);
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p2", cwd: PROJECT_CWD }), false);
  assert.equal(drawer.when({ chatId: "abc123", cwd: PROJECT_CWD }), false);
  assert.equal(drawer.when({ projectId: "p1", cwd: PROJECT_CWD }), false);
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p1", cwd: "/opt/remote.futrx" }), false);
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p1" }), false);
});

test("a Remote without drawers is left untouched", () => {
  assert.doesNotThrow(() => activate({ application: { id: "terminal" }, ui: {} }));
});

test("mounting connects the chat's shell and labels it with the container path", (t) => {
  const listeners = new Map();
  globalThis.window = {
    addEventListener: (name, listener) => listeners.set(name, listener),
    removeEventListener: (name) => listeners.delete(name),
  };
  t.after(() => { delete globalThis.window; });
  const calls = [];
  const statuses = [];
  let options;
  let viewHandlers;
  const view = {
    size: () => ({ cols: 80, rows: 24 }),
    write: () => {}, reset: () => {}, fitAndResize: () => {},
    notice: (text) => calls.push(["notice", text]),
    dispose: () => calls.push(["view.dispose"]),
  };
  const cleanup = mountTerminal("body", {
    chatId: "abc123", projectId: "p1", cwd: `${PROJECT_CWD}/api`,
    setStatus: (status) => statuses.push(status),
  }, {
    reachable: true,
    createApi: () => "api",
    createView: (body, handlers) => { viewHandlers = handlers; return view; },
    createConnection: (given) => {
      options = given;
      return {
        start: () => calls.push(["start"]),
        input: (data) => calls.push(["input", data]),
        resize: (size) => calls.push(["resize", size]),
        setVisible: (shown) => calls.push(["visible", shown]),
        resume: () => calls.push(["resume"]),
        dispose: () => calls.push(["connection.dispose"]),
      };
    },
  });
  assert.equal(options.api, "api");
  assert.equal(options.session, "abc123");
  assert.equal(options.cwd, "/workspace/api");
  for (const status of ["connecting", "connected", "reconnecting", "ended"]) options.onStatus(status);
  assert.deepEqual(statuses, [
    { label: "Connecting - /workspace/api", active: false },
    { label: "Connected - /workspace/api", active: true },
    { label: "Reconnecting - /workspace/api", active: false },
    { label: "Closed - /workspace/api", active: false },
  ]);
  viewHandlers.onData("ls\r");
  viewHandlers.onResize({ cols: 90, rows: 30 });
  viewHandlers.onVisible(true);
  listeners.get("online")();
  options.onEnded();
  cleanup();
  assert.deepEqual(calls, [
    ["start"],
    ["input", "ls\r"],
    ["resize", { cols: 90, rows: 30 }],
    ["visible", true],
    ["resume"],
    ["notice", "Shell exited. Press Enter to start a new one."],
    ["connection.dispose"],
    ["view.dispose"],
  ]);
  assert.equal(listeners.size, 0);
});

test("a chat without a reachable terminal says so instead of connecting", () => {
  const statuses = [];
  const cleanup = mountTerminal("body", { setStatus: (status) => statuses.push(status) }, {
    reachable: false,
    createView: () => assert.fail("a view was created"),
  });
  assert.equal(cleanup, undefined);
  assert.deepEqual(statuses, [{ label: "Unavailable for this chat", active: false }]);
});
