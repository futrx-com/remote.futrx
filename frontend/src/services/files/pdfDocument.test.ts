import assert from "node:assert/strict";
import test from "node:test";
import { pdfContentStream, pdfDocument } from "./pdfDocument.ts";

test("content stream positions each line and escapes its text", () => {
  const stream = pdfContentStream([
    { text: "a (b) \\ é", x: 56, y: 780, font: "F1", size: 16 },
    { text: "next", x: 10, y: 20, font: "F2", size: 9 },
  ]);
  assert.deepEqual(stream.split("\n"), [
    "BT /F1 16 Tf 1 0 0 1 56 780 Tm (a \\(b\\) \\\\ -) Tj ET",
    "BT /F2 9 Tf 1 0 0 1 10 20 Tm (next) Tj ET",
  ]);
});

test("document records the byte offset of every object in the xref table", () => {
  const doc = pdfDocument(["<< /A 1 >>", "<< /B 2 >>"]);
  const xrefAt = Number(/startxref\n(\d+)\n%%EOF\n$/.exec(doc)![1]);
  assert.ok(doc.startsWith("%PDF-1.4\n"));
  assert.ok(doc.slice(xrefAt).startsWith("xref\n0 3\n"));
  assert.ok(doc.includes("<< /Size 3 /Root 1 0 R >>"));
  const offsets = [...doc.matchAll(/^(\d{10}) 00000 n $/gm)].map((m) => Number(m[1]));
  assert.equal(offsets.length, 2);
  offsets.forEach((offset, i) => {
    assert.ok(doc.slice(offset).startsWith(`${i + 1} 0 obj\n`));
  });
});
