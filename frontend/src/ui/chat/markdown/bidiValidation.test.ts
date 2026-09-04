import assert from "node:assert/strict";
import test from "node:test";
import { getTextDirection, getTextAlignClass, isRtlText, hasLtrText, splitBidiSegments } from "./bidi.ts";
import { parseMarkdown } from "./blockParser.ts";

test("Case 1: Arabic only", () => {
  const text = "هذا نص عربي بالكامل.";
  assert.equal(isRtlText(text), true);
  assert.equal(hasLtrText(text), false);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
  const segs = splitBidiSegments(text);
  assert.equal(segs.length, 1);
  assert.equal(segs[0].isLtr, false);
});

test("Case 2: English only", () => {
  const text = "This is a normal English message with Markdown rendering.";
  assert.equal(isRtlText(text), false);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "ltr");
  assert.equal(getTextAlignClass(text), "text-left");
});

test("Case 3: Arabic + English", () => {
  const text = "يمكن استخدام Angular للواجهة الأمامية و Python للواجهة الخلفية.";
  assert.equal(isRtlText(text), true);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["Angular", "Python"]);
});

test("Case 4: Arabic with parentheses and OAuth / OIDC", () => {
  const text = "نستخدم OpenID Connect (OIDC) مع OAuth 2.0 لتسجيل الدخول.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["OpenID Connect (OIDC)", "OAuth 2.0"]);
});

test("Case 5: Long English run with slashes and parentheses", () => {
  const text = "الخيار هو OAuth 2.0 / OpenID Connect (OIDC) للمصادقة.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["OAuth 2.0 / OpenID Connect (OIDC)"]);
});

test("Case 6: Arabic list with English technical terms", () => {
  const md = `- الواجهة الأمامية Angular
- الواجهة الخلفية Python
- قاعدة البيانات PostgreSQL`;
  const blocks = parseMarkdown(md);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "list");
  if (blocks[0].type === "list") {
    assert.equal(blocks[0].items.some(item => isRtlText(item.text)), true);
    for (const item of blocks[0].items) {
      assert.equal(isRtlText(item.text), true);
      assert.equal(getTextDirection(item.text), "rtl");
    }
  }
});

test("Case 7: Arabic heading", () => {
  const md = "## إدارة المستخدمين والصلاحيات";
  const blocks = parseMarkdown(md);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "heading");
  if (blocks[0].type === "heading") {
    assert.equal(isRtlText(blocks[0].text), true);
    assert.equal(getTextDirection(blocks[0].text), "rtl");
    assert.equal(getTextAlignClass(blocks[0].text), "text-right");
  }
});

test("Case 8: Mixed table", () => {
  const md = `| العنصر | التقنية |
|---|---|
| الواجهة | Angular |
| Backend | Python / FastAPI |`;
  const blocks = parseMarkdown(md);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "table");
  if (blocks[0].type === "table") {
    // Header cells
    assert.equal(getTextDirection(blocks[0].header[0]), "rtl");
    assert.equal(getTextDirection(blocks[0].header[1]), "rtl");
    // Row 1
    assert.equal(getTextDirection(blocks[0].rows[0][0]), "rtl");
    assert.equal(getTextDirection(blocks[0].rows[0][1]), "ltr");
    // Row 2
    assert.equal(getTextDirection(blocks[0].rows[1][0]), "ltr");
    assert.equal(getTextDirection(blocks[0].rows[1][1]), "ltr");
  }
});

test("Case 9: Technical inline code", () => {
  const text = "شغل الأمر `npm run build` ثم أعد تشغيل الخدمة.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("Case 10: Single Page Application - SPA", () => {
  const text = "يتم بناء الواجهة باستخدام Angular لتجربة Single Page Application - SPA سريعة.";
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["Angular", "Single Page Application - SPA"]);
});

test("Case 11: LDAP / Active Directory", () => {
  const text = "ندعم الربط مع LDAP / Active Directory لإدارة الهويات.";
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["LDAP / Active Directory"]);
});

test("Case 12: Backend API and Django REST Framework", () => {
  const text = "نستخدم Django REST Framework لبناء Backend API موثوق.";
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["Django REST Framework", "Backend API"]);
});

test("Case 13: Bold English inside Arabic with parentheses", () => {
  const text = "**Angular** هو إطار عمل للواجهة الأمامية **(Frontend / Client-side UI)**.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("Case 14: Bold OAuth 2.0 / OpenID Connect (OIDC)", () => {
  const text = "نستخدم **OAuth 2.0 / OpenID Connect (OIDC)** لتسجيل الدخول.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
});

