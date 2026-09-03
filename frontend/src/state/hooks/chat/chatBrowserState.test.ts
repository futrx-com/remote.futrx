import assert from "node:assert/strict";
import test from "node:test";
import { chatBrowserState } from "./chatBrowserState.ts";

test("preserves browser element context formatting", () => {
  assert.equal(
    chatBrowserState.formatElementCapture({
      url: "https://example.com",
      selector: "#save",
      tag: "button",
      classes: ["primary", "large"],
      text: "Save",
      styles: { color: "red", display: "" },
    }),
    [
      "[Browser element]",
      "URL: https://example.com",
      "Selector: #save",
      "Tag: button",
      "Classes: primary large",
      "Styles:",
      "- color: red",
      "Text: Save",
      "[/Browser element]",
    ].join("\n")
  );
});
