import assert from "node:assert/strict";
import test from "node:test";

import { mountServicePanel, portSummary } from "./servicePanel.js";

test("describes the host-to-container port mapping", () => {
  assert.equal(
    portSummary(4781, 4780),
    "127.0.0.1:4781 → container:4780/tcp"
  );
  assert.equal(portSummary(undefined, 4780), "Unknown");
});

test("loads service facts and removes its refresh listener on cleanup", async () => {
  const elements = serviceElements();
  const calls = [];
  const backend = {
    call: (path, target) => {
      calls.push([path, target]);
      return Promise.resolve({
        service: "hello-remote",
        version: "build-1",
        provisionedVersion: "14",
        externalPort: 4781,
        internalPort: 4780,
        message: "Hello from the container service.",
      });
    },
  };
  const target = { projectId: "project-1" };

  const dispose = mountServicePanel(elements.host, backend, target, () => false);
  assert.equal(elements.refresh.disabled, true);
  assert.equal(elements.status.textContent, "Calling the supervised service…");

  await new Promise(setImmediate);
  assert.deepEqual(calls, [["service", target]]);
  assert.equal(elements.refresh.disabled, false);
  assert.equal(elements.facts.hidden, false);
  assert.equal(elements.fields.get("unit").textContent, "hello-remote");
  assert.equal(elements.fields.get("provisioning").textContent, "14");
  assert.equal(elements.fields.get("ports").textContent, "127.0.0.1:4781 → container:4780/tcp");
  assert.equal(elements.status.textContent, "Hello from the container service.");

  dispose();
  elements.refresh.dispatchEvent(new Event("click"));
  assert.equal(calls.length, 1);
});

function serviceElements() {
  const refresh = new EventTarget();
  refresh.disabled = false;
  const status = { textContent: "" };
  const facts = { hidden: true };
  const fields = new Map(
    ["unit", "version", "provisioning", "ports"]
      .map((name) => [name, { textContent: "" }])
  );
  return {
    refresh,
    status,
    facts,
    fields,
    host: {
      querySelector(selector) {
        if (selector === "[data-service-refresh]") return refresh;
        if (selector === "[data-service-status]") return status;
        if (selector === "[data-service-facts]") return facts;
        return fields.get(selector.match(/^\[data-service-(.+)]$/)?.[1]);
      },
    },
  };
}
