import assert from "node:assert/strict";
import test from "node:test";
import { agentBrowserStatusState } from "./agentBrowserStatusState.ts";

test("a ready browser with an address is shown and stops polling", () => {
  assert.deepEqual(
    agentBrowserStatusState.resolve({ status: "ready", url: "https://vnc.example" }),
    { status: "ready", guiUrl: "https://vnc.example", error: null, keepPolling: false },
  );
});

test("a ready browser without an address is a failure, not a ready browser", () => {
  const view = agentBrowserStatusState.resolve({ status: "ready", url: "" });
  assert.equal(view.status, "error");
  assert.equal(view.guiUrl, "");
  assert.equal(view.keepPolling, false);
  assert.match(view.error ?? "", /incomplete address/);
});

test("an error keeps the backend's reason when it gives one", () => {
  assert.equal(
    agentBrowserStatusState.resolve({ status: "error", error: "container is down" }).error,
    "container is down",
  );
});

test("an error without a reason falls back to a generic one", () => {
  assert.match(
    agentBrowserStatusState.resolve({ status: "error" }).error ?? "",
    /Failed to start/,
  );
});

test("only a browser still coming up is worth polling again", () => {
  for (const status of ["starting", "core-ready"] as const) {
    assert.equal(agentBrowserStatusState.resolve({ status }).keepPolling, true, status);
  }
  // "idle" is deliberately absent: AgentBrowserServerStatus excludes it, so the
  // backend can never report it.
  assert.equal(agentBrowserStatusState.resolve({ status: "stopped" }).keepPolling, false);
});
