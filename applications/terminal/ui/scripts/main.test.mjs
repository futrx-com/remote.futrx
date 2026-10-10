import assert from "node:assert/strict";
import test from "node:test";
import activate from "./main.js";
import { terminalUrl, workspaceLocation } from "./workspacePath.js";

const ORIGIN = "https://remote.example.test";
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
    assert.equal(terminalUrl(cwd, "abc123", "terminal", ORIGIN), null, String(cwd));
  }
});

test("the terminal page is addressed by project subdomain, chat and container path", () => {
  const url = new URL(terminalUrl(`${PROJECT_CWD}/a b`, "abc123", "terminal", `${ORIGIN}/c/abc123?x=1#top`));
  assert.equal(url.origin, "https://terminal--example.remote.example.test");
  assert.equal(url.pathname, "/");
  assert.equal(url.hash, "");
  assert.deepEqual([...url.searchParams.keys()], ["session", "cwd"]);
  assert.equal(url.searchParams.get("session"), "abc123");
  assert.equal(url.searchParams.get("cwd"), "/workspace/a b");
});

test("a host name the gateway could not route yields no terminal", () => {
  assert.equal(terminalUrl(PROJECT_CWD, "", "terminal", ORIGIN), null);
  assert.equal(terminalUrl(PROJECT_CWD, "abc123", undefined, ORIGIN), null);
  assert.equal(terminalUrl(PROJECT_CWD, "abc123", "Term", ORIGIN), null);
  assert.equal(terminalUrl(PROJECT_CWD, "abc123", "a--b", ORIGIN), null);
  const longSlug = "a".repeat(60);
  assert.equal(
    terminalUrl(`/var/lib/remote/projects/${longSlug}/workspace`, "abc123", "terminal", ORIGIN),
    null,
  );
  assert.equal(
    terminalUrl("/var/lib/remote/projects/a--b/workspace", "abc123", "terminal", ORIGIN),
    null,
  );
});

function fakeDom(t) {
  const listeners = new Set();
  const frame = { style: {}, contentWindow: {}, removed: false, remove() { this.removed = true; } };
  globalThis.window = {
    location: { origin: ORIGIN },
    addEventListener: (_name, listener) => listeners.add(listener),
    removeEventListener: (_name, listener) => listeners.delete(listener),
  };
  globalThis.document = { createElement: () => frame };
  t.after(() => {
    delete globalThis.window;
    delete globalThis.document;
  });
  return { frame, listeners, post: (event) => listeners.forEach((listener) => listener(event)) };
}

function register(remoteOverrides = {}) {
  let drawer;
  activate({
    application: { id: "terminal", web: { subdomain: "terminal" } },
    ui: { registerDrawer: (options) => { drawer = options; return () => {}; } },
    ...remoteOverrides,
  });
  return drawer;
}

test("the drawer is offered only for a chat in a project workspace", (t) => {
  fakeDom(t);
  const drawer = register();
  assert.equal(drawer.id, "shell");
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p1", cwd: PROJECT_CWD }), true);
  assert.equal(drawer.when({ chatId: "abc123", cwd: PROJECT_CWD }), false);
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p1", cwd: "/opt/remote.futrx" }), false);
  assert.equal(drawer.when({ chatId: "abc123", projectId: "p1" }), false);
});

test("a Remote without drawers is left untouched", () => {
  assert.doesNotThrow(() => activate({ application: { id: "terminal" }, ui: {} }));
});

test("mounting frames the terminal page and labels it with the container path", (t) => {
  const dom = fakeDom(t);
  const drawer = register();
  const appended = [];
  const statuses = [];
  const cleanup = drawer.mount(
    { appendChild: (node) => appended.push(node) },
    { chatId: "abc123", projectId: "p1", cwd: `${PROJECT_CWD}/api`, setStatus: (status) => statuses.push(status) },
  );
  assert.deepEqual(appended, [dom.frame]);
  assert.equal(
    dom.frame.src,
    "https://terminal--example.remote.example.test/?session=abc123&cwd=%2Fworkspace%2Fapi",
  );
  assert.deepEqual(statuses, [{ label: "Connecting - /workspace/api", active: false }]);

  const fromPage = { origin: "https://terminal--example.remote.example.test", source: dom.frame.contentWindow };
  dom.post({ ...fromPage, data: { source: "remote-terminal", status: "connected" } });
  dom.post({ ...fromPage, data: { source: "remote-terminal", status: "reconnecting" } });
  dom.post({ ...fromPage, data: { source: "remote-terminal", status: "ended" } });
  assert.deepEqual(statuses.slice(1), [
    { label: "Connected - /workspace/api", active: true },
    { label: "Reconnecting - /workspace/api", active: false },
    { label: "Closed - /workspace/api", active: false },
  ]);

  cleanup();
  assert.equal(dom.frame.removed, true);
  assert.equal(dom.listeners.size, 0);
});

test("status messages from anywhere but the terminal page are ignored", (t) => {
  const dom = fakeDom(t);
  const drawer = register();
  const statuses = [];
  drawer.mount(
    { appendChild: () => {} },
    { chatId: "abc123", projectId: "p1", cwd: PROJECT_CWD, setStatus: (status) => statuses.push(status) },
  );
  const origin = "https://terminal--example.remote.example.test";
  const connected = { source: "remote-terminal", status: "connected" };
  dom.post({ origin: "https://evil.example", source: dom.frame.contentWindow, data: connected });
  dom.post({ origin: "https://terminal--other.remote.example.test", source: dom.frame.contentWindow, data: connected });
  dom.post({ origin, source: {}, data: connected });
  dom.post({ origin, source: dom.frame.contentWindow, data: { source: "someone-else", status: "connected" } });
  dom.post({ origin, source: dom.frame.contentWindow, data: { source: "remote-terminal", status: "constructor" } });
  dom.post({ origin, source: dom.frame.contentWindow, data: null });
  assert.equal(statuses.length, 1, "only the initial status was set");
});

test("a chat the gateway cannot route shows why instead of an empty frame", (t) => {
  fakeDom(t);
  const drawer = register();
  const statuses = [];
  const appended = [];
  const cleanup = drawer.mount(
    { appendChild: (node) => appended.push(node) },
    { chatId: "abc123", projectId: "p1", cwd: "/opt/remote.futrx", setStatus: (status) => statuses.push(status) },
  );
  assert.equal(cleanup, undefined);
  assert.deepEqual(appended, []);
  assert.deepEqual(statuses, [{ label: "Unavailable for this chat", active: false }]);
});
