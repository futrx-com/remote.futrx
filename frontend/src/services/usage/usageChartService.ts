import type {
  UsageChartMetric,
  UsageChartModel,
  UsageDayPoint,
} from "../../models/usage.ts";
import { usageFormatService } from "./usageFormatService.ts";

// Geometry and labels for the Usage page's inline-SVG bar chart. Kept out of
// the component so the scaling rules are unit-testable and the view stays a
// pure renderer.
class UsageChartService {
  build(daily: UsageDayPoint[], metric: UsageChartMetric): UsageChartModel {
    const values = daily.map((point) => (metric === "cost" ? point.costUsd : point.totalTokens));
    const peak = values.reduce((max, value) => Math.max(max, value), 0);
    const bars = daily.map((point, index) => {
      const value = values[index];
      return {
        day: point.day,
        value,
        // A zero peak would divide by zero; an all-empty window draws flat.
        ratio: peak > 0 ? value / peak : 0,
        runs: point.runs,
        label: `${point.day}: ${this.valueLabel(value, metric)} · ${point.runs} run${
          point.runs === 1 ? "" : "s"
        }`,
      };
    });
    return {
      bars,
      peak,
      peakLabel: this.peakLabel(peak, metric),
      isEmpty: peak === 0,
    };
  }

  private valueLabel(value: number, metric: UsageChartMetric): string {
    return metric === "cost"
      ? usageFormatService.usd(value)
      : `${usageFormatService.tokens(value)} tokens`;
  }

  private peakLabel(peak: number, metric: UsageChartMetric): string {
    return metric === "cost" ? usageFormatService.usd(peak) : usageFormatService.tokens(peak);
  }
}

export const usageChartService = new UsageChartService();
