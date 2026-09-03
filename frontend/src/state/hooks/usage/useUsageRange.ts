import { useCallback, useState } from "preact/hooks";
import type { UsageRange, UsageRangePreset } from "../../../models/usage";
import { usageRangeService } from "../../../services/usage/usageRangeService.ts";
import { USAGE_DEFAULT_RANGE_PRESET } from "../../../config/usage.ts";

/** The selected window. Owns nothing but the range and how it is chosen. */
export function useUsageRange() {
  const [range, setRange] = useState<UsageRange>(() => usageRangeService.forPreset(USAGE_DEFAULT_RANGE_PRESET, Date.now()));

  const setPreset = useCallback((preset: UsageRangePreset) => {
    setRange((current) =>
      preset === "custom" ? { ...current, preset } : usageRangeService.forPreset(preset, Date.now())
    );
  }, []);

  const setCustomRange = useCallback((from: string, to: string) => {
    setRange((current) => usageRangeService.fromDates(current, from, to));
  }, []);

  return { range, setPreset, setCustomRange };
}
