import { strict as assert } from "node:assert";
import { describe, it } from "node:test";

import type {
  AppApplication,
  AppInstance,
} from "../../models/application.ts";
import {
  hasContainer,
  hasPortBinding,
  instanceSummary,
  uninstallConsequence,
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

  it("summarises a tool by where it runs, not by a UI it does not have", () => {
    assert.match(instanceSummary(application({ container: true }), true), /Infrastructure/);
    assert.match(instanceSummary(application({ container: true }), false), /Start it/);
    assert.match(instanceSummary(application({ backend: true }), true), /Go plugin/);
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
