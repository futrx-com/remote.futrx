import { strict as assert } from "node:assert";
import { describe, it } from "node:test";

import type {
  AppImage,
  AppInstance,
  AppKind,
  AppPackage,
} from "../../models/application.ts";
import {
  describeInstalls,
  describeOutcome,
  hasContainer,
  hasPortBinding,
  instanceSummary,
  packageScopes,
  packageSummary,
  uninstallConsequence,
  whereToInstall,
} from "./applicationPresentation.ts";

// The server derives needsContainer/needsPort from the kind and ships them with
// every catalog entry, so a fixture image is one of those payloads rather than
// a kind the SPA re-interprets.
const KIND_FLAGS: Record<
  AppKind,
  { needsContainer: boolean; needsPort: boolean }
> = {
  service: { needsContainer: true, needsPort: true },
  tool: { needsContainer: true, needsPort: false },
  ui: { needsContainer: false, needsPort: false },
  backend: { needsContainer: false, needsPort: false },
};

function image(type: AppKind): AppImage {
  return {
    id: "x",
    name: "X",
    type,
    scopes: ["project"],
    ...KIND_FLAGS[type],
  } as AppImage;
}

function instance(overrides: Partial<AppInstance> = {}): AppInstance {
  return {
    id: "i1",
    imageId: "x",
    name: "X",
    scope: "project",
    status: "running",
    bindAddress: "127.0.0.1",
    externalPort: 5433,
    internalPort: 5432,
    ...overrides,
  } as AppInstance;
}

describe("application presentation", () => {
  it("treats a tool as living in a container but not binding a port", () => {
    // The two questions are different for exactly one kind, which is the whole
    // reason the second predicate exists.
    assert.equal(hasContainer(image("tool")), true);
    assert.equal(hasPortBinding(image("tool")), false);
  });

  it("keeps service images on the port presentation", () => {
    assert.equal(hasContainer(image("service")), true);
    assert.equal(hasPortBinding(image("service")), true);
  });

  it("keeps extension images off both", () => {
    for (const kind of ["ui", "backend"] as const) {
      assert.equal(hasContainer(image(kind)), false);
      assert.equal(hasPortBinding(image(kind)), false);
    }
  });

  it("falls back to the service presentation while the catalog is loading", () => {
    assert.equal(hasContainer(undefined), true);
    assert.equal(hasPortBinding(undefined), true);
  });

  it("summarises a tool by where it runs, not by a UI it does not have", () => {
    assert.match(instanceSummary(image("tool"), true), /Workspace tool/);
    assert.match(instanceSummary(image("tool"), false), /Start it/);
    assert.match(instanceSummary(image("backend"), true), /Go plugin/);
    assert.match(instanceSummary(image("ui"), true), /Interface extension/);
  });

  it("never promises to release a host port a tool never held", () => {
    const message = uninstallConsequence(instance(), image("tool"));
    assert.doesNotMatch(message, /port/i);
    assert.match(message, /project container/);
  });

  it("still reports the released port for a service", () => {
    const message = uninstallConsequence(instance(), image("service"));
    assert.match(message, /127\.0\.0\.1:5433/);
  });

  it("says nothing is removed from a container for an extension", () => {
    const message = uninstallConsequence(instance(), image("ui"));
    assert.match(message, /Nothing is removed from any container/);
  });
});

// The uploader is shown wherever an admin manages applications — Settings and
// every project — but one catalog serves both, and a package installs only
// where it says it does. These two lines are what stop the page a package is
// listed on from implying an answer the app never gave.
describe("uploaded package scope", () => {
  function pkg(scopes: AppPackage["scopes"]): AppPackage {
    return { id: "p", name: "P", scopes, size: 0, sha256: "", uploadedAt: 0 };
  }

  it("tells a project reader that a global-only app cannot be installed there", () => {
    const line = packageScopes(pkg(["global"]), "project");
    assert.match(line, /globally only/);
    assert.match(line, /not in a project/);
    assert.match(whereToInstall(pkg(["global"]), "project"), /Settings/);
  });

  it("tells a global reader that a project-only app cannot be installed there", () => {
    const line = packageScopes(pkg(["project"]), "global");
    assert.match(line, /inside a project only/);
    assert.match(whereToInstall(pkg(["project"]), "global"), /that project's/);
  });

  it("points at the list below when the app installs where you are looking", () => {
    for (const viewing of ["global", "project"] as const) {
      assert.equal(whereToInstall(pkg([viewing]), viewing), "Install it below.");
      assert.equal(whereToInstall(pkg(["global", "project"]), viewing), "Install it below.");
    }
    assert.match(packageScopes(pkg(["project"]), "project"), /including this one/);
    assert.match(packageScopes(pkg(["global"]), "global"), /from this page/);
  });

  it("says an app declaring no scope cannot be installed at all", () => {
    for (const viewing of ["global", "project"] as const) {
      assert.match(packageScopes(pkg([]), viewing), /cannot be installed/);
      assert.match(whereToInstall(pkg(undefined), viewing), /no scope/);
    }
  });
});

describe("uploaded package provenance", () => {
  function stored(overrides: Partial<AppPackage> = {}): AppPackage {
    return { id: "p", name: "P", size: 0, sha256: "", uploadedAt: 0, ...overrides };
  }

  it("names every place a package is installed, project copies by project", () => {
    const line = describeInstalls(
      stored({
        installs: [
          { instanceId: "a", name: "A", scope: "project", projectId: "proj-1", status: "running" },
          { instanceId: "b", name: "B", scope: "global", status: "running" },
        ],
      }),
    );
    assert.equal(line, "Installed project proj-1, globally.");
  });

  it("describes an upgraded copy by its project, and a global one by name alone", () => {
    assert.equal(
      describeOutcome({ instanceId: "a", name: "A", scope: "project", projectId: "p1", toVersion: "2" }),
      "A in project p1",
    );
    assert.equal(
      describeOutcome({ instanceId: "b", name: "B", scope: "global", toVersion: "2" }),
      "B",
    );
  });

  it("omits provenance it does not have rather than printing empty parts", () => {
    assert.equal(packageSummary(stored()), "");
    assert.equal(packageSummary(stored({ uploadedBy: "me@example.com" })), "by me@example.com");
  });

  it("scales the archive size by unit, and keeps a zero size out of the summary", () => {
    assert.equal(packageSummary(stored({ size: 512 })), "512 B");
    assert.equal(packageSummary(stored({ size: 2048 })), "2 KB");
    assert.equal(packageSummary(stored({ size: 3 * 1024 * 1024 })), "3.0 MB");
    assert.equal(packageSummary(stored({ size: 0 })), "");
  });
});
