import assert from "node:assert/strict";
import test from "node:test";
import { recoveryCodeFileService } from "./recoveryCodeFileService.ts";

const codes = ["ABCD-EFGH-JKMN", "PQRS-TUVW-XYZ2", "3456-789A-BCDE"];
const generatedAt = new Date(2026, 8, 4);

function decode(bytes: Uint8Array): string {
  return new TextDecoder("latin1").decode(bytes);
}

test("filenames carry the product, the format and the day", () => {
  assert.equal(
    recoveryCodeFileService.filename("txt", generatedAt),
    "remote.futrx-recovery-codes-2026-09-04.txt"
  );
  assert.equal(
    recoveryCodeFileService.filename("pdf", new Date(2026, 0, 9)),
    "remote.futrx-recovery-codes-2026-01-09.pdf"
  );
});

test("the text file lists every code, numbered, one per line", () => {
  const file = recoveryCodeFileService.build(codes, "txt", generatedAt);

  assert.equal(file.filename, "remote.futrx-recovery-codes-2026-09-04.txt");
  assert.equal(file.mimeType, "text/plain;charset=utf-8");

  const lines = decode(file.content).split("\n");
  assert.ok(lines[0].includes("Two-factor recovery codes"));
  assert.ok(lines[1].includes("2026-09-04"));
  for (const [index, code] of codes.entries()) {
    assert.ok(
      lines.includes(` ${index + 1}. ${code}`),
      `expected a numbered line for ${code}`
    );
  }
  assert.ok(decode(file.content).includes("used once"));
});

test("the pdf is a single-page document holding every code", () => {
  const file = recoveryCodeFileService.build(codes, "pdf", generatedAt);
  const pdf = decode(file.content);

  assert.equal(file.filename, "remote.futrx-recovery-codes-2026-09-04.pdf");
  assert.equal(file.mimeType, "application/pdf");
  assert.ok(pdf.startsWith("%PDF-1.4\n"));
  assert.ok(pdf.endsWith("%%EOF\n"));
  assert.ok(pdf.includes("/Type /Pages /Kids [3 0 R] /Count 1"));
  for (const code of codes) {
    assert.ok(pdf.includes(code), `expected ${code} in the content stream`);
  }
});

test("the pdf cross-reference table points at the real object offsets", () => {
  const pdf = decode(recoveryCodeFileService.build(codes, "pdf", generatedAt).content);

  const startxref = Number(pdf.slice(pdf.lastIndexOf("startxref") + 9).trim().split("\n")[0]);
  assert.equal(pdf.slice(startxref, startxref + 4), "xref");

  const offsets = pdf
    .slice(startxref)
    .split("\n")
    .filter((line) => / 00000 n $/.test(line))
    .map((line) => Number(line.slice(0, 10)));
  assert.equal(offsets.length, 6);
  offsets.forEach((offset, index) => {
    assert.equal(pdf.slice(offset, offset + `${index + 1} 0 obj`.length), `${index + 1} 0 obj`);
  });
});

test("the pdf declares the byte length its content stream actually has", () => {
  const pdf = decode(recoveryCodeFileService.build(codes, "pdf", generatedAt).content);

  const declared = Number(/<< \/Length (\d+) >>/.exec(pdf)?.[1]);
  const start = pdf.indexOf("stream\n") + "stream\n".length;
  const end = pdf.indexOf("\nendstream");
  assert.equal(end - start, declared);
});

test("parentheses and backslashes in a code cannot break out of a pdf string", () => {
  const pdf = decode(
    recoveryCodeFileService.build(["A(B)C\\D"], "pdf", generatedAt).content
  );
  assert.ok(pdf.includes("A\\(B\\)C\\\\D) Tj"));
});
