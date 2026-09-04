import type {
  UsageRange,
  UsageRangeLabels,
  UsageRangePreset,
} from "../../models/usage.ts";
import {
  USAGE_DEFAULT_RANGE_PRESET,
  USAGE_RANGE_SPAN_DAYS,
} from "../../config/usage.ts";

/**
 * Date-range selection for the Usage page. Days are bounded in UTC because the
 * ledger buckets records by UTC day, so a range picked in any timezone lines up
 * with the bars drawn from the same response.
 */
class UsageRangeService {
  private readonly dayMs = 24 * 60 * 60 * 1000;

  /**
   * Resolves a preset against "now". `7d` and `30d` include today, so "7 days"
   * spans today plus the six days before it rather than a bare now-minus-7.
   */
  forPreset(preset: UsageRangePreset, now: number): UsageRange {
    const to = this.endOfUtcDay(now);
    switch (preset) {
      case "7d":
        return { preset, from: this.startOfWindow(now, USAGE_RANGE_SPAN_DAYS["7d"]), to };
      case "month": {
        const today = new Date(this.startOfUtcDay(now));
        const from = Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), 1);
        return { preset, from, to };
      }
      case "custom":
      case "30d":
      default:
        return {
          preset: preset === "custom" ? "custom" : USAGE_DEFAULT_RANGE_PRESET,
          from: this.startOfWindow(now, USAGE_RANGE_SPAN_DAYS[USAGE_DEFAULT_RANGE_PRESET]),
          to,
        };
    }
  }

  /**
   * Builds a custom range from two date inputs. Reversed inputs are swapped so
   * the picker cannot produce a window the backend rejects; malformed input
   * leaves the previous range untouched.
   */
  fromDates(current: UsageRange, fromValue: string, toValue: string): UsageRange {
    const from = this.fromDateInputValue(fromValue);
    const to = this.fromDateInputValue(toValue);
    if (from == null || to == null) return current;
    const [start, end] = from <= to ? [from, to] : [to, from];
    return { preset: "custom", from: start, to: this.endOfUtcDay(end) };
  }

  /** The `<input type=date>` values for a range's two ends. */
  labels(range: UsageRange): UsageRangeLabels {
    return {
      fromDate: this.toDateInputValue(range.from),
      toDate: this.toDateInputValue(range.to),
    };
  }

  /** First ms of a window that ends today and spans `days` UTC days. Today is
   *  one of them, which is why it steps back one fewer day than it spans. */
  private startOfWindow(now: number, days: number): number {
    return this.startOfUtcDay(now) - (days - 1) * this.dayMs;
  }

  /** Start of the UTC day containing `at`. */
  private startOfUtcDay(at: number): number {
    return Math.floor(at / this.dayMs) * this.dayMs;
  }

  /** Last millisecond of the UTC day containing `at`. */
  private endOfUtcDay(at: number): number {
    return this.startOfUtcDay(at) + this.dayMs - 1;
  }

  /** ISO `YYYY-MM-DD` for the UTC day containing `at`. */
  private toDateInputValue(at: number): string {
    return new Date(this.startOfUtcDay(at)).toISOString().slice(0, 10);
  }

  /** Parses an ISO `YYYY-MM-DD` as a UTC day start, or null when malformed. */
  private fromDateInputValue(value: string): number | null {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
    const parsed = Date.parse(`${value}T00:00:00.000Z`);
    return Number.isNaN(parsed) ? null : parsed;
  }
}

export const usageRangeService = new UsageRangeService();
