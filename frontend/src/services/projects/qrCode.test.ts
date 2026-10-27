import assert from "node:assert/strict";
import test from "node:test";
import { QRGenerator, type QRCodeMatrix } from "./QRGenerator.js";

const qrGenerator = new QRGenerator();

function matrixFingerprint(code: QRCodeMatrix): { darkModules: number; hash: string } {
  let darkModules = 0;
  let hash = 2166136261;

  for (let y = 0; y < code.size; y++) {
    for (let x = 0; x < code.size; x++) {
      const value = code.isDark(x, y);
      darkModules += Number(value);
      hash ^= value ? 49 : 48;
      hash = Math.imul(hash, 16777619);
    }
  }

  return { darkModules, hash: (hash >>> 0).toString(16) };
}

test("encodes a compact known QR symbol deterministically", () => {
  const code = qrGenerator.createMatrix("HELLO WORLD");

  assert.equal(code.size, 21);
  assert.deepEqual(matrixFingerprint(code), { darkModules: 222, hash: "54f84b45" });
  assert.equal(code.isDark(-1, 0), false);
  assert.equal(code.isDark(code.size, 0), false);
});

test("encodes a representative authenticator enrollment URI", () => {
  const code = qrGenerator.createMatrix(
    "otpauth://totp/remote.futrx:user@example.com?secret=JBSWY3DPEHPK3PXP&issuer=remote.futrx",
  );

  assert.equal(code.size, 41);
  assert.deepEqual(matrixFingerprint(code), { darkModules: 866, hash: "ced53afd" });
});

// Both format-information copies must be one of the eight ISO/IEC 18004 medium-ECC
// bit strings, otherwise scanners reject the symbol before reading any data.
const MEDIUM_FORMAT_BITS = [
  "101010000010010", "101000100100101", "101111001111100", "101101101001011",
  "100010111111001", "100000011001110", "100111110010111", "100101010100000",
];

function readFormatBits(code: QRCodeMatrix, positions: [number, number][]): string {
  return positions.map(([x, y]) => (code.isDark(x, y) ? "1" : "0")).reverse().join("");
}

test("writes spec-conformant format information in both copies", () => {
  for (const value of ["HELLO WORLD", "REMOTE FUTRX 2FA", "012345678901234567890123456789"]) {
    const code = qrGenerator.createMatrix(value);
    const last = code.size - 1;
    const primary: [number, number][] = [
      [8, 0], [8, 1], [8, 2], [8, 3], [8, 4], [8, 5], [8, 7], [8, 8],
      [7, 8], [5, 8], [4, 8], [3, 8], [2, 8], [1, 8], [0, 8],
    ];
    const secondary: [number, number][] = [
      [last, 8], [last - 1, 8], [last - 2, 8], [last - 3, 8], [last - 4, 8],
      [last - 5, 8], [last - 6, 8], [last - 7, 8],
      [8, code.size - 7], [8, code.size - 6], [8, code.size - 5],
      [8, code.size - 4], [8, code.size - 3], [8, code.size - 2], [8, last],
    ];

    const bits = readFormatBits(code, primary);
    assert.ok(MEDIUM_FORMAT_BITS.includes(bits), `${value} produced format bits ${bits}`);
    assert.equal(readFormatBits(code, secondary), bits);
    assert.equal(code.isDark(8, code.size - 8), true);
  }
});

test("places version 7 alignment patterns at the coordinates required by the spec", () => {
  const code = qrGenerator.createMatrix("x".repeat(107));

  assert.equal(code.size, 45);
  for (const [x, y] of [[22, 22], [22, 38], [38, 22], [6, 22], [22, 6]]) {
    assert.equal(code.isDark(x, y), true);
    assert.equal(code.isDark(x - 1, y), false);
    assert.equal(code.isDark(x - 2, y), true);
  }
});

test("selects numeric, alphanumeric, and UTF-8 byte modes", () => {
  assert.equal(qrGenerator.createMatrix("012345678901234567890123456789").size, 21);
  assert.equal(qrGenerator.createMatrix("REMOTE FUTRX 2FA").size, 21);
  assert.equal(qrGenerator.createMatrix("تسجيل دخول آمن 🔐").size, 29);
});

test("draws finder patterns at all three required corners", () => {
  const code = qrGenerator.createMatrix("finder-pattern-check");
  const centers = [[3, 3], [code.size - 4, 3], [3, code.size - 4]];

  for (const [x, y] of centers) {
    assert.equal(code.isDark(x, y), true);
    assert.equal(code.isDark(x - 2, y), false);
    assert.equal(code.isDark(x - 3, y), true);
  }
});

test("rejects invalid inputs and data beyond QR Model 2 capacity", () => {
  assert.throws(() => qrGenerator.createMatrix(null as unknown as string), TypeError);
  assert.throws(() => qrGenerator.createMatrix("x".repeat(3000)), /too long/);
});
