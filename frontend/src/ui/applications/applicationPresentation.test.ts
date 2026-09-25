import { strict as assert } from "node:assert";
import { describe, it } from "node:test";

import type {
  AppApplication,
  AppInstance,
  AppPackage,
} from "../../models/application.ts";
import {
  describeInstalls,
  describeOutcome,
  hasAssignedConnection,
  hasContainer,
  hasPortBinding,
  instanceLifecycleAction,
  instanceSummary,
  packageCountLabel,
  packageScopes,
  packageSummary,
  uninstallConsequence,
  whereToInstall,
} from "./applicationPresentation.ts";

function application(capabilities: {
  container?: boolean;
  port?: boolean;
  backend?: boolean;
} = {}): AppApplication {
  return {
    id: "x",
    name: "X",
    scopes: ["project"],
    needsContainer: capabilities.container ?? false,
    needsPort: capabilities.port ?? false,
    backend: capabilities.backend ? {} : undefined,
  } as AppApplication;
}

function instance(overrides: Partial<AppInstance> = {}): AppInstance {
  return {
    id: "i1",
    applicationId: "x",
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
    assert.equal(hasContainer(application({ container: true })), true);
    assert.equal(hasPortBinding(application({ container: true })), false);
  });

  it("keeps service applications on the port presentation", () => {
    assert.equal(hasContainer(application({ container: true, port: true })), true);
    assert.equal(hasPortBinding(application({ container: true, port: true })), true);
  });

  it("keeps extension applications off both", () => {
    assert.equal(hasContainer(application()), false);
    assert.equal(hasPortBinding(application({ backend: true })), false);
  });

  it("falls back to the service presentation while the catalog is loading", () => {
    assert.equal(hasContainer(undefined), true);
    assert.equal(hasPortBinding(undefined), true);
  });

  it("only exposes lifecycle actions for completed installs", () => {
    assert.equal(instanceLifecycleAction("running"), "stop");
    assert.equal(instanceLifecycleAction("stopped"), "start");
    assert.equal(instanceLifecycleAction("installing"), null);
    assert.equal(instanceLifecycleAction("error"), null);
  });

  it("does not invent connection details for an early failed attempt", () => {
    const service = application({ container: true, port: true });
    assert.equal(hasAssignedConnection(instance({ containerName: "project-1" }), service), true);
    assert.equal(hasAssignedConnection(instance({ containerName: "" }), service), false);
    assert.equal(hasAssignedConnection(instance({ externalPort: 0 }), service), false);
    assert.equal(hasAssignedConnection(instance(), application({ container: true })), false);
  });

  it("summarises a tool by where it runs, not by a UI it does not have", () => {
    assert.match(instanceSummary(application({ container: true }), true), /Infrastructure/);
    assert.match(instanceSummary(application({ container: true }), false), /Start it/);
    assert.match(instanceSummary(application({ backend: true }), true), /Go backend/);
    assert.match(instanceSummary(application(), true), /Interface extension/);
  });

  it("never promises to release a host port a tool never held", () => {
    const message = uninstallConsequence(instance(), application({ container: true }));
    assert.doesNotMatch(message, /port/i);
    assert.match(message, /project container/);
  });

  it("still reports the released port for a service", () => {
    const message = uninstallConsequence(instance(), application({ container: true, port: true }));
    assert.match(message, /127\.0\.0\.1:5433/);
  });

  it("says nothing is removed from a container for an extension", () => {
    const message = uninstallConsequence(instance(), application());
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

  // The scope set nearly every real package declares. Each page gets its own
  // sentence: "here, and in every other project" is true standing in a
  // project and false standing in Settings, which is not a project at all.
  it("says a dual-scope app offers both, in the words of the page asking", () => {
    assert.equal(
      packageScopes(pkg(["global", "project"]), "project"),
      "Installs here, and in every other project — it offers both scopes.",
    );
    assert.equal(
      packageScopes(pkg(["global", "project"]), "global"),
      "Installs globally, or inside a project.",
    );
  });

  it("never tells a page outside a project that an app installs in this one", () => {
    // The symptom the tautology produced: Settings → Applications claiming a
    // dual-scope package installs "here, and in every other project" on a
    // page that is not a project.
    assert.doesNotMatch(packageScopes(pkg(["global", "project"]), "global"), /here/);
    assert.doesNotMatch(packageScopes(pkg(["global"]), "global"), /every other project/);
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

  it("counts what is listed, and refuses to count a listing that failed", () => {
    // "none yet" and "the request failed" are different answers, and only one
    // of them invites the operator to upload a package they already uploaded.
    assert.equal(packageCountLabel(0, false), "none yet");
    assert.equal(packageCountLabel(1, false), "1 package");
    assert.equal(packageCountLabel(4, false), "4 packages");
    assert.equal(packageCountLabel(0, true), "could not be listed");
    assert.equal(packageCountLabel(3, true), "could not be listed");
  });

  it("omits provenance it does not have rather than printing empty parts", () => {
    assert.equal(packageSummary(stored()), "");
    assert.equal(packageSummary(stored({ uploadedBy: "me@example.com" })), "by me@example.com");
  });

  it("leads with the upload time and joins every part it has", () => {
    // Asserted by shape, not by locale text: toLocaleString renders differently
    // per machine, and what this line owns is the order and the separator.
    assert.match(
      packageSummary(stored({ uploadedAt: 1700000000, uploadedBy: "me@example.com", size: 512 })),
      /^uploaded .+ · by me@example\.com · 512 B$/,
    );
  });

  it("scales the archive size by unit, and keeps a zero size out of the summary", () => {
    assert.equal(packageSummary(stored({ size: 512 })), "512 B");
    assert.equal(packageSummary(stored({ size: 2048 })), "2 KB");
    assert.equal(packageSummary(stored({ size: 3 * 1024 * 1024 })), "3.0 MB");
    assert.equal(packageSummary(stored({ size: 0 })), "");
  });
});
