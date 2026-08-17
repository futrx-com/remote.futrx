import assert from "node:assert/strict";
import test from "node:test";
import type { ProjectTemplate } from "../../models/template.ts";
import { newProjectState, type NewProjectState } from "./newProjectState.ts";

const catalog: ProjectTemplate[] = [
  {
    name: "blank",
    title: "Blank",
    description: "Nothing extra.",
    icon: "blank",
    default: true,
    provisions: false,
    prebuiltImageAvailable: false,
  },
  {
    name: "wordpress",
    title: "WordPress",
    description: "PHP + MariaDB + WP-CLI.",
    icon: "wordpress",
    defaultPorts: [8080],
    default: false,
    provisions: true,
    prebuiltImage: "futrx-remote-wordpress-base",
    prebuiltImageAvailable: false,
  },
  {
    name: "laravel",
    title: "Laravel",
    description: "PHP + Composer + MariaDB.",
    icon: "laravel",
    default: false,
    provisions: true,
    prebuiltImage: "futrx-remote-laravel-base",
    prebuiltImageAvailable: true,
  },
];

function loaded(): NewProjectState {
  const opened = newProjectState.reduce(newProjectState.createInitial(), { type: "open" });
  return newProjectState.reduce(opened, { type: "templates-loaded", templates: catalog });
}

test("starts closed on the default template", () => {
  const initial = newProjectState.createInitial();
  assert.equal(initial.open, false);
  assert.equal(initial.template, "blank");
  assert.equal(newProjectState.canSubmit(initial), false);
});

test("loading the catalog keeps the catalog's own default", () => {
  const state = loaded();
  assert.equal(state.template, "blank");
  assert.equal(state.templates.length, 3);
  assert.equal(state.templatesLoading, false);
});

test("a selection that survives a catalog reload is kept", () => {
  const selected = newProjectState.reduce(loaded(), {
    type: "select-template",
    template: "wordpress",
  });
  const reloaded = newProjectState.reduce(selected, {
    type: "templates-loaded",
    templates: catalog,
  });
  assert.equal(reloaded.template, "wordpress");
});

test("a selection the catalog no longer offers falls back to the default", () => {
  const selected = newProjectState.reduce(loaded(), {
    type: "select-template",
    template: "wordpress",
  });
  const reloaded = newProjectState.reduce(selected, {
    type: "templates-loaded",
    templates: [catalog[0]],
  });
  assert.equal(reloaded.template, "blank");
});

test("a failed catalog still allows creating a default project", () => {
  const failed = newProjectState.reduce(loaded(), {
    type: "templates-failed",
    error: "503",
  });
  assert.equal(failed.template, "blank");
  assert.equal(failed.templatesError, "503");
  assert.equal(failed.templates.length, 0);

  const named = newProjectState.reduce(failed, { type: "set-name", name: "Site" });
  assert.equal(newProjectState.canSubmit(named), true);
});

test("reopening clears the form but keeps the loaded catalog", () => {
  const dirty = newProjectState.reduce(
    newProjectState.reduce(loaded(), { type: "set-name", name: "Draft" }),
    { type: "select-template", template: "laravel" }
  );
  const failed = newProjectState.reduce(dirty, { type: "submit-failed", error: "boom" });
  const reopened = newProjectState.reduce(failed, { type: "open" });

  assert.equal(reopened.name, "");
  assert.equal(reopened.error, "");
  assert.equal(reopened.submitting, false);
  assert.equal(reopened.template, "blank");
  assert.equal(reopened.templates.length, 3);
});

test("submission requires a non-blank name and blocks double submits", () => {
  const blank = newProjectState.reduce(loaded(), { type: "set-name", name: "   " });
  assert.equal(newProjectState.canSubmit(blank), false);

  const named = newProjectState.reduce(loaded(), { type: "set-name", name: "  Site  " });
  assert.equal(newProjectState.submittedName(named), "Site");
  assert.equal(newProjectState.canSubmit(named), true);

  const submitting = newProjectState.reduce(named, { type: "submit" });
  assert.equal(newProjectState.canSubmit(submitting), false);

  const failed = newProjectState.reduce(submitting, { type: "submit-failed", error: "409" });
  assert.equal(failed.error, "409");
  assert.equal(newProjectState.canSubmit(failed), true);
});

test("the first-start notice reflects whether a prebuilt image is published", () => {
  const blank = loaded();
  assert.equal(newProjectState.firstStartNotice(blank), "");

  const slow = newProjectState.reduce(blank, { type: "select-template", template: "wordpress" });
  assert.match(newProjectState.firstStartNotice(slow), /several minutes/);

  const fast = newProjectState.reduce(blank, { type: "select-template", template: "laravel" });
  assert.match(newProjectState.firstStartNotice(fast), /starts immediately/);
});

test("closing leaves no in-flight submission behind", () => {
  const submitting = newProjectState.reduce(
    newProjectState.reduce(loaded(), { type: "set-name", name: "Site" }),
    { type: "submit" }
  );
  const closed = newProjectState.reduce(submitting, { type: "close" });
  assert.equal(closed.open, false);
  assert.equal(closed.submitting, false);
});
