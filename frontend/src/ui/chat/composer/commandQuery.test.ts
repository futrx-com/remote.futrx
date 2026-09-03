import assert from "node:assert/strict";
import test from "node:test";
import type { RegisteredSkill } from "../../../models/skill";
import { commandQuery, filterCommands } from "./commandQuery.ts";

const skills: RegisteredSkill[] = [
  {
    name: "Analyze code",
    command: "/analyze",
    description: "Deep-dive a codebase",
    provider: "codex",
  },
  {
    name: "Build steps",
    command: "/build",
    description: "Run the build and report failures",
    provider: "codex",
  },
  {
    name: "Refactor",
    command: "/refactor",
    description: "Restructure code",
    provider: "anthropic",
  },
];

test("commandQuery returns null for empty or non-leading slash text", () => {
  assert.equal(commandQuery(""), null);
  assert.equal(commandQuery("hello"), null);
  assert.equal(commandQuery("say /help please"), null);
});

test("commandQuery captures the term right after a leading slash", () => {
  assert.equal(commandQuery("/"), "");
  assert.equal(commandQuery("/build"), "build");
  assert.equal(commandQuery("/analyze now"), "analyze now");
});

test("filterCommands returns the full list for an empty query", () => {
  assert.equal(filterCommands(skills, ""), skills);
  assert.equal(filterCommands(skills, null), skills);
});

test("filterCommands prefers command-prefix matches", () => {
  const result = filterCommands(skills, "b");
  assert.equal(result.length, 1);
  assert.equal(result[0].name, "Build steps");
});

test("filterCommands falls back to a broad description search", () => {
  const result = filterCommands(skills, "restructure");
  assert.equal(result.length, 1);
  assert.equal(result[0].name, "Refactor");
});

test("filterCommands is case-insensitive", () => {
  const result = filterCommands(skills, "BUILD");
  assert.equal(result.length, 1);
  assert.equal(result[0].name, "Build steps");
});
