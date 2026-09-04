import assert from "node:assert/strict";
import test from "node:test";
import { projectPreviewUrlService } from "./projectPreviewUrlService.ts";

const publicHostname = "remote.example.com";

test("builds a preview URL beneath the runtime public hostname", () => {
  assert.equal(
    projectPreviewUrlService.build("demo", 4173, publicHostname),
    "https://demo--4173.dev.remote.example.com",
  );
});

test("extracts and validates only URLs for the runtime public hostname", () => {
  const expected = "https://demo--4173.dev.remote.example.com/path";
  const urls = projectPreviewUrlService.findInText(
    `custom ${expected}. production https://demo--4173.dev.remote.futrx.com`,
    publicHostname,
  );

  assert.deepEqual(urls, [expected]);
  assert.equal(projectPreviewUrlService.belongsToProject(expected, "demo", publicHostname), true);
  assert.equal(
    projectPreviewUrlService.belongsToProject(
      "https://demo--4173.dev.remote.futrx.com",
      "demo",
      publicHostname,
    ),
    false,
  );
});

test("rejects invalid preview ports", () => {
  assert.equal(
    projectPreviewUrlService.belongsToProject(
      "https://demo--1023.dev.remote.example.com",
      "demo",
      publicHostname,
    ),
    false,
  );
  assert.equal(projectPreviewUrlService.port("https://demo--4173.dev.remote.example.com"), 4173);
});
