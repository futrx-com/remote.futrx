import assert from "node:assert/strict";
import test from "node:test";
import type { RegisteredSkill } from "../../models/skill.ts";
import { skillSearchService } from "./skillSearchService.ts";

const skills: RegisteredSkill[] = [
  {
    name: "ui-ux-pro-max",
    command: "/ui-ux-pro-max",
    description: "Design polished interfaces in small steps",
    provider: "claude",
    source: "user",
  },
  {
    name: "Build steps",
    command: "/build",
    description: "Run the build and report failures",
    provider: "codex",
    source: "system",
  },
  {
    name: "Refactor",
    command: "/refactor",
    description: "Restructure code and interface boundaries",
    provider: "claude",
    source: "plugin",
  },
];

function names(result: RegisteredSkill[]): string[] {
  return result.map((skill) => skill.name);
}

test("an empty query returns the list untouched", () => {
  assert.equal(skillSearchService.filter(skills, ""), skills);
  assert.equal(skillSearchService.filter(skills, "   "), skills);
  assert.equal(skillSearchService.filter(skills, null), skills);
});

test("space-separated words find a hyphenated command", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "ui ux")), ["ui-ux-pro-max"]);
  assert.deepEqual(names(skillSearchService.filter(skills, "UI UX PRO")), ["ui-ux-pro-max"]);
});

test("a partial last word still reads as the start of the command", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "ui-u")), ["ui-ux-pro-max"]);
  assert.deepEqual(names(skillSearchService.filter(skills, "ui u")), ["ui-ux-pro-max"]);
});

test("command-prefix hits win outright over looser matches", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "b")), ["Build steps"]);
});

test("words from the middle of a command still match it", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "pro max")), ["ui-ux-pro-max"]);
});

test("falls back to the description when no command starts with the query", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "restructure")), ["Refactor"]);
});

test("a command or name hit ranks above a description-only hit", () => {
  // "steps" is in ui-ux-pro-max's description but in Build's name, so Build
  // moves ahead of the skill listed before it.
  assert.deepEqual(names(skillSearchService.filter(skills, "steps")), ["Build steps", "ui-ux-pro-max"]);
});

test("field weight scales match quality rather than overriding it", () => {
  // "deploy" is one typo away from the /deply command but spells the other
  // skill's name exactly; the exact name hit ranks first.
  const typo: RegisteredSkill = { name: "deply", command: "/deply", provider: "claude" };
  const exact: RegisteredSkill = { name: "deploy", command: "/ship", provider: "claude" };
  assert.deepEqual(names(skillSearchService.filter([typo, exact], "deploy")), ["deploy", "deply"]);
});

test("tolerates a small typo in a longer word", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "refactr")), ["Refactor"]);
});

test("matches the skill source, as the skill picker always has", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "plugin")), ["Refactor"]);
});

test("every query word must match somewhere in one field", () => {
  assert.deepEqual(names(skillSearchService.filter(skills, "build zzz")), []);
});
