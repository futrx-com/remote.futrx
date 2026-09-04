import assert from "node:assert/strict";
import test from "node:test";

import { EXTENSION_SLOTS } from "../../../config/extensions.ts";
import type {
  ExtensionSlotContext,
  ExtensionSlotName,
} from "../../../models/extension.ts";
import { visibleExtensionContributions } from "../../hooks/extensions/extensionContributionState.ts";
import { createExtensionStore } from "./extensionStore.ts";

const noop = () => {};

function cardContext(imageId: string): ExtensionSlotContext {
  return {
    slot: EXTENSION_SLOTS.applicationCardActions,
    scope: "project",
    instance: {
      id: `${imageId}-instance`,
      imageId,
      name: imageId,
      scope: "project",
      projectId: "p1",
      containerName: "p1",
      deviceName: `app-${imageId}`,
      internalPort: 1,
      externalPort: 1,
      bindAddress: "127.0.0.1",
      status: "running",
      createdAt: 0,
      updatedAt: 0,
    },
  };
}

function createRegistry() {
  const store = createExtensionStore();
  return {
    register: store.getState().register,
    setVisibility: store.getState().setVisibility,
    setActiveProject: store.getState().setActiveProject,
    removeImage: store.getState().removeImage,
    subscribe: store.subscribe,
    contributions(
      slot: ExtensionSlotName,
      context?: ExtensionSlotContext,
    ) {
      const state = store.getState();
      const contributions = state.bySlot.get(slot) ?? [];
      return context
        ? visibleExtensionContributions(
            contributions,
            context,
            state.activeProjectId,
          )
        : contributions;
    },
  };
}

test("contributions render in order, ties keeping registration order", () => {
  const registry = createRegistry();
  registry.register("late", EXTENSION_SLOTS.sidebarHeaderActions, noop, { order: 10 });
  registry.register("early", EXTENSION_SLOTS.sidebarHeaderActions, noop, { order: -5 });
  registry.register("first-zero", EXTENSION_SLOTS.sidebarHeaderActions, noop);
  registry.register("second-zero", EXTENSION_SLOTS.sidebarHeaderActions, noop);

  assert.deepEqual(
    registry
      .contributions(EXTENSION_SLOTS.sidebarHeaderActions)
      .map((c) => c.imageId),
    ["early", "first-zero", "second-zero", "late"]
  );
});

test("an unknown slot is dropped instead of throwing", () => {
  const registry = createRegistry();
  const dispose = registry.register("mysql", "sidebar.nowhere", noop);

  assert.equal(typeof dispose, "function");
  assert.equal(
    registry.contributions(EXTENSION_SLOTS.sidebarHeaderActions).length,
    0,
    "a dropped contribution is not a change",
  );
});

test("a `when` predicate scopes a contribution to its own image's cards", () => {
  const registry = createRegistry();
  registry.register("mysql", EXTENSION_SLOTS.applicationCardActions, noop, {
    when: (context) => context.instance?.imageId === "mysql",
  });

  assert.equal(registry.contributions(EXTENSION_SLOTS.applicationCardActions, cardContext("mysql")).length, 1);
  assert.equal(registry.contributions(EXTENSION_SLOTS.applicationCardActions, cardContext("redis")).length, 0);
  assert.equal(
    registry.contributions(EXTENSION_SLOTS.applicationCardActions).length,
    1,
    "without a context the predicate cannot be evaluated, so nothing is filtered"
  );
});

test("a throwing predicate hides only its own contribution", () => {
  const registry = createRegistry();
  registry.register("broken", EXTENSION_SLOTS.applicationCardActions, noop, {
    when: () => {
      throw new Error("boom");
    },
  });
  registry.register("healthy", EXTENSION_SLOTS.applicationCardActions, noop);

  assert.deepEqual(
    registry
      .contributions(EXTENSION_SLOTS.applicationCardActions, cardContext("mysql"))
      .map((c) => c.imageId),
    ["healthy"]
  );
});

test("disposing removes a contribution once and notifies subscribers", () => {
  const registry = createRegistry();
  let changes = 0;
  registry.subscribe(() => {
    changes++;
  });

  const dispose = registry.register("mysql", EXTENSION_SLOTS.applicationsPanel, noop);
  assert.equal(changes, 1);

  dispose();
  assert.equal(registry.contributions(EXTENSION_SLOTS.applicationsPanel).length, 0);
  assert.equal(changes, 2);

  dispose();
  assert.equal(changes, 2, "a second dispose is a no-op");
});

test("removeImage drops every contribution from one image and leaves others", () => {
  const registry = createRegistry();
  registry.register("mysql", EXTENSION_SLOTS.sidebarHeaderActions, noop);
  registry.register("mysql", EXTENSION_SLOTS.applicationsPanel, noop);
  registry.register("redis", EXTENSION_SLOTS.applicationsPanel, noop);

  registry.removeImage("mysql");

  assert.equal(registry.contributions(EXTENSION_SLOTS.sidebarHeaderActions).length, 0);
  assert.deepEqual(
    registry.contributions(EXTENSION_SLOTS.applicationsPanel).map((c) => c.imageId),
    ["redis"]
  );
});

test("contribution ids are unique per image so slots can key on them", () => {
  const registry = createRegistry();
  registry.register("mysql", EXTENSION_SLOTS.applicationsPanel, noop);
  registry.register("mysql", EXTENSION_SLOTS.applicationsPanel, noop);

  const ids = registry
    .contributions(EXTENSION_SLOTS.applicationsPanel)
    .map((c) => c.id);
  assert.deepEqual(ids, ["mysql#1", "mysql#2"]);
});

