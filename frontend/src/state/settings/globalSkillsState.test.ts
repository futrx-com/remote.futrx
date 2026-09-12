import assert from "node:assert/strict";
import test from "node:test";
import { globalSkillsState } from "./globalSkillsState.ts";
import type { GlobalSkill } from "../../models/globalSkill.ts";
import type { RegisteredSkill } from "../../models/skill.ts";

test("reads name and description from SKILL.md frontmatter", () => {
  const manifest = [
    "---",
    'name: "code-review-guard"',
    "description: Rigorous review checklist.",
    "---",
    "",
    "# Ignored heading",
  ].join("\n");

  assert.deepEqual(globalSkillsState.parseManifest(manifest), {
    name: "code-review-guard",
    description: "Rigorous review checklist.",
  });
});

test("falls back to the first heading when a manifest has no frontmatter", () => {
  assert.deepEqual(globalSkillsState.parseManifest("# WordPress Guard\n\nbody"), {
    name: "WordPress Guard",
    description: "",
  });
});

test("validates directory names the same way the backend does", () => {
  // Names are normalized before validation, exactly like the service, so a
  // pasted title only fails on characters no lowercase form can fix.
  assert.equal(globalSkillsState.normalizeName("  Guard  "), "guard");

  for (const valid of ["code-review-guard", "guard.v2_1", "a", "Guard"]) {
    assert.equal(globalSkillsState.isValidName(valid), true, valid);
  }
  for (const invalid of ["", "code guard", "../escape", ".hidden", "_index", "a".repeat(65)]) {
    assert.equal(globalSkillsState.isValidName(invalid), false, invalid);
  }
});

test("suggests a directory name from a human title", () => {
  assert.equal(globalSkillsState.suggestName("Code Review Guard"), "code-review-guard");
  assert.equal(globalSkillsState.suggestName("  WordPress / Woo Guard!  "), "wordpress-woo-guard");
});

test("keeps SKILL.md authoritative when assembling the payload", () => {
  const files = globalSkillsState.buildFiles({
    name: "guard",
    manifest: "manifest body",
    extraFiles: { "SKILL.md": "sneaky", "refs/a.md": "ref" },
    alwaysOn: false,
  });

  assert.deepEqual(files, { "SKILL.md": "manifest body", "refs/a.md": "ref" });
});

test("rejects drafts the server would reject", () => {
  const draft = {
    name: "guard",
    manifest: "---\nname: guard\n---\n",
    extraFiles: {},
    alwaysOn: false,
  };

  assert.equal(globalSkillsState.validateDraft(draft), null);
  assert.match(
    globalSkillsState.validateDraft({ ...draft, name: "Bad Name" }) ?? "",
    /lowercase/
  );
  assert.match(globalSkillsState.validateDraft({ ...draft, manifest: "  " }) ?? "", /empty/);
  assert.match(
    globalSkillsState.validateDraft({ ...draft, extraFiles: { "../escape.md": "x" } }) ?? "",
    /Invalid file path/
  );
});

test("round-trips a stored skill through the editor draft", () => {
  const skill: GlobalSkill = {
    name: "guard",
    alwaysOn: true,
    files: { "SKILL.md": "manifest", "refs/a.md": "ref" },
  };
  const draft = globalSkillsState.draftFromSkill(skill);

  assert.deepEqual(draft, {
    name: "guard",
    manifest: "manifest",
    extraFiles: { "refs/a.md": "ref" },
    alwaysOn: true,
  });
  assert.deepEqual(globalSkillsState.buildFiles(draft), skill.files);
});

test("keeps the library list sorted through create, edit, and delete", () => {
  let library: GlobalSkill[] = [];
  library = globalSkillsState.upsert(library, { name: "wordpress-guard", alwaysOn: false });
  library = globalSkillsState.upsert(library, { name: "code-review-guard", alwaysOn: false });
  assert.deepEqual(library.map((skill) => skill.name), [
    "code-review-guard",
    "wordpress-guard",
  ]);

  library = globalSkillsState.upsert(library, { name: "code-review-guard", alwaysOn: true });
  assert.equal(library.length, 2);
  assert.equal(library[0].alwaysOn, true);

  library = globalSkillsState.remove(library, "code-review-guard");
  assert.deepEqual(library.map((skill) => skill.name), ["wordpress-guard"]);
});

test("shadowed global skills cannot be selected", () => {
  const shadowed: RegisteredSkill = {
    name: "Guard",
    command: "guard",
    provider: "claude",
    source: "global",
    scope: "global",
    shadowed: true,
  };
  const visible: RegisteredSkill = { ...shadowed, shadowed: false };

  assert.equal(globalSkillsState.isSelectable(shadowed), false);
  assert.equal(globalSkillsState.isSelectable(visible), true);
});

test("badges global skills by scope, shadowing, and always-on", () => {
  const base: RegisteredSkill = {
    name: "Guard",
    command: "guard",
    provider: "codex",
    source: "global",
    scope: "global",
  };

  assert.equal(globalSkillsState.badgeFor(base), "global");
  assert.equal(globalSkillsState.badgeFor({ ...base, alwaysOn: true }), "global · always on");
  assert.equal(globalSkillsState.badgeFor({ ...base, alwaysOn: true, shadowed: true }), "shadowed");
  assert.equal(
    globalSkillsState.badgeFor({ name: "Local", command: "local", provider: "codex", source: "project" }),
    "project"
  );
});
