import assert from "node:assert/strict";
import test from "node:test";
import { openSettingsForm, validateSettingsText } from "./settingsForm.js";

test("settings form accepts a complete JSON object and rejects malformed input", () => {
  assert.deepEqual(validateSettingsText('{"editor.fontSize": 16, "files.exclude": {"**/.git": true}}'), {
    "editor.fontSize": 16,
    "files.exclude": { "**/.git": true },
  });
  for (const value of ["null", "[]", "123", "{"]) {
    assert.throws(() => validateSettingsText(value));
  }
});

test("settings form loads and saves through the installed app backend", async () => {
  const elements = [];
  globalThis.document = {
    createElement: (tag) => {
      const element = {
        tag, children: [], listeners: {},
        append(...children) { this.children.push(...children); },
        replaceChildren(...children) { this.children = children; },
        setAttribute(name, value) { this[name] = value; },
        addEventListener(name, handler) { this.listeners[name] = handler; },
        removeEventListener(name) { delete this.listeners[name]; },
      };
      elements.push(element);
      return element;
    },
  };
  const calls = [];
  let cleanup;
  const remote = {
    ui: { openPopup: ({ mount }) => { cleanup = mount({ replaceChildren() {} }); } },
    backend: { call: async (path, options) => {
      calls.push([path, options]);
      return { settings: options.method === "POST" ? options.body.settings : '{"editor.fontSize":14}\n' };
    } },
  };
  try {
    openSettingsForm(remote, "install-1");
    await Promise.resolve();
    const editor = elements.find((element) => element.tag === "textarea");
    const form = elements.find((element) => element.tag === "form");
    assert.equal(editor.value, '{"editor.fontSize":14}\n');
    editor.value = '{"editor.fontSize":18}';
    await form.listeners.submit({ preventDefault() {} });
    assert.deepEqual(calls, [
      ["settings", { instanceId: "install-1" }],
      ["settings", { instanceId: "install-1", method: "POST", body: { settings: '{"editor.fontSize":18}' } }],
    ]);
    cleanup();
  } finally {
    delete globalThis.document;
  }
});
