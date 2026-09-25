import assert from "node:assert/strict";
import test from "node:test";

import {
  backendSummary,
  contextSummary,
  installSummary,
  uploadSummary,
} from "./showcaseExplorer.js";
import { activateFrontendShowcase } from "./showcase.js";
import { createUploadTracker } from "./uploadTracker.js";

test("registers a control in every frontend extension slot", () => {
  const iconSlots = [];
  const buttonSlots = [];
  const panelSlots = [];
  const events = [];
  const slots = {
    sidebarHeaderActions: "sidebar.header.actions",
    sidebarSearchActions: "sidebar.search.actions",
    projectRowActions: "sidebar.project.actions",
    chatHeaderActions: "chat.header.actions",
    composerActions: "chat.composer.actions",
    applicationCardActions: "applications.card.actions",
    applicationsPanel: "applications.panel",
    projectSettingsPanel: "project.settings.panel",
  };
  activateFrontendShowcase({
    slots,
    application: { id: "hello-remote" },
    ui: {
      addIconButton: (slot) => iconSlots.push(slot),
      addButton: (slot) => buttonSlots.push(slot),
      register: (slot) => panelSlots.push(slot),
    },
    events: { on: (name) => events.push(name) },
  });

  assert.deepEqual(iconSlots, [
    slots.sidebarHeaderActions,
    slots.sidebarSearchActions,
    slots.projectRowActions,
    slots.chatHeaderActions,
    slots.composerActions,
  ]);
  assert.deepEqual(buttonSlots, [slots.applicationCardActions]);
  assert.deepEqual(panelSlots, [slots.projectSettingsPanel]);
  assert.deepEqual(events, ["upload.completed"]);
  // applicationsPanel is the full greeting/container panel registered by
  // main.js; together these registrations cover all eight current slots.
});

test("summarizes install visibility and slot context", () => {
  assert.equal(
    installSummary({ global: true, projectIds: ["p1"] }),
    "global, project p1"
  );
  assert.equal(
    contextSummary({ projectName: "Demo", chatId: "c1", cwd: "/workspace/demo" }),
    "project: Demo · chat: c1 · cwd: /workspace/demo"
  );
  assert.equal(contextSummary({}), "no optional fields");
});

test("summarizes backend instances and observed uploads", () => {
  assert.equal(backendSummary({ available: false, instances: [] }), "unavailable");
  assert.equal(
    backendSummary({ available: true, instances: [{}, {}] }),
    "2 running instances"
  );
  assert.equal(
    uploadSummary({ count: 0, latest: null }),
    "0 · upload.completed is being observed"
  );
  assert.equal(
    uploadSummary({ count: 1, latest: { fileName: "demo.txt", size: 12 } }),
    "1 · latest: demo.txt (12 bytes)"
  );
  assert.equal(
    uploadSummary({ count: 0, latest: null, claimNext: true, claimed: 0 }),
    "0 · upload.completed is being observed · pass-through claim armed"
  );
});

test("an armed upload claim preserves the original attachment path", async () => {
  const uploads = createUploadTracker();
  uploads.armPassThroughClaim();
  let claimed;
  const upload = {
    path: "/workspace/.uploads/demo.txt",
    claim: (work) => { claimed = work; },
  };
  const used = uploads.observe(upload);

  assert.equal(used, true);
  assert.deepEqual(uploads.snapshot(), {
    count: 1,
    latest: upload,
    claimNext: false,
    claimed: 0,
  });
  assert.equal(await claimed, "/workspace/.uploads/demo.txt");
  assert.equal(uploads.snapshot().claimed, 1);
});
