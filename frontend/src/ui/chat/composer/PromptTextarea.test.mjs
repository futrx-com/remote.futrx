import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";
import ts from "typescript";

// The Node test runner strips types but does not transform TSX.
const { outputText } = ts.transpileModule(
  readFileSync(new URL("./PromptTextarea.tsx", import.meta.url), "utf8"),
  { compilerOptions: { jsx: ts.JsxEmit.ReactJSX, jsxImportSource: "preact", module: ts.ModuleKind.CommonJS } },
);
const exports = {};
new Function("require", "exports", outputText)(createRequire(import.meta.url), exports);

function press(overrides = {}, handled = false, touchInput = false) {
  let sends = 0;
  let prevented = false;
  const previousWindow = globalThis.window;
  globalThis.window = {
    matchMedia(query) {
      assert.equal(query, "(pointer: coarse)");
      return { matches: touchInput };
    },
  };
  try {
    const textarea = exports.PromptTextarea({
      textareaRef: { current: null }, text: "Hello", uploading: false,
      streaming: false, disconnected: false,
      onTextChange() {}, onPaste() {},
      onSend() { sends++; },
      onKeyDown() { return handled; },
    });
    textarea.props.onKeyDown({
      key: "Enter", ctrlKey: false, metaKey: false, shiftKey: false, isComposing: false,
      preventDefault() { prevented = true; }, ...overrides,
    });
    return { sends, prevented, props: textarea.props };
  } finally {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
  }
}

test("Enter sends and prevents a newline", () => {
  const result = press();
  assert.equal(result.sends, 1);
  assert.equal(result.prevented, true);
});

test("Shift+Enter preserves native newline behavior", () => {
  const result = press({ shiftKey: true });
  assert.equal(result.sends, 0);
  assert.equal(result.prevented, false);
});

test("Command+Enter and Ctrl+Enter still send", () => {
  for (const event of [{ metaKey: true }, { ctrlKey: true }]) {
    const result = press(event);
    assert.equal(result.sends, 1);
    assert.equal(result.prevented, true);
  }
});

test("composition and ordinary typing never send", () => {
  for (const event of [{ isComposing: true }, { key: "a" }]) {
    const result = press(event);
    assert.equal(result.sends, 0);
    assert.equal(result.prevented, false);
  }
});

test("command palette consumption takes priority over sending", () => {
  const result = press({}, true);
  assert.equal(result.sends, 0);
  assert.equal(result.prevented, false);
});

test("keyboard hints advertise Enter to send", () => {
  const { props } = press();
  assert.equal(props.enterkeyhint, "send");
  assert.equal(props["aria-keyshortcuts"], "Enter Control+Enter Meta+Enter");
});

test("Return on a touch device preserves native newline behavior", () => {
  const result = press({}, false, true);
  assert.equal(result.sends, 0);
  assert.equal(result.prevented, false);
  assert.equal(result.props.enterkeyhint, "enter");
  assert.equal(result.props["aria-keyshortcuts"], "Control+Enter Meta+Enter");
});

test("touch devices with an attached keyboard can send with Command/Ctrl+Enter", () => {
  for (const event of [{ metaKey: true }, { ctrlKey: true }]) {
    const result = press(event, false, true);
    assert.equal(result.sends, 1);
    assert.equal(result.prevented, true);
  }
});

test("touch keyboard shortcuts preserve composition, Shift+Enter, and palette handling", () => {
  for (const [event, handled] of [
    [{ ctrlKey: true, isComposing: true }, false],
    [{ metaKey: true, shiftKey: true }, false],
    [{ ctrlKey: true }, true],
  ]) {
    const result = press(event, handled, true);
    assert.equal(result.sends, 0);
    assert.equal(result.prevented, false);
  }
});
