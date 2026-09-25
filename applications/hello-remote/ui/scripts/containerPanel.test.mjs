import assert from "node:assert/strict";
import test from "node:test";

import {
  formatBytes,
  formatDuration,
  mountContainerPanel,
} from "./containerPanel.js";

test("formats container memory in GiB", () => {
  assert.equal(formatBytes(8 * 1024 ** 3), "8.0 GiB");
  assert.equal(formatBytes(16 * 1024 ** 3), "16 GiB");
  assert.equal(formatBytes(-1), "Unknown");
});

test("formats container uptime in days, hours, and minutes", () => {
  assert.equal(formatDuration(2 * 86400 + 3 * 3600), "2d 3h");
  assert.equal(formatDuration(2 * 3600 + 15 * 60), "2h 15m");
  assert.equal(formatDuration(Number.NaN), "Unknown");
});

test("loads container facts and hides the loading status on success", async () => {
  const elements = containerElements();
  const backend = {
    call: () => Promise.resolve({
      name: "project-1",
      hostname: "hello",
      operatingSystem: "Ubuntu",
      kernel: "Linux",
      architecture: "x86_64",
      cpuCount: 4,
      memoryTotalBytes: 8 * 1024 ** 3,
      uptimeSeconds: 2 * 3600 + 15 * 60,
    }),
  };

  mountContainerPanel(elements.host, backend, {}, () => false);
  assert.equal(elements.refresh.disabled, true);
  assert.equal(elements.status.hidden, false);
  assert.equal(elements.status.textContent, "Inspecting the container…");

  await new Promise(setImmediate);
  assert.equal(elements.refresh.disabled, false);
  assert.equal(elements.status.hidden, true);
  assert.equal(elements.facts.hidden, false);
  assert.equal(elements.fields.get("name").textContent, "project-1");
  assert.equal(elements.fields.get("memory").textContent, "8.0 GiB");
  assert.equal(elements.fields.get("uptime").textContent, "2h 15m");
});

function containerElements() {
  const refresh = new EventTarget();
  refresh.disabled = false;
  const status = { hidden: true, textContent: "" };
  const facts = { hidden: true };
  const fields = new Map(
    ["name", "hostname", "os", "kernel", "architecture", "cpus", "memory", "uptime"]
      .map((name) => [name, { textContent: "" }])
  );
  return {
    refresh,
    status,
    facts,
    fields,
    host: {
      querySelector(selector) {
        if (selector === "[data-container-refresh]") return refresh;
        if (selector === "[data-container-status]") return status;
        if (selector === "[data-container-facts]") return facts;
        return fields.get(selector.match(/^\[data-container-(.+)]$/)?.[1]);
      },
    },
  };
}
