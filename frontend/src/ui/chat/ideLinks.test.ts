import assert from "node:assert/strict";
import test from "node:test";
import { buildIdeUrl, openFilePayload } from "./ideLinks.ts";

test("project IDE links stay on the main site", () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "location");
  Object.defineProperty(globalThis, "location", {
    configurable: true,
    value: { origin: "https://remote.example.test" },
  });
  try {
    const raw = buildIdeUrl(
      "/var/lib/remote/projects/example/workspace",
      "/var/lib/remote/projects/example/workspace/src/App.tsx",
      12,
    );
    const url = new URL(raw);
    assert.equal(url.origin + url.pathname, "https://remote.example.test/example/code/");
    assert.equal(url.searchParams.get("folder"), "/workspace");
    assert.match(url.searchParams.get("payload") || "", /vscode-remote:\/\/remote.example.test\/workspace\/src\/App.tsx:12/);
  } finally {
    if (previous) Object.defineProperty(globalThis, "location", previous);
    else Reflect.deleteProperty(globalThis, "location");
  }
});

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