test("Case 15: HTTPS URL in Arabic text", () => {
  const text = "راجع الرابط https://example.com/docs/api لمزيد من التفاصيل.";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  assert.equal(getTextAlignClass(text), "text-right");
});

test("Phase 6 Test 1: RTL list with LTR-only items inside Arabic document context", () => {
  const md = `اطلب منهم التأكد من تضمين:

- email / username / employee_id
- user_type / user_role
- position / job_title
- department / team_id`;

  assert.equal(isRtlText(md), true);
  const blocks = parseMarkdown(md);
  assert.equal(blocks.length, 2);
  assert.equal(blocks[0].type, "paragraph");
  assert.equal(blocks[1].type, "list");

  if (blocks[1].type === "list") {
    // Each item's text is technically English/LTR, but the list belongs to the Arabic context
    assert.equal(blocks[1].items.length, 4);
    for (const item of blocks[1].items) {
      const segs = splitBidiSegments(item.text);
      assert.equal(segs.length, 1);
      assert.equal(segs[0].isLtr, true);
    }
  }
});

test("Phase 6 Test 2: Arabic question with OIDC and SAML", () => {
  const text = "هل النظام الداخلي يعتمد OAuth 2.0 / OpenID Connect (OIDC) أم SAML 2.0؟";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["OAuth 2.0 / OpenID Connect (OIDC)", "SAML 2.0"]);
  // Last segment is the Arabic question mark
  const last = segs[segs.length - 1];
  assert.equal(last.isLtr, false);
  assert.equal(last.text, "؟");
});

test("Phase 6 Test 3: IdP Integration Metadata heading", () => {
  const text = "بيانات الربط التقنية (IdP Integration Metadata):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["(IdP Integration Metadata)"]);
  const last = segs[segs.length - 1];
  assert.equal(last.isLtr, false);
  assert.equal(last.text, ":");
});

test("Phase 6 Test 4: Token Claims / User Attributes heading", () => {
  const text = "هيكل بيانات التوكن (Token Claims / User Attributes):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["(Token Claims / User Attributes)"]);
  const last = segs[segs.length - 1];
  assert.equal(last.isLtr, false);
  assert.equal(last.text, ":");
});

test("Phase 6 Test 5: Role Mapping Matrix heading", () => {
  const text = "مصفوفة الصلاحيات (Role Mapping Matrix):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["(Role Mapping Matrix)"]);
  const last = segs[segs.length - 1];
  assert.equal(last.isLtr, false);
  assert.equal(last.text, ":");
});

test("Phase 6 Test 6: Staging IdP & Test Accounts heading", () => {
  const text = "بيئة تجارب وحسابات اختبارية (Staging IdP & Test Accounts):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["(Staging IdP & Test Accounts)"]);
  const last = segs[segs.length - 1];
  assert.equal(last.isLtr, false);
  assert.equal(last.text, ":");
});

test("Phase 6 Test 7: Inline code in RTL list item", () => {
  const item1 = "استخدام الأمر `npm run build` لتجميع المشروع";
  const item2 = "الحقل `email` مطلوب";
  assert.equal(isRtlText(item1), true);
  assert.equal(isRtlText(item2), true);
  assert.equal(getTextDirection(item1), "rtl");
  assert.equal(getTextDirection(item2), "rtl");
});

test("Phase 6 Test 8: English-only normal paragraph must remain LTR", () => {
  const text = "This is a standard English paragraph without any Arabic content.";
  assert.equal(isRtlText(text), false);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "ltr");
  assert.equal(getTextAlignClass(text), "text-left");
});

test("Phase 6 Test 9: Nested RTL list containing English technical children", () => {
  const md = `- إعدادات المصادقة
  - OAuth 2.0 / OpenID Connect (OIDC)
  - SAML 2.0`;
  assert.equal(isRtlText(md), true);
  const blocks = parseMarkdown(md);
  assert.equal(blocks.length, 1);
  assert.equal(blocks[0].type, "list");
});

test("Phase 6 Test A: Angular with parenthetical architecture label", () => {
  const text = "إمكانية تحويل النظام إلى Angular (Architecture & Feasibility)";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["Angular (Architecture & Feasibility)"]);
});

test("Phase 6 Test B: Python label with colon", () => {
  const text = "بايثون (Python):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  assert.equal(segs.length, 3);
  assert.equal(segs[0].text, "بايثون ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Python)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ":");
  assert.equal(segs[2].isLtr, false);
});

test("Phase 6 Test C: Frontend label with colon", () => {
  const text = "الواجهة (Frontend):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  assert.equal(segs[0].text, "الواجهة ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Frontend)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ":");
  assert.equal(segs[2].isLtr, false);
});

