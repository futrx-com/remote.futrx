import assert from "node:assert/strict";
import test from "node:test";
import { ideWindowName, openIdeWindow } from "./ideWindow.js";

function fakeWindow() {
  return { closed: false, opener: {}, focused: 0, focus() { this.focused += 1; } };
}

// Runs `body` with a window whose open() hands back `fake` and records the
// arguments it was called with.
function withWindow(fake, body) {
  const previous = globalThis.window;
  const calls = [];
  globalThis.window = {
    open: (...args) => { calls.push(args); return fake; },
  };
  try {
    body(calls);
  } finally {
    if (previous) globalThis.window = previous;
    else delete globalThis.window;
  }
}

test("the editor window is named after the origin it runs on", () => {
  assert.equal(
    ideWindowName("https://code--example.remote.example.test/?folder=%2Fworkspace"),
    "futrx-ide-code--example.remote.example.test",
  );
  assert.equal(
    ideWindowName("https://code--other-project.remote.example.test:8443/?folder=%2Fworkspace"),
    "futrx-ide-code--other-project.remote.example.test:8443",
  );
});

test("the editor opens in a named window that drops the opener reference", () => {
  const url = "https://code--named.remote.example.test/?folder=%2Fworkspace";
  const fake = fakeWindow();
  withWindow(fake, (calls) => {
    openIdeWindow(url);
    assert.deepEqual(calls, [[url, "futrx-ide-code--named.remote.example.test"]]);
    assert.equal(fake.opener, null);
  });
});

test("a second click focuses the editor that is already open", () => {
  const url = "https://code--focused.remote.example.test/?folder=%2Fworkspace";
  const fake = fakeWindow();
  withWindow(fake, (calls) => {
    openIdeWindow(url);
    openIdeWindow(url);
    assert.equal(calls.length, 1);
    assert.equal(fake.focused, 1);
  });
});

test("a closed editor window is opened again", () => {
  const url = "https://code--closed.remote.example.test/?folder=%2Fworkspace";
  const fake = fakeWindow();
  withWindow(fake, (calls) => {
    openIdeWindow(url);
    fake.closed = true;
    openIdeWindow(url);
    assert.equal(calls.length, 2);
    assert.equal(fake.focused, 0);
  });
});

test("every project keeps its own editor window", () => {
  const first = fakeWindow();
  const second = fakeWindow();
  const previous = globalThis.window;
  const calls = [];
  globalThis.window = {
    open: (...args) => {
      calls.push(args);
      return args[0].includes("first") ? first : second;
    },
  };
  try {
    openIdeWindow("https://code--first.remote.example.test/?folder=%2Fworkspace");
    openIdeWindow("https://code--second.remote.example.test/?folder=%2Fworkspace");
    assert.equal(calls.length, 2);
    assert.equal(first.focused, 0);
    assert.equal(second.focused, 0);
    openIdeWindow("https://code--first.remote.example.test/?folder=%2Fworkspace");
    assert.equal(calls.length, 2);
    assert.equal(first.focused, 1);
  } finally {
    if (previous) globalThis.window = previous;
    else delete globalThis.window;
  }
});
