import assert from "node:assert/strict";
import test from "node:test";
import { extractImageSyntaxes } from "./extractImageSyntax.ts";

test("extracts a single image", () => {
  const text = "Look ![alt text](/path/to/x.png) here";
  const result = extractImageSyntaxes(text);
  assert.equal(result.length, 1);
  assert.deepEqual({ alt: result[0].alt, src: result[0].src }, {
    alt: "alt text",
    src: "/path/to/x.png",
  });
  assert.equal(text.slice(result[0].start, result[0].end), "![alt text](/path/to/x.png)");
});

test("extracts multiple images and ignores literal !", () => {
  const text = "Image ![a](u.png) and ![b](v.jpg) plus just ! ok.";
  const result = extractImageSyntaxes(text);
  assert.equal(result.length, 2);
  assert.deepEqual(result.map((r) => r.src), ["u.png", "v.jpg"]);
});

test("skips unclosed image syntax", () => {
  const text = "Broken ![alt without closing";
  assert.deepEqual(extractImageSyntaxes(text), []);
});

test("skips when there is no ( after ]", () => {
  const text = "broken ![alt]src";
  assert.deepEqual(extractImageSyntaxes(text), []);
});

test("treats escaped ! as literal", () => {
  const text = "literal \\![alt](src.png)";
  const result = extractImageSyntaxes(text);
  assert.equal(result.length, 0);
});
