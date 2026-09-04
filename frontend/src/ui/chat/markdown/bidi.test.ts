import assert from "node:assert/strict";
import test from "node:test";
import { getTextDirection, getTextAlignClass, isRtlText, hasLtrText, splitBidiSegments } from "./bidi.ts";

test("identifies pure Arabic text as RTL", () => {
  const text = "هذا اختبار لعرض النص العربي بشكل صحيح داخل المحادثة.";
  assert.equal(isRtlText(text), true);
  assert.equal(hasLtrText(text), false);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("identifies pure English text as LTR", () => {
  const text = "This is a normal English message with Markdown rendering.";
  assert.equal(isRtlText(text), false);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "ltr");
  assert.equal(getTextAlignClass(text), "text-left");
});

test("identifies mixed Arabic and English as RTL", () => {
  const text = "يمكن استخدام Angular للواجهة الأمامية و Python مع FastAPI للواجهة الخلفية.";
  assert.equal(isRtlText(text), true);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("identifies Arabic with brackets and numbers as RTL", () => {
  const text = "نستخدم OpenID Connect (OIDC) مع OAuth 2.0 لتسجيل الدخول.";
  assert.equal(isRtlText(text), true);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("identifies Arabic with inline code and paths as RTL", () => {
  const text = "شغل الأمر `npm run build` ثم تحقق من ملف `/var/www/app/frontend`.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("handles empty and undefined gracefully", () => {
  assert.equal(isRtlText(""), false);
  assert.equal(isRtlText(null), false);
  assert.equal(isRtlText(undefined), false);
  assert.equal(hasLtrText(""), false);
  assert.equal(getTextDirection(""), "ltr");
  assert.equal(getTextAlignClass(""), "text-left");
});

test("splitBidiSegments extracts coherent English technical terms in Arabic text", () => {
  const text = "يمكن استخدام Angular للواجهة الأمامية و Python مع FastAPI للواجهة الخلفية.";
  const segs = splitBidiSegments(text);
  const ltrTerms = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltrTerms, ["Angular", "Python", "FastAPI"]);
});

test("splitBidiSegments preserves coherent complex English phrases with punctuation", () => {
  const text = "نظام تسجيل الدخول باستخدام OAuth 2.0 / OpenID Connect (OIDC) مناسب للشركة";
  const segs = splitBidiSegments(text);
  const ltrTerms = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltrTerms, ["OAuth 2.0 / OpenID Connect (OIDC)"]);
});

test("splitBidiSegments handles Single Page Application - SPA", () => {
  const text = "يتم بناء الواجهة باستخدام Angular لتجربة Single Page Application - SPA سريعة.";
  const segs = splitBidiSegments(text);
  const ltrTerms = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltrTerms, ["Angular", "Single Page Application - SPA"]);
});

test("splitBidiSegments isolates parentheses around English phrase", () => {
  const text = "هو إطار عمل للواجهة الأمامية (Frontend / Client-side UI).";
  const segs = splitBidiSegments(text);
  const ltrTerms = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltrTerms, ["(Frontend / Client-side UI)"]);
});

test("splitBidiSegments leaves pure Arabic unmodified", () => {
  const text = "هذا نص عربي بالكامل.";
  const segs = splitBidiSegments(text);
  assert.equal(segs.length, 1);
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[0].text, text);
});
