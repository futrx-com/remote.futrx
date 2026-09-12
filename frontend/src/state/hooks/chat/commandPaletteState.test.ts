import assert from "node:assert/strict";
import test from "node:test";
import type { RegisteredSkill } from "../../../models/skill";
import { commandPaletteState } from "./commandPaletteState.ts";

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

test("query returns null for empty or non-leading slash text", () => {
  assert.equal(commandPaletteState.query(""), null);
  assert.equal(commandPaletteState.query("hello"), null);
  assert.equal(commandPaletteState.query("say /help please"), null);
});

test("query captures the term right after a leading slash", () => {
  assert.equal(commandPaletteState.query("/"), "");
  assert.equal(commandPaletteState.query("/build"), "build");
  assert.equal(commandPaletteState.query("/analyze now"), "analyze now");
});

test("filter returns the full list for an empty query", () => {
  assert.equal(commandPaletteState.filter(skills, ""), skills);
  assert.equal(commandPaletteState.filter(skills, null), skills);
});

test("filter prefers command-prefix matches", () => {
  const result = commandPaletteState.filter(skills, "b");
  assert.equal(result.length, 1);
  assert.equal(result[0].name, "Build steps");
});

test("filter falls back to a broad description search", () => {
  const result = commandPaletteState.filter(skills, "restructure");
  assert.equal(result.length, 1);
  assert.equal(result[0].name, "Refactor");
});

test("filter is case-insensitive", () => {
  const result = commandPaletteState.filter(skills, "BUILD");
  assert.equal(result.length, 1);
  assert.equal(result[0].name, "Build steps");
});

test("key actions preserve palette handling and ignore empty navigation", () => {
  assert.equal(commandPaletteState.actionForKey("Escape", 0), "dismiss");
  assert.equal(commandPaletteState.actionForKey("ArrowDown", 0), "ignore");
  assert.equal(commandPaletteState.actionForKey("ArrowUp", 0), "ignore");
  assert.equal(commandPaletteState.actionForKey("Enter", 0), "ignore");
  assert.equal(commandPaletteState.actionForKey("Tab", 0), "ignore");
  assert.equal(commandPaletteState.actionForKey("a", 3), "ignore");
  assert.equal(commandPaletteState.actionForKey("ArrowDown", 3), "next");
  assert.equal(commandPaletteState.actionForKey("ArrowUp", 3), "previous");
  assert.equal(commandPaletteState.actionForKey("Enter", 3), "choose");
  assert.equal(commandPaletteState.actionForKey("Tab", 3), "choose");
});

test("highlight movement wraps and selection clamps to the last item", () => {
  assert.equal(commandPaletteState.moveHighlight(0, 1, 3), 1);
  assert.equal(commandPaletteState.moveHighlight(2, 1, 3), 0);
  assert.equal(commandPaletteState.moveHighlight(0, -1, 3), 2);
  assert.equal(commandPaletteState.moveHighlight(2, 1, 0), 0);
  assert.equal(commandPaletteState.selectedItem(skills, 9), skills[2]);
  assert.equal(commandPaletteState.selectedItem([], 0), undefined);
});
