import assert from "node:assert/strict";
import test from "node:test";
import {
  formatSize,
  hostCommitmentPercent,
  parseSize,
  usageMeters,
  validateFleetSettings,
  validateOverride,
} from "./resourcePolicyState.ts";

const MiB = 1024 ** 2;
const GiB = 1024 ** 3;

test("parseSize accepts the LXD byte-size grammar and rejects everything else", () => {
  const cases: Array<[string | undefined, number | undefined]> = [
    ["2GiB", 2 * GiB],
    ["768MiB", 768 * MiB],
    ["1TiB", 1024 * GiB],
    ["1.5GiB", 1.5 * GiB],
    [" 3GiB ", 3 * GiB],
    ["", undefined],
    [undefined, undefined],
    ["20GB", undefined],
    ["20G", undefined],
    ["0GiB", undefined],
    ["twenty", undefined],
    ["-2GiB", undefined],
  ];
  for (const [input, want] of cases) {
    assert.equal(parseSize(input), want, `parseSize(${JSON.stringify(input)})`);
  }
});

test("formatSize picks the largest unit that divides exactly", () => {
  assert.equal(formatSize(2 * GiB), "2GiB");
  assert.equal(formatSize(768 * MiB), "768MiB");
  assert.equal(formatSize(1024 * GiB), "1TiB");
  assert.equal(formatSize(3 * GiB + 256 * MiB), "3328MiB");
  assert.equal(formatSize(0), "—");
  assert.equal(formatSize(undefined), "—");
});

test("validateOverride bounds a project override by the fleet ceiling", () => {
  const ceiling = { cpu: "4", memory: "3GiB", disk: "40GiB" };

  assert.equal(validateOverride({}, ceiling), undefined);
  assert.equal(validateOverride({ cpu: "4", memory: "3GiB", disk: "40GiB" }, ceiling), undefined);
  assert.equal(validateOverride({ cpu: " " }, ceiling), undefined);

  assert.match(validateOverride({ cpu: "0" }, ceiling) ?? "", /whole number/);
  assert.match(validateOverride({ cpu: "2.5" }, ceiling) ?? "", /whole number/);
  assert.match(validateOverride({ cpu: "8" }, ceiling) ?? "", /fleet maximum of 4/);
  assert.match(validateOverride({ memory: "8 gigs" }, ceiling) ?? "", /MiB, GiB, or TiB/);
  assert.match(validateOverride({ memory: "8GiB" }, ceiling) ?? "", /fleet maximum of 3GiB/);
  assert.match(validateOverride({ disk: "80GiB" }, ceiling) ?? "", /fleet maximum of 40GiB/);
});

test("validateOverride without a ceiling only checks the grammar", () => {
  assert.equal(validateOverride({ cpu: "64", memory: "64GiB", disk: "1TiB" }), undefined);
  assert.match(validateOverride({ memory: "64" }) ?? "", /MiB, GiB, or TiB/);
});

test("validateFleetSettings enforces the policy's internal consistency", () => {
  const defaults = { memory: "2GiB", cpu: 2, processes: 2000, disk: "20GiB" };
  const maxOverride = { memory: "3GiB", cpu: 4, disk: "40GiB" };
  const host = {
    memoryBytes: 8 * GiB,
    cpus: 4,
    diskBytes: 200 * GiB,
    reserveMemoryBytes: 768 * MiB,
    budgetMemoryBytes: 8 * GiB - 768 * MiB,
    committedMemoryBytes: 0,
    runningContainers: 0,
  };

  assert.equal(validateFleetSettings(defaults, "768MiB", maxOverride, 0, host), undefined);
  assert.equal(validateFleetSettings(defaults, "768MiB", maxOverride, 5, host), undefined);

  assert.match(
    validateFleetSettings({ ...defaults, memory: "big" }, "768MiB", maxOverride, 0, host) ?? "",
    /Default memory must use/
  );
  assert.match(
    validateFleetSettings({ ...defaults, memory: "64MiB" }, "768MiB", maxOverride, 0, host) ?? "",
    /at least 256MiB/
  );
  assert.match(
    validateFleetSettings({ ...defaults, cpu: 0 }, "768MiB", maxOverride, 0, host) ?? "",
    /Default CPU/
  );
  assert.match(
    validateFleetSettings({ ...defaults, processes: 0 }, "768MiB", maxOverride, 0, host) ?? "",
    /process limit/
  );
  assert.match(
    validateFleetSettings(defaults, "not a size", maxOverride, 0, host) ?? "",
    /Host reserve must use/
  );
  assert.match(
    validateFleetSettings(defaults, "768MiB", { ...maxOverride, memory: "1GiB" }, 0, host) ?? "",
    /cannot be below the default/
  );
  assert.match(
    validateFleetSettings(defaults, "768MiB", { ...maxOverride, cpu: 1 }, 0, host) ?? "",
    /CPU cannot be below/
  );
  assert.match(
    validateFleetSettings(defaults, "768MiB", { ...maxOverride, disk: "10GiB" }, 0, host) ?? "",
    /disk cannot be below/
  );
  assert.match(
    validateFleetSettings(defaults, "768MiB", maxOverride, -1, host) ?? "",
    /Maximum running containers/
  );
});