// ---- install scope ---------------------------------------------------------

function chatContext(projectId?: string): ExtensionSlotContext {
  return { slot: EXTENSION_SLOTS.chatHeaderActions, chatId: "c1", projectId };
}

const sidebarContext: ExtensionSlotContext = {
  slot: EXTENSION_SLOTS.sidebarHeaderActions,
};

function registryWith(
  imageId: string,
  visibility: { global: boolean; projectIds: string[] }
) {
  const registry = createRegistry();
  registry.setVisibility(imageId, visibility);
  registry.register(imageId, EXTENSION_SLOTS.chatHeaderActions, noop);
  registry.register(imageId, EXTENSION_SLOTS.sidebarHeaderActions, noop);
  return registry;
}

test("a globally installed extension renders on every surface", () => {
  const registry = registryWith("global-app", { global: true, projectIds: [] });

  assert.equal(registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p1")).length, 1);
  assert.equal(registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p2")).length, 1);
  assert.equal(registry.contributions(EXTENSION_SLOTS.sidebarHeaderActions, sidebarContext).length, 1);
});

test("a project extension renders in its own project's chat and nowhere else", () => {
  const registry = registryWith("project-app", { global: false, projectIds: ["p1"] });
  registry.setActiveProject("p1");

  assert.equal(
    registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p1")).length,
    1,
    "its own project"
  );
  assert.equal(
    registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p2")).length,
    0,
    "another project's surface, while its own is open"
  );
  assert.equal(
    registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext(undefined)).length,
    1,
    "a project-less surface, while its own project is open"
  );
});

// Working elsewhere puts a project's extension away completely — including on
// that project's own sidebar row, which stays visible from other projects.
test("a project extension disappears entirely when working in another project", () => {
  const registry = registryWith("project-app", { global: false, projectIds: ["p1"] });
  const onOwnProjectSurface = () =>
    registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p1")).length;

  registry.setActiveProject("p1");
  assert.equal(onOwnProjectSurface(), 1, "while inside p1");

  registry.setActiveProject("p2");
  assert.equal(onOwnProjectSurface(), 0, "p1's own surface, seen from p2");

  registry.setActiveProject(null);
  assert.equal(onOwnProjectSurface(), 0, "p1's own surface, with no project open");
});

test("a project extension is gated by the project the user is working in", () => {
  const registry = registryWith("project-app", { global: false, projectIds: ["p1"] });
  const sidebar = () =>
    registry.contributions(EXTENSION_SLOTS.sidebarHeaderActions, sidebarContext).length;

  assert.equal(sidebar(), 0, "no project open yet");

  registry.setActiveProject("p1");
  assert.equal(sidebar(), 1, "working inside the project that installed it");

  registry.setActiveProject("p2");
  assert.equal(sidebar(), 0, "switched to another project");

  registry.setActiveProject(null);
  assert.equal(sidebar(), 0, "left every project");
});

test("an explicitly global surface is never reached by a project install", () => {
  const registry = createRegistry();
  registry.setVisibility("project-app", { global: false, projectIds: ["p1"] });
  registry.register("project-app", EXTENSION_SLOTS.applicationsPanel, noop);
  registry.setActiveProject("p1");

  assert.equal(
    registry.contributions(EXTENSION_SLOTS.applicationsPanel, {
      slot: EXTENSION_SLOTS.applicationsPanel,
      scope: "global",
    }).length,
    0,
    "the server-wide Applications page"
  );
  assert.equal(
    registry.contributions(EXTENSION_SLOTS.applicationsPanel, {
      slot: EXTENSION_SLOTS.applicationsPanel,
      scope: "project",
      projectId: "p1",
    }).length,
    1,
    "the project's own Applications page"
  );
});

test("an extension installed both globally and in a project renders everywhere", () => {
  const registry = registryWith("both", { global: true, projectIds: ["p1"] });

  assert.equal(registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p2")).length, 1);
});

test("re-recording visibility re-scopes contributions already registered", () => {
  const registry = createRegistry();
  registry.setActiveProject("p1");
  registry.setVisibility("app", { global: true, projectIds: [] });
  registry.register("app", EXTENSION_SLOTS.chatHeaderActions, noop);
  assert.equal(registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p2")).length, 1);

  // Uninstalled globally, still installed in p1: the same contribution narrows.
  registry.setVisibility("app", { global: false, projectIds: ["p1"] });
  assert.equal(registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p2")).length, 0);
  assert.equal(registry.contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p1")).length, 1);
});

test("two extensions in one slot render in order regardless of load order", () => {
  const registry = createRegistry();
  registry.setVisibility("sandbox", { global: true, projectIds: [] });
  registry.setVisibility("playground", { global: true, projectIds: [] });
  // Sandbox loads first but orders itself after the playground.
  registry.register("sandbox", EXTENSION_SLOTS.chatHeaderActions, noop, { order: -99 });
  registry.register("playground", EXTENSION_SLOTS.chatHeaderActions, noop, { order: -100 });

  assert.deepEqual(
    registry
      .contributions(EXTENSION_SLOTS.chatHeaderActions, chatContext("p1"))
      .map((c) => c.imageId),
    ["playground", "sandbox"]
  );
});
