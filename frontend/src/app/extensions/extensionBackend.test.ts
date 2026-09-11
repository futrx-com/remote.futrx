import assert from "node:assert/strict";
import { test } from "node:test";
import { createBackendApi } from "./extensionBackend.ts";
import type { AppBackendInstance, AppImage } from "../../models/application.ts";

const image = {
  id: "demo",
  name: "Demo",
  type: "backend",
  scopes: ["global", "project"],
  port: { internal: 0, defaultExternal: 0 },
  backend: {},
} as AppImage;

const globalInstance: AppBackendInstance = {
  instanceId: "global-1",
  scope: "global",
};
const projectInstance: AppBackendInstance = {
  instanceId: "project-1",
  scope: "project",
  projectId: "alpha",
};

test("a global install is addressed through the global route", () => {
  const backend = createBackendApi(image, [globalInstance]);
  assert.equal(backend.available, true);
  assert.equal(
    backend.url("health"),
    "/api/applications/global-1/backend/health",
  );
});

test("a project install is addressed through its project route", () => {
  const backend = createBackendApi(image, [projectInstance]);
  assert.equal(
    backend.url("health", { projectId: "alpha" }),
    "/api/projects/alpha/applications/project-1/backend/health",
  );
});

// A project id is a preference, not a filter: an extension passing
// context.projectId wants this project's backend or the server-wide one, which
// is how the same extension is scoped on screen.
test("a project id prefers that project's install and falls back to the global one", () => {
  const backend = createBackendApi(image, [globalInstance, projectInstance]);
  assert.equal(
    backend.url("health", { projectId: "alpha" }),
    "/api/projects/alpha/applications/project-1/backend/health",
  );
  assert.equal(
    backend.url("health", { projectId: "beta" }),
    "/api/applications/global-1/backend/health",
  );
  assert.equal(
    backend.url("health"),
    "/api/applications/global-1/backend/health",
  );
});

test("with only a project install, a call from elsewhere still reaches it", () => {
  const backend = createBackendApi(image, [projectInstance]);
  assert.equal(
    backend.url("health"),
    "/api/projects/alpha/applications/project-1/backend/health",
  );
});

test("an explicit instance id wins, and an unknown one is refused", () => {
  const backend = createBackendApi(image, [globalInstance, projectInstance]);
  assert.equal(
    backend.url("health", { instanceId: "project-1", projectId: "beta" }),
    "/api/projects/alpha/applications/project-1/backend/health",
  );
  assert.throws(
    () => backend.url("health", { instanceId: "missing" }),
    /no running backend with id missing/,
  );
});

// An image whose plugin is not running is the case an extension should degrade
// around, so `available` says so rather than every call throwing later.
test("an image with no running install reports itself unavailable", () => {
  const backend = createBackendApi(image, []);
  assert.equal(backend.available, false);
  assert.deepEqual(backend.instances, []);
  assert.throws(() => backend.url("health"), /no running backend/);
});

test("an image that ships no plugin is unavailable even when installed", () => {
  const uiOnly = { ...image, type: "ui", backend: undefined } as AppImage;
  assert.equal(createBackendApi(uiOnly, [globalInstance]).available, false);
});

// Route separators have to survive encoding, or a plugin sees an escaped path
// it never declared.
test("route separators survive encoding but segments are escaped", () => {
  const backend = createBackendApi(image, [globalInstance]);
  assert.equal(
    backend.url("kv/greeting"),
    "/api/applications/global-1/backend/kv/greeting",
  );
  assert.equal(
    backend.url("kv/a b"),
    "/api/applications/global-1/backend/kv/a%20b",
  );
  assert.equal(backend.url(""), "/api/applications/global-1/backend");
  assert.equal(backend.url("/health"), "/api/applications/global-1/backend/health");
});
