import { strict as assert } from "node:assert";
import { it } from "node:test";
import type { AppApplication } from "../../models/application.ts";
import { prepareInstallRequest } from "./prepareInstallRequest.ts";

const application = {
  id: "code-server",
  name: "Code Server",
  env: [{ key: "CODE_SERVER_SETTINGS_JSON", label: "VS Code settings.json", format: "json", default: "{}" }],
} as AppApplication;

it("keeps install defaults and validates the complete JSON settings object", () => {
  const input = { name: "  ", env: {}, externalPort: "  " };
  assert.deepEqual(prepareInstallRequest(application, input, false), {
    applicationId: "code-server",
    name: "Code Server",
    env: input.env,
    externalPort: undefined,
  });
  assert.equal(prepareInstallRequest(application, { ...input, name: " Editor ", externalPort: "8842" }, true).name, "Editor");
  assert.throws(() => prepareInstallRequest(application, { ...input, env: { CODE_SERVER_SETTINGS_JSON: "[]" } }, false),
    { message: "VS Code settings.json must be a JSON object." });
  assert.throws(() => prepareInstallRequest(application, { ...input, env: { CODE_SERVER_SETTINGS_JSON: "{" } }, false),
    { message: "VS Code settings.json must be valid JSON." });
  assert.throws(() => prepareInstallRequest(application, { ...input, externalPort: "0" }, true),
    { message: "External port must be 1–65535." });
});
