import assert from "node:assert/strict";
import test from "node:test";
import { createShellConnection } from "./connection.js";

const encode = (text) => Buffer.from(text).toString("base64");
const decode = (bytes) => Buffer.from(bytes).toString();
const settle = () => new Promise((resolve) => setImmediate(resolve));

function failure(status) {
  const error = new Error(`status ${status}`);
  error.status = status;
  return error;
}

// A backend whose every call waits until the test answers it.
function fakeBackend() {
  const calls = [];
  const call = (kind, args) => new Promise((resolve, reject) => {
    calls.push({ kind, args, resolve, reject });
  });
  return {
    calls,
    api: {
      attach: (body) => call("attach", body),
      read: (target, signal) => call("read", { ...target, signal }),
      write: (target, data) => call("write", { ...target, data }),
      resize: (body) => call("resize", body),
    },
    next(kind) {
      const index = calls.findIndex((entry) => entry.kind === kind);
      assert.notEqual(index, -1, `no pending ${kind} in ${calls.map((entry) => entry.kind)}`);
      return calls.splice(index, 1)[0];
    },
  };
}

function fakeTimers() {
  const pending = new Map();
  let id = 0;
  return {
    pending,
    setTimeout: (callback, delay) => { pending.set(++id, { callback, delay }); return id; },
    clearTimeout: (timer) => { pending.delete(timer); },
    fire() {
      const [[timer, entry]] = pending;
      pending.delete(timer);
      entry.callback();
      return entry.delay;
    },
  };
}

function connect(overrides = {}) {
  const backend = fakeBackend();
  const timers = fakeTimers();
  const seen = { output: "", resets: 0, ended: 0, connected: 0, statuses: [] };
  const connection = createShellConnection({
    api: backend.api,
    session: "chat-1",
    cwd: "/workspace/api",
    size: () => ({ cols: 80, rows: 24 }),
    onOutput: (bytes) => { seen.output += decode(bytes); },
    onReset: () => { seen.resets++; seen.output = ""; },
    onEnded: () => { seen.ended++; },
    onConnected: () => { seen.connected++; },
    onStatus: (status) => seen.statuses.push(status),
    timers,
    ...overrides,
  });
  return { backend, timers, seen, connection };
}

async function attached(backend, generation = "1") {
  const attach = backend.next("attach");
  attach.resolve({ session: "chat-1", generation });
  await settle();
  return attach;
}

test("a shell is attached, replayed and then read by offset", async () => {
  const { backend, seen, connection } = connect();
  connection.start();
  const attach = await attached(backend);
  assert.deepEqual(
    { session: attach.args.session, cwd: attach.args.cwd, cols: attach.args.cols, rows: attach.args.rows },
    { session: "chat-1", cwd: "/workspace/api", cols: 80, rows: 24 },
  );
  assert.deepEqual(seen.statuses, ["connecting", "connected"]);
  assert.equal(seen.connected, 1);

  let read = backend.next("read");
  assert.equal(read.args.offset, -1);
  assert.equal(read.args.generation, "1");
  read.resolve({ offset: 6, data: encode("$ old\n"), reset: true });
  await settle();
  assert.equal(seen.resets, 1);
  assert.equal(seen.output, "$ old\n");

  read = backend.next("read");
  assert.equal(read.args.offset, 6);
  read.resolve({ offset: 6, data: null });
  await settle();
  read = backend.next("read");
  assert.equal(read.args.offset, 6);
  read.resolve({ offset: 9, data: encode("new") });
  await settle();
  assert.equal(seen.output, "$ old\nnew");
  connection.dispose();
});

test("typing is sent in order, batching what was typed meanwhile", async () => {
  const { backend, connection } = connect();
  connection.input("lost before attach");
  connection.start();
  await attached(backend);
  connection.input("l");
  connection.input("s");
  connection.input("\r");
  const first = backend.next("write");
  assert.equal(first.args.data, "l");
  assert.equal(backend.calls.some((entry) => entry.kind === "write"), false);
  first.resolve({ ok: true });
  await settle();
  const second = backend.next("write");
  assert.equal(second.args.data, "s\r");
  assert.equal(second.args.generation, "1");
  connection.dispose();
});

test("an exited shell waits for Enter before starting another", async () => {
  const { backend, seen, connection } = connect();
  connection.start();
  await attached(backend);
  backend.next("read").resolve({ offset: 4, data: encode("bye\n"), exited: true });
  await settle();
  assert.equal(seen.ended, 1);
  assert.equal(seen.statuses.at(-1), "ended");
  assert.equal(backend.calls.length, 0, "nothing more was requested");
  connection.input("x");
  connection.setVisible(true);
  assert.equal(backend.calls.length, 0, "only Enter starts a new shell");
  connection.input("\r");
  await settle();
  backend.next("attach");
  assert.equal(seen.statuses.at(-1), "connecting");
  connection.dispose();
});

test("a lost connection is retried with backoff only while visible", async () => {
  const { backend, timers, seen, connection } = connect();
  connection.setVisible(true);
  connection.start();
  await attached(backend);
  backend.next("read").reject(new Error("network down"));
  await settle();
  assert.equal(seen.statuses.at(-1), "reconnecting");
  assert.equal(timers.fire(), 1000);
  backend.next("attach").reject(new Error("still down"));
  await settle();
  assert.equal(timers.fire(), 2000);
  await attached(backend);
  assert.equal(seen.statuses.at(-1), "connected");

  // Hidden, a failure waits; showing the pane reconnects at once.
  connection.setVisible(false);
  backend.next("read").reject(new Error("network down"));
  await settle();
  assert.equal(timers.pending.size, 0);
  connection.setVisible(true);
  await settle();
  backend.next("attach");
  assert.equal(seen.statuses.at(-1), "connecting");
  connection.dispose();
});

test("a session the backend no longer has is reopened straight away", async () => {
  const { backend, timers, connection } = connect();
  connection.setVisible(true);
  connection.start();
  await attached(backend);
  backend.next("read").reject(failure(404));
  await settle();
  assert.equal(timers.fire(), 0);
  await attached(backend, "2");
  assert.equal(backend.next("read").args.generation, "2");
  connection.dispose();
});

test("resizes go to the attached shell, and disposal stops everything", async () => {
  const { backend, timers, connection } = connect();
  connection.resize({ cols: 10, rows: 5 });
  connection.start();
  await attached(backend);
  connection.resize({ cols: 120, rows: 40 });
  assert.deepEqual(
    (({ session, generation, cols, rows }) => ({ session, generation, cols, rows }))(backend.next("resize").args),
    { session: "chat-1", generation: "1", cols: 120, rows: 40 },
  );
  const read = backend.next("read");
  connection.dispose();
  assert.equal(read.args.signal.aborted, true);
  read.reject(Object.assign(new Error("aborted"), { name: "AbortError" }));
  await settle();
  assert.equal(backend.calls.length, 0);
  assert.equal(timers.pending.size, 0);
});
