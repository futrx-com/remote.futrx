import assert from "node:assert/strict";
import test from "node:test";
import { createFrontendBuildStore } from "./frontendBuildStore.ts";

test("records the build the server names and keeps it through failed checks", async () => {
  const answers: Array<() => Promise<string | null>> = [
    async () => "b1",
    async () => null,
    async () => { throw new Error("restarting"); },
    async () => "b2",
  ];
  const store = createFrontendBuildStore(() => answers.shift()!());

  await store.getState().check();
  assert.equal(store.getState().served, "b1");
  await store.getState().check();
  assert.equal(store.getState().served, "b1");
  await store.getState().check();
  assert.equal(store.getState().served, "b1");
  await store.getState().check();
  assert.equal(store.getState().served, "b2");
});

test("overlapping checks share one request", async () => {
  let calls = 0;
  let answer: ((build: string) => void) | undefined;
  const store = createFrontendBuildStore(() => {
    calls++;
    return new Promise((resolve) => { answer = resolve; });
  });

  const first = store.getState().check();
  const second = store.getState().check();
  assert.equal(calls, 1);
  answer!("b1");
  await Promise.all([first, second]);
  assert.equal(store.getState().served, "b1");

  void store.getState().check();
  assert.equal(calls, 2);
});
