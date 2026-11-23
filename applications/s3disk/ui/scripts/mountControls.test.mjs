import test from "node:test";
import assert from "node:assert/strict";
import { openMountControls } from "./mountControls.js";

test("closing mount controls leaves the view unchanged after a pending status reply", async () => {
  const status = { textContent: "Loading mount status…" };
  const view = {
    "[data-status]": status,
    "[data-actions]": { appendChild() {} },
    "[data-progress]": { textContent: "" },
    "[data-output]": { textContent: "" },
  };
  const body = { querySelector: selector => view[selector] };
  const previousDocument = globalThis.document;
  globalThis.document = {
    createElement: () => ({ style: {}, addEventListener() {} }),
  };

  try {
    let resolveStatus;
    const pendingStatus = new Promise(resolve => { resolveStatus = resolve; });
    const calls = [];
    let dispose;
    const remote = {
      backend: {
        call(route, target) {
          calls.push([route, target]);
          if (route === "status") return pendingStatus;
          if (route === "operation") return Promise.resolve({ running: false });
          throw new Error(`unexpected ${route} call`);
        },
      },
      ui: {
        openPopup({ mount }) { dispose = mount(body); },
      },
    };

    openMountControls(remote, { instance: { id: "mount-1" } }, { recent: () => [] });
    assert.deepEqual(calls, [["status", { instanceId: "mount-1" }]]);
    dispose();
    resolveStatus({ mounted: true, mountpoint: "/workspace/s3" });
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(status.textContent, "Loading mount status…");
    // The current refresh().then(poll) chain still makes this request after
    // close; this characterization intentionally preserves that sequencing.
    assert.deepEqual(calls.map(([route]) => route), ["status", "operation"]);
  } finally {
    globalThis.document = previousDocument;
  }
});
