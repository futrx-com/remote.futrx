import { strict as assert } from "node:assert";
import { describe, it } from "node:test";

import type { AppImage, AppInstance, AppKind } from "../../models/application.ts";
import {
  hasContainer,
  hasPortBinding,
  instanceSummary,
  uninstallConsequence,
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

  it("falls back to the service presentation while the catalog is loading", () => {
    assert.equal(hasContainer(undefined), true);
    assert.equal(hasPortBinding(undefined), true);
  });

  it("summarises a tool by where it runs, not by a port it does not bind", () => {
    assert.match(instanceSummary(image("tool"), true), /Workspace tool/);
    assert.match(instanceSummary(image("tool"), false), /Start it/);
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

});
