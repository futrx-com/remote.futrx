import test from "node:test";
import assert from "node:assert/strict";
import type { UsageDayPoint } from "../../models/usage.ts";
import { usageChartService } from "./usageChartService.ts";

const daily: UsageDayPoint[] = [
  { day: "2026-08-01", totalTokens: 1000, costUsd: 0.5, runs: 2 },
  { day: "2026-08-02", totalTokens: 0, costUsd: 0, runs: 0 },
  { day: "2026-08-03", totalTokens: 4000, costUsd: 1.5, runs: 6 },
];

test("scales bars against the day with the most tokens", () => {
  const chart = usageChartService.build(daily);
  assert.equal(chart.peak, 4000);
  assert.deepEqual(
    chart.bars.map((bar) => bar.ratio),
    [0.25, 0, 1]
  );
  assert.equal(chart.isEmpty, false);
  assert.equal(chart.peakLabel, "4.0K");
});

test("an all-empty window draws flat instead of dividing by zero", () => {
  const chart = usageChartService.build([
    { day: "2026-08-01", totalTokens: 0, costUsd: 0, runs: 0 },
    { day: "2026-08-02", totalTokens: 0, costUsd: 0, runs: 0 },
  ]);
  assert.equal(chart.isEmpty, true);
  assert.ok(chart.bars.every((bar) => bar.ratio === 0));
});

test("labels each bar with its day, tokens and run count, never money", () => {
  const chart = usageChartService.build(daily);
  assert.equal(chart.bars[0].label, "2026-08-01: 1.0K tokens · 2 runs");
  assert.equal(chart.bars[1].label, "2026-08-02: 0 tokens · 0 runs");
  assert.ok(chart.bars.every((bar) => !bar.label.includes("$")));
});
