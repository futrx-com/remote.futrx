import assert from "node:assert/strict";
import test from "node:test";
import {
  invitationCredentialFileService,
  UnsupportedCredentialTextError,
} from "./invitationCredentialFileService.ts";

const details = {
  email: "new.member@example.com",
  temporaryPassword: "K7M2P-X9QRT-4WBHN-D8FVC",
  signInUrl: "https://remote.example.com",
};
const generatedAt = new Date(2026, 9, 17);

function decode(bytes: Uint8Array): string {
  return new TextDecoder("latin1").decode(bytes);
}

function build(overrides: Partial<typeof details> = {}) {
  return invitationCredentialFileService.build({ ...details, ...overrides }, generatedAt);
}

test("the file is a dated single-page pdf", () => {
  const file = build();
  const pdf = decode(file.content);

  assert.equal(file.filename, "remote.futrx-credentials-2026-10-17.pdf");
  assert.equal(file.mimeType, "application/pdf");
  assert.ok(pdf.startsWith("%PDF-1.4\n"));
  assert.ok(pdf.endsWith("%%EOF\n"));
  assert.ok(pdf.includes("/Type /Pages /Kids [3 0 R] /Count 1"));
});

test("the email, password and address appear in the page text exactly as given", () => {
  const pdf = decode(build().content);
  for (const text of [details.email, details.temporaryPassword, details.signInUrl]) {
    assert.ok(pdf.includes(`(${text}) Tj`), `expected ${text} unescaped in the content stream`);
  }
  assert.ok(pdf.includes("choose a new password"));
});

test("the cross-reference table points at the real object offsets", () => {
  const pdf = decode(build().content);

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

test("the declared stream length is the real one", () => {
  const pdf = decode(build().content);
  const declared = Number(/<< \/Length (\d+) >>/.exec(pdf)?.[1]);
  const start = pdf.indexOf("stream\n") + "stream\n".length;
  assert.equal(pdf.indexOf("\nendstream") - start, declared);
});

test("a password with parentheses and backslashes cannot break out of its string", () => {
  const pdf = decode(build({ temporaryPassword: "a(b)c\\d-padding-1234" }).content);
  assert.ok(pdf.includes("(a\\(b\\)c\\\\d-padding-1234) Tj"));
});

test("a non-ASCII email is refused rather than garbled", () => {
  for (const email of ["josé@exemple.fr", "用户@example.com", "ab\u0000c@example.com"]) {
    assert.throws(() => build({ email }), UnsupportedCredentialTextError);
    assert.ok(invitationCredentialFileService.unsupportedReason({ email }));
  }
});

test("a non-ASCII password or address is refused too", () => {
  assert.throws(() => build({ temporaryPassword: "pässword-123456789" }), UnsupportedCredentialTextError);
  assert.throws(() => build({ signInUrl: "https://exämple.com" }), UnsupportedCredentialTextError);
});

test("an ASCII email has no reason to refuse", () => {
  assert.equal(invitationCredentialFileService.unsupportedReason({ email: details.email }), null);
});
