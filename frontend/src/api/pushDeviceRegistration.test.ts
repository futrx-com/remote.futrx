import assert from "node:assert/strict";
import test from "node:test";

import {
  restoreDeviceRegistration,
  type PushDevice,
  type PushDevicePorts,
} from "./pushDeviceRegistration.ts";

interface Subscription {
  endpoint: string;
}

const HELD = "https://push.example.com/device";

interface ServerAnswer {
  /** What the ownership check returns, or the failure it raises. */
  owns?: boolean | Error;
  retiredKey?: boolean;
}

class RecordingPorts implements PushDevicePorts<Subscription> {
  readonly invalidated: string[] = [];
  readonly discarded: string[] = [];
  created = 0;
  #answer: ServerAnswer;

  constructor(answer: ServerAnswer = {}) {
    this.#answer = answer;
  }

  isSignedWithRetiredKey = (): boolean => this.#answer.retiredKey === true;

  ownsEndpoint = async (): Promise<boolean> => {
    if (this.#answer.owns instanceof Error) throw this.#answer.owns;
    return this.#answer.owns ?? true;
  };

  invalidateLocally = async (subscription: Subscription): Promise<void> => {
    this.invalidated.push(subscription.endpoint);
  };

  discardRegistration = async (subscription: Subscription): Promise<void> => {
    this.discarded.push(subscription.endpoint);
  };

  createRegistration = async (): Promise<void> => {
    this.created++;
  };
}

function device(overrides: Partial<PushDevice<Subscription>> = {}): PushDevice<Subscription> {
  return {
    subscription: { endpoint: HELD },
    optedIn: true,
    permissionGranted: true,
    ...overrides,
  };
}

test("a confirmed device is left exactly as it is", async () => {
  const ports = new RecordingPorts();

  assert.equal(await restoreDeviceRegistration(device(), ports), "registered");
  assert.deepEqual(ports.invalidated, []);
  assert.equal(ports.created, 0);
});

test("a device the server cannot confirm keeps its subscription", async () => {
  const ports = new RecordingPorts({ owns: new Error("502 while the backend restarts") });

  assert.equal(await restoreDeviceRegistration(device(), ports), "unverified");
  assert.deepEqual(ports.invalidated, []);
  assert.equal(ports.created, 0);
});

test("a subscription the server lost is recreated without asking again", async () => {
  const ports = new RecordingPorts({ owns: false });

  assert.equal(await restoreDeviceRegistration(device(), ports), "registered");
  assert.deepEqual(ports.invalidated, [HELD]);
  assert.equal(ports.created, 1);
});

test("a subscription signed with a retired key is replaced on both sides", async () => {
  const ports = new RecordingPorts({ retiredKey: true });

  assert.equal(await restoreDeviceRegistration(device(), ports), "registered");
  assert.deepEqual(ports.discarded, [HELD]);
  assert.equal(ports.created, 1);
});

test("a device with nothing registered is restored from a remembered opt-in", async () => {
  const ports = new RecordingPorts();

  assert.equal(
    await restoreDeviceRegistration(device({ subscription: null }), ports),
    "registered"
  );
  assert.equal(ports.created, 1);
});

test("an account that never opted in on this device is left alone", async () => {
  const ports = new RecordingPorts();

  assert.equal(
    await restoreDeviceRegistration(device({ subscription: null, optedIn: false }), ports),
    "absent"
  );
  assert.equal(ports.created, 0);
});

test("restoring never subscribes before permission is granted", async () => {
  const ports = new RecordingPorts();

  assert.equal(
    await restoreDeviceRegistration(
      device({ subscription: null, permissionGranted: false }),
      ports
    ),
    "absent"
  );
  assert.equal(ports.created, 0);
});
