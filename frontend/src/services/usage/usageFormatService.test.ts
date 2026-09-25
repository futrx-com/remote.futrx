import test from "node:test";
import assert from "node:assert/strict";
import { usageFormatService } from "./usageFormatService.ts";

test("formats tokens compactly", () => {
  assert.equal(usageFormatService.tokens(0), "0");
  assert.equal(usageFormatService.tokens(812), "812");
  assert.equal(usageFormatService.tokens(1234), "1.2K");
  assert.equal(usageFormatService.tokens(34_500), "35K");
  assert.equal(usageFormatService.tokens(1_250_000), "1.3M");
  assert.equal(usageFormatService.tokens(12_500_000), "13M");
});
