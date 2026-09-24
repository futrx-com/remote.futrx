import assert from "node:assert/strict";
import test from "node:test";

import type { AppApplication } from "../../models/application.ts";
import type {
  ExtensionMediaOptions,
  ExtensionRegistry,
} from "../../models/extension.ts";
import { mediaViewerStore } from "../../state/stores/media/mediaViewerStore.ts";
import { createExtensionApi } from "./extensionApi.ts";

const application: AppApplication = {
  id: "media-extension",
  name: "Media Extension",
  version: "1",
  scopes: ["global"],
  port: { internal: 0, defaultExternal: 0 },
  ui: { entry: "scripts/main.js" },
};

const registry: ExtensionRegistry = {
  register: () => () => {},
  registerWorkspacePane: () => () => {},
  setVisibility: () => {},
  removeApplication: () => {},
};

test.afterEach(() => {
  mediaViewerStore.getState().close();
});

test("openMedia delegates the typed item to the app-wide media viewer", () => {
  const media: ExtensionMediaOptions = {
    url: "/api/applications/instance-1/backend/files/media?path=demo.mp4",
    name: "demo.mp4",
    kind: "video",
  };
  const remote = createExtensionApi(
    application,
    { global: true, projectIds: [] },
    [],
    registry,
  );

  const result = remote.ui.openMedia(media);

  assert.equal(result, undefined);
  assert.equal(mediaViewerStore.getState().item, media);
});
