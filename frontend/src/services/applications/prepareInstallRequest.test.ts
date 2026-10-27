import { strict as assert } from "node:assert";
import { it } from "node:test";
import type { AppApplication } from "../../models/application.ts";
import { prepareInstallRequest } from "./prepareInstallRequest.ts";

const application = {
  id: "editor",
  name: "Editor",
  env: [{ key: "EDITOR_SETTINGS_JSON", label: "Editor settings.json", format: "json", default: "{}" }],
} as AppApplication;

it("keeps install defaults and validates the complete JSON settings object", () => {
  const input = { name: "  ", env: {}, externalPort: "  " };
  assert.deepEqual(prepareInstallRequest(application, input, false), {
    applicationId: "editor",
    name: "Editor",
    env: input.env,
    externalPort: undefined,
  });
  assert.equal(prepareInstallRequest(application, { ...input, name: " Editor ", externalPort: "8400" }, true).name, "Editor");
  assert.throws(() => prepareInstallRequest(application, { ...input, env: { EDITOR_SETTINGS_JSON: "[]" } }, false),
    { message: "Editor settings.json must be a JSON object." });
  assert.throws(() => prepareInstallRequest(application, { ...input, env: { EDITOR_SETTINGS_JSON: "{" } }, false),
    { message: "Editor settings.json must be valid JSON." });
  assert.throws(() => prepareInstallRequest(application, { ...input, externalPort: "0" }, true),
    { message: "External port must be 1–65535." });
});
