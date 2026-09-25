import type { UsageRangePreset } from "../models/usage";

/**
 * Inclusive day spans for the fixed-length windows: "7 days" is today plus the
 * six before it. The labels below are rendered from these numbers rather than
 * repeating them, so a button can never promise a window the range service
 * does not actually produce.
 */
export const USAGE_RANGE_SPAN_DAYS = { "7d": 7, "30d": 30 } as const;

/** The window the page opens on, and what an unrecognised preset falls back to. */
export const USAGE_DEFAULT_RANGE_PRESET = "30d";

/** The windows the Usage page offers, in the order it lists them. */
export const USAGE_RANGE_PRESETS: Array<{ id: UsageRangePreset; label: string }> = [
  { id: "7d", label: `${USAGE_RANGE_SPAN_DAYS["7d"]} days` },
  { id: "30d", label: `${USAGE_RANGE_SPAN_DAYS["30d"]} days` },
  { id: "month", label: "This month" },
  { id: "custom", label: "Custom" },
];