test("Phase 6 Test D: Backend API label with colon", () => {
  const text = "الخلفية (Backend API):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  assert.equal(segs[0].text, "الخلفية ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Backend API)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ":");
  assert.equal(segs[2].isLtr, false);
});

test("Phase 6 Test E: Decoupled Architecture label with colon", () => {
  const text = "السيناريو الأمثل للتنفيذ (Decoupled Architecture):";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  assert.equal(segs[0].text, "السيناريو الأمثل للتنفيذ ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Decoupled Architecture)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ":");
  assert.equal(segs[2].isLtr, false);
});

test("Phase 6 Test F: Separation of Concerns with period", () => {
  const text = "فصل تام بين طبقة العرض وطبقة البيانات (Separation of Concerns).";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  assert.equal(segs.length, 3);
  assert.equal(segs[0].text, "فصل تام بين طبقة العرض وطبقة البيانات ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Separation of Concerns)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ".");
  assert.equal(segs[2].isLtr, false);
});

test("Phase 6 Test G: IT team with User Management & RBAC", () => {
  const text = "ما يجب طلبه من فريق الـ IT بخصوص (User Management & RBAC)";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["IT", "(User Management & RBAC)"]);
});

test("Phase 6 Test H: OAuth 2.0 / OpenID Connect (OIDC) and SAML 2.0 with Arabic question mark", () => {
  const text = "هل النظام يعتمد OAuth 2.0 / OpenID Connect (OIDC) أم SAML 2.0؟";
  assert.equal(isRtlText(text), true);
  assert.equal(getTextDirection(text), "rtl");
  const segs = splitBidiSegments(text);
  const ltr = segs.filter(s => s.isLtr).map(s => s.text);
  assert.deepEqual(ltr, ["OAuth 2.0 / OpenID Connect (OIDC)", "SAML 2.0"]);
  assert.equal(segs[segs.length - 1].text, "؟");
  assert.equal(segs[segs.length - 1].isLtr, false);
});

test("Phase 6 Test I: English-only paragraph remains LTR", () => {
  const text = "This is an English paragraph verifying standard LTR layout.";
  assert.equal(isRtlText(text), false);
  assert.equal(hasLtrText(text), true);
  assert.equal(getTextDirection(text), "ltr");
  assert.equal(getTextAlignClass(text), "text-left");
});

test("Phase 6 Test J: RTL list with technical English items", () => {
  const md = `اطلب منهم التأكد من تضمين:

- email / username / employee_id
- user_type / user_role
- position / job_title
- department / team_id`;

  assert.equal(isRtlText(md), true);
  const blocks = parseMarkdown(md);
  assert.equal(blocks.length, 2);
  assert.equal(blocks[1].type, "list");
  if (blocks[1].type === "list") {
    assert.equal(blocks[1].items.length, 4);
    assert.equal(blocks[1].items[0].text, "email / username / employee_id");
  }
});

test("Phase 6 Test K: Bold Angular (Architecture & Feasibility)", () => {
  const content = "Angular (Architecture & Feasibility)";
  assert.equal(isRtlText(content), false);
  assert.equal(hasLtrText(content), true);
  assert.equal(getTextDirection(content), "ltr");
  const segs = splitBidiSegments(content);
  assert.equal(segs.length, 1);
  assert.equal(segs[0].isLtr, true);
  assert.equal(segs[0].text, "Angular (Architecture & Feasibility)");
});

test("Phase 6 Test L: Bold بايثون (Python): with outer colon in RTL context", () => {
  const content = "بايثون (Python):";
  assert.equal(isRtlText(content), true);
  assert.equal(hasLtrText(content), true);
  assert.equal(getTextDirection(content), "rtl");
  const segs = splitBidiSegments(content);
  assert.equal(segs.length, 3);
  assert.equal(segs[0].text, "بايثون ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Python)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ":");
  assert.equal(segs[2].isLtr, false);
});

test("Phase 6 Test M: Bold السيناريو الأمثل للتنفيذ (Decoupled Architecture):", () => {
  const content = "السيناريو الأمثل للتنفيذ (Decoupled Architecture):";
  assert.equal(isRtlText(content), true);
  assert.equal(hasLtrText(content), true);
  assert.equal(getTextDirection(content), "rtl");
  const segs = splitBidiSegments(content);
  assert.equal(segs.length, 3);
  assert.equal(segs[0].text, "السيناريو الأمثل للتنفيذ ");
  assert.equal(segs[0].isLtr, false);
  assert.equal(segs[1].text, "(Decoupled Architecture)");
  assert.equal(segs[1].isLtr, true);
  assert.equal(segs[2].text, ":");
  assert.equal(segs[2].isLtr, false);
});
