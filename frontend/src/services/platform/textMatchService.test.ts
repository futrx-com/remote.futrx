import assert from "node:assert/strict";
import test from "node:test";
import { textFoldService } from "./textFoldService.ts";
import { textMatchService } from "./textMatchService.ts";

test("tokenizing splits separators and camelCase", () => {
  assert.deepEqual(textMatchService.tokenize("workspaceSidebarState.ts"), [
    "workspace",
    "sidebar",
    "state",
    "ts",
  ]);
  assert.deepEqual(textMatchService.tokenize("remote.futrx"), ["remote", "futrx"]);
});

test("tokenizing keeps words in every script", () => {
  assert.deepEqual(textMatchService.tokenize("مرحبا بكم"), ["مرحبا", "بكم"]);
  assert.deepEqual(textMatchService.tokenize("Привет мир"), ["привет", "мир"]);
  assert.deepEqual(textMatchService.tokenize("工作区"), ["工作区"]);
  assert.deepEqual(textMatchService.tokenize("שלום"), ["שלום"]);
});

test("a chunk that is only symbols is searched for literally", () => {
  // Nothing else in it could match, so it is the query rather than a separator.
  assert.deepEqual(textMatchService.tokenize("→"), ["→"]);
  assert.deepEqual(textMatchService.tokenize("?!"), ["?!"]);
  assert.deepEqual(textMatchService.tokenize("caddy →"), ["caddy", "→"]);
});

test("non-latin queries match, and extra words still narrow", () => {
  const folded = textFoldService.fold("مراجعة الكود العربي");
  assert.ok(textMatchService.matchField(folded, textMatchService.tokenize("الكود")));
  assert.ok(textMatchService.matchField(folded, textMatchService.tokenize("مراجعة الكود")));
  assert.equal(textMatchService.matchField(folded, textMatchService.tokenize("قاعدة")), null);
});

test("a symbol query matches only fields carrying that symbol", () => {
  const arrow = textFoldService.fold("Deploy → QA");
  assert.ok(textMatchService.matchField(arrow, textMatchService.tokenize("→")));
  assert.equal(
    textMatchService.matchField(textFoldService.fold("Deploy to QA"), textMatchService.tokenize("→")),
    null
  );
});

test("all query tokens must match, so extra words narrow the results", () => {
  const folded = textFoldService.fold("Caddy TLS on-demand ask");
  assert.ok(textMatchService.matchField(folded, ["caddy", "tls"]));
  assert.equal(textMatchService.matchField(folded, ["caddy", "postgres"]), null);
});

// The edit-distance bound is an internal of `matchField`, so it is pinned
// through the contract it serves: a near miss matches, a different word does
// not, and the typo scores below the direct hit it stands in for.
test("bounded typo tolerance accepts near misses and rejects far ones", () => {
  const folded = textFoldService.fold("Sidebar search rewrite");
  const exact = textMatchService.matchField(folded, ["sidebar"]);
  const typo = textMatchService.matchField(folded, ["sidbar"]);
  assert.ok(exact);
  assert.ok(typo);
  assert.ok(typo.score < exact.score);
  assert.equal(textMatchService.matchField(folded, ["toolbar"]), null);
});
