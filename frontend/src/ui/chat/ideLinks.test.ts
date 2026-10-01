import assert from "node:assert/strict";
import test from "node:test";
import { builtinWorkspaceFileUrl, openFilePayload } from "./ideLinks.ts";

test("openFilePayload targets a file at line and column", () => {
  const payload = openFilePayload("code.remote.futrx.dev", "/workspace/docs/flow.md", 92, 5);
  assert.deepEqual(JSON.parse(payload), [
    ["openFile", "vscode-remote://code.remote.futrx.dev/workspace/docs/flow.md:92:5"],
    ["gotoLineMode", "true"],
  ]);
});

test("openFilePayload with a line only omits the column", () => {
  const payload = openFilePayload("code.remote.futrx.dev", "/workspace/docs/flow.md", 92);
  assert.deepEqual(JSON.parse(payload), [
    ["openFile", "vscode-remote://code.remote.futrx.dev/workspace/docs/flow.md:92"],
    ["gotoLineMode", "true"],
  ]);
});

test("openFilePayload without a line skips gotoLineMode", () => {
  const payload = openFilePayload("code.remote.futrx.dev", "/workspace/README.md");
  assert.deepEqual(JSON.parse(payload), [
    ["openFile", "vscode-remote://code.remote.futrx.dev/workspace/README.md"],
  ]);
});

test("openFilePayload percent-encodes special path segments", () => {
  const payload = openFilePayload("code.remote.futrx.dev", "/workspace/a b/no#te.md", 3);
  const [openFile] = JSON.parse(payload) as [string, string][];
  assert.equal(openFile[1], "vscode-remote://code.remote.futrx.dev/workspace/a%20b/no%23te.md:3");
});


test("built-in fallback preserves project file links before the editor migration", () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "location");
  Object.defineProperty(globalThis, "location", {
    configurable: true,
    value: { origin: "https://remote.example.test" },
  });
  try {
    const url = new URL(builtinWorkspaceFileUrl({
      cwd: "/var/lib/remote/projects/example/workspace/src",
      path: "/workspace/src/main.ts",
      line: 12,
      column: 3,
    })!);
    assert.equal(url.origin, "https://code.remote.example.test");
    assert.equal(url.pathname, "/example/");
    assert.deepEqual(JSON.parse(url.searchParams.get("payload")!), [
      ["openFile", "vscode-remote://code.remote.example.test/workspace/src/main.ts:12:3"],
      ["gotoLineMode", "true"],
    ]);
  } finally {
    if (previous) Object.defineProperty(globalThis, "location", previous);
    else Reflect.deleteProperty(globalThis, "location");
  }
});
