import { API_ROUTES } from "../config/routes";
import type {
  UsageGroupBy,
  UsagePriceTable,
  UsageRebuildResult,
  UsageRecordPage,
  UsageSummary,
} from "../models/usage";
import { requestJson } from "./apiRequest";

export interface UsageSummaryQuery {
  from: number;
  to: number;
  groupBy?: UsageGroupBy;
  projectId?: string;
  chatId?: string;
}

export interface UsageRecordsQuery {
  from: number;
  to: number;
  projectId?: string;
  chatId?: string;
  limit?: number;
  cursor?: string;
}

export const usageApi = {
  summary: (query: UsageSummaryQuery) =>
    requestJson<UsageSummary>("GET", API_ROUTES.usage.summary(toQueryString(query))),
  records: (query: UsageRecordsQuery) =>
    requestJson<UsageRecordPage>("GET", API_ROUTES.usage.records(toQueryString(query))),
  projectSummary: (projectId: string, from: number, to: number) =>
    requestJson<UsageSummary>("GET", API_ROUTES.projects.usage(projectId, toQueryString({ from, to }))),
  prices: () => requestJson<UsagePriceTable>("GET", API_ROUTES.usage.prices),
  savePrices: (table: UsagePriceTable) =>
    requestJson<UsagePriceTable>("PUT", API_ROUTES.usage.prices, table),
  rebuild: () => requestJson<UsageRebuildResult>("POST", API_ROUTES.usage.rebuild),
};

/** Drops empty values so the backend sees only the filters actually chosen. */
function toQueryString(query: object): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query) as Array<[string, unknown]>) {
    if (value === undefined || value === null || value === "") continue;
    params.set(key, String(value));
  }
  return params.toString();
}
