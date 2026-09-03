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
  assert.deepEqual(matrixFingerprint(code), { darkModules: 816, hash: "2b3d6c2d" });
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
