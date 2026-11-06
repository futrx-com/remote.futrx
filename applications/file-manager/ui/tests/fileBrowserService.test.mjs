import assert from "node:assert/strict";
import test from "node:test";
import { createFileBrowser } from "../scripts/services/fileBrowserService.js";

import { createFileBrowserState } from "../scripts/state/fileBrowserState.js";

const browserWith = (api, changed) => createFileBrowser(api, createFileBrowserState(changed));

const settle = () => new Promise((resolve) => setImmediate(resolve));

test("reset keeps notification order and ignores an older directory response", async () => {
  const requests = [];
  const changes = [];
  const api = {
    list: (path, signal) => new Promise((resolve) => requests.push({ path, signal, resolve })),
  };
  const browser = browserWith(api, () => changes.push({
    loading: browser.state.rootLoading,
    entries: browser.state.rootEntries,
    subtitle: browser.subtitle(),
  }));
  browser.reset();
  browser.reset();
  assert.equal(requests[0].signal.aborted, true);
  assert.deepEqual(changes.map(({ loading }) => loading), [false, true, true, true]);
  requests[0].resolve({ entries: ["old"], truncated: true });
  await settle();
  assert.equal(changes.length, 4);
  assert.deepEqual(browser.state.rootEntries, []);
  requests[1].resolve({ entries: ["current"], truncated: true });
  await settle();
  assert.deepEqual(changes.at(-1), { loading: false, entries: ["current"], subtitle: "workspace" });
  assert.equal(browser.state.truncatedDirs.has(""), true);
  browser.dispose();
  assert.equal(requests[1].signal.aborted, true);
});

test("directory errors finish loading and blocked-link dismissal notifies in a microtask", async () => {
  let changes = 0;
  const browser = browserWith({ list: async () => { throw new Error("offline"); } }, () => changes++);
  browser.reset();
  await settle();
  assert.equal(browser.state.rootLoading, false);
  assert.equal(browser.state.rootError, "offline");
  assert.equal(browser.state.errorByDir.get(""), "offline");
  browser.showBlocked("a.txt", "https://editor.test");
  const before = changes;
  browser.dismissBlocked();
  assert.equal(browser.state.blocked, null);
  assert.equal(changes, before);
  await settle();
  assert.equal(changes, before + 1);
  browser.dispose();
});