test("validateFleetSettings measures the policy against real host capacity", () => {
  // The operator's box today: 1 vCPU and 4 GiB.
  const host = {
    memoryBytes: 4 * GiB,
    cpus: 1,
    diskBytes: 40 * GiB,
    reserveMemoryBytes: 768 * MiB,
    budgetMemoryBytes: 4 * GiB - 768 * MiB,
    committedMemoryBytes: 0,
    runningContainers: 0,
  };
  const defaults = { memory: "3GiB", cpu: 1, processes: 2000, disk: "10GiB" };

  assert.equal(validateFleetSettings(defaults, "768MiB", {}, 0, host), undefined);
  assert.match(
    validateFleetSettings({ ...defaults, memory: "4GiB" }, "768MiB", {}, 0, host) ?? "",
    /exceeds the 3328MiB left after the host reserve/
  );
  assert.match(
    validateFleetSettings(defaults, "8GiB", {}, 0, host) ?? "",
    /leaves nothing of the host's 4GiB/
  );
  // Without host facts the capacity checks are skipped rather than guessed.
  assert.equal(validateFleetSettings({ ...defaults, memory: "4GiB" }, "768MiB", {}, 0), undefined);
});

test("usageMeters pair live consumption with the enforced limit", () => {
  const meters = usageMeters(
    { memoryCurrentBytes: 1 * GiB, diskUsageBytes: 5 * GiB },
    { memory: "2GiB", disk: "20GiB" }
  );

  assert.deepEqual(meters.map((m) => m.label), ["Memory", "Disk"]);
  assert.equal(meters[0].percent, 50);
  assert.equal(meters[0].detail, "1GiB of 2GiB");
  assert.equal(meters[1].percent, 25);
  assert.equal(meters[1].detail, "5GiB of 20GiB");
});

test("usageMeters degrade rather than draw a misleading bar", () => {
  const stopped = usageMeters(undefined, { memory: "2GiB" });
  assert.equal(stopped[0].percent, undefined);
  assert.equal(stopped[0].detail, "limit 2GiB");
  assert.equal(stopped[1].percent, undefined);
  assert.equal(stopped[1].detail, "no quota");

  const unquotedDisk = usageMeters({ diskUsageBytes: 3 * GiB }, {});
  assert.equal(unquotedDisk[1].percent, undefined);
  assert.equal(unquotedDisk[1].detail, "3GiB used, no quota");

  // Usage above the limit is clamped so the bar never overflows its track.
  const over = usageMeters({ memoryCurrentBytes: 3 * GiB }, { memory: "2GiB" });
  assert.equal(over[0].percent, 100);
});

test("usageMeters fall back to the LXD-reported memory total when no limit is set", () => {
  const meters = usageMeters(
    { memoryCurrentBytes: 512 * MiB, memoryTotalBytes: 2 * GiB },
    {}
  );
  assert.equal(meters[0].percent, 25);
  assert.equal(meters[0].detail, "512MiB of 2GiB");
});

test("hostCommitmentPercent reports how much of the workspace budget is claimed", () => {
  const host = {
    memoryBytes: 8 * GiB,
    cpus: 4,
    diskBytes: 200 * GiB,
    reserveMemoryBytes: 0,
    budgetMemoryBytes: 8 * GiB,
    committedMemoryBytes: 6 * GiB,
    runningContainers: 3,
  };
  assert.equal(hostCommitmentPercent(host), 75);
  assert.equal(hostCommitmentPercent({ ...host, budgetMemoryBytes: 0 }), undefined);
  assert.equal(hostCommitmentPercent(undefined), undefined);
});
