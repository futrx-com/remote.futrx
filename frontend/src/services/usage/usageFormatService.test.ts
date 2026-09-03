import test from "node:test";
import assert from "node:assert/strict";
import { EMPTY_USAGE_TOTALS } from "../../models/usage.ts";
import { usageFormatService } from "./usageFormatService.ts";

test("formats tokens compactly", () => {
  assert.equal(usageFormatService.tokens(0), "0");
  assert.equal(usageFormatService.tokens(812), "812");
  assert.equal(usageFormatService.tokens(1234), "1.2K");
  assert.equal(usageFormatService.tokens(34_500), "35K");
  assert.equal(usageFormatService.tokens(1_250_000), "1.3M");
  assert.equal(usageFormatService.tokens(12_500_000), "13M");
});

test("keeps sub-cent costs visible", () => {
  assert.equal(usageFormatService.usd(0), "$0.00");
  assert.equal(usageFormatService.usd(0.0007), "$0.0007");
  assert.equal(usageFormatService.usd(1.239), "$1.24");
});

test("marks estimated and unpriced totals", () => {
  assert.equal(
    usageFormatService.costWithConfidence({ ...EMPTY_USAGE_TOTALS, costUsd: 2, runs: 2 }),
    "$2.00"
  );
  assert.equal(
    usageFormatService.costWithConfidence({ ...EMPTY_USAGE_TOTALS, costUsd: 2, estimatedCostUsd: 2 }),
    "~$2.00"
  );
  assert.equal(
    usageFormatService.costWithConfidence({ ...EMPTY_USAGE_TOTALS, costUsd: 2, estimatedCostUsd: 0.5 }),
    "$2.00*"
  );
});

test("explains where a total is uncertain", () => {
  assert.equal(usageFormatService.confidenceNote(EMPTY_USAGE_TOTALS), null);
  assert.equal(
    usageFormatService.confidenceNote({ ...EMPTY_USAGE_TOTALS, costUsd: 2, estimatedCostUsd: 0.5 }),
    "$0.50 estimated from the price table"
  );
  assert.equal(
    usageFormatService.confidenceNote({ ...EMPTY_USAGE_TOTALS, unpricedRuns: 1 }),
    "1 run with unknown cost"
  );
  assert.equal(
    usageFormatService.confidenceNote({ ...EMPTY_USAGE_TOTALS, estimatedCostUsd: 0.5, unpricedRuns: 3 }),
    "$0.50 estimated from the price table · 3 runs with unknown cost"
  );
});
