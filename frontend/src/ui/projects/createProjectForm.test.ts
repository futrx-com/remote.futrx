import assert from "node:assert/strict";
import test from "node:test";
import { PROJECT_MAX_SLUG_LEN } from "../../config/project.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { createProjectForm } from "./createProjectForm.ts";

function project(overrides: Partial<ProjectMeta>): ProjectMeta {
  return {
    id: "id",
    name: "Name",
    slug: "name",
    cwd: "/var/lib/remote/projects/name/workspace",
    containerName: "name",
    status: "running",
    createdAt: 1,
    updatedAt: 1,
    ...overrides,
  };
}

test("slugify mirrors the backend rules", () => {
  assert.equal(createProjectForm.slugify("My Project"), "my-project");
  assert.equal(createProjectForm.slugify("  FutrX Web  "), "futrx-web");
  assert.equal(createProjectForm.slugify("a_b.c/d"), "a-b-c-d");
  assert.equal(createProjectForm.slugify("123 go"), "p-123-go");
  assert.equal(createProjectForm.slugify("!!!"), "");
  assert.equal(createProjectForm.slugify("trailing---"), "trailing");
  const long = createProjectForm.slugify("x".repeat(50));
  assert.equal(long.length, PROJECT_MAX_SLUG_LEN);
});

test("validate rejects empty and short names", () => {
  const projects = [project({ name: "futrx-web", slug: "futrx-web" })];
  assert.equal(createProjectForm.validate("", projects).ok, false);
  assert.equal(createProjectForm.validate("", projects).message, "");
  assert.equal(
    createProjectForm.validate("!", projects).message,
    "Use at least 2 letters or numbers."
  );
  const ok = createProjectForm.validate("Ops Runbook", projects);
  assert.deepEqual(ok, { ok: true, slug: "ops-runbook", message: "Saved as ops-runbook" });
  assert.equal(createProjectForm.validate("plain", projects).message, "");
});

test("validate rejects duplicate display names after trimming and case folding", () => {
  const projects = [
    project({ id: "1", name: "  FutrX Web  ", slug: "legacy-project-slug" }),
  ];

  assert.deepEqual(createProjectForm.validate("fUtRx WeB", projects), {
    ok: false,
    slug: "futrx-web",
    message: "A project named futrx-web already exists.",
  });
});

test("validate suffixes a slug collision when display names differ", () => {
  const projects = [
    project({ id: "1", name: "FutrX-Web", slug: "futrx-web" }),
    project({ id: "2", name: "Something Else", slug: "futrx-web-2" }),
  ];

  assert.deepEqual(createProjectForm.validate("FutrX Web", projects), {
    ok: true,
    slug: "futrx-web-3",
    message: "Saved as futrx-web-3",
  });
});

test("validate truncates a taken maximum-length slug before adding its suffix", () => {
  const base = "x".repeat(PROJECT_MAX_SLUG_LEN);
  const projects = [
    project({ id: "1", name: "Existing one", slug: base }),
    project({ id: "2", name: "Existing two", slug: `${"x".repeat(30)}-2` }),
  ];

  assert.deepEqual(createProjectForm.validate(base, projects), {
    ok: true,
    slug: `${"x".repeat(30)}-3`,
    message: `Saved as ${"x".repeat(30)}-3`,
  });
});

test("pathPreview derives the workspace root from an existing project", () => {
  const projects = [
    project({ slug: "futrx-web", cwd: "/var/lib/remote/projects/futrx-web/workspace" }),
  ];
  assert.equal(
    createProjectForm.pathPreview(projects, "ops"),
    "/var/lib/remote/projects/ops/workspace"
  );
  assert.equal(
    createProjectForm.pathPreview(projects, ""),
    "/var/lib/remote/projects/…/workspace"
  );
  assert.equal(createProjectForm.pathPreview([], "ops"), "~/projects/ops");
  assert.equal(createProjectForm.pathPreview([], ""), "~/projects/…");
});
