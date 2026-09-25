import { API_ROUTES } from "../../config/routes.ts";
import type { AccountQuota, QuotaWindow, QuotaWindowKind } from "../../models/agentQuota.ts";
import { sendHttpRequest } from "../../transport/http.ts";

export const agentQuotaApi = {
  /** Reads stored plan windows by provider account. A response that cannot be
   *  trusted rejects rather than reading as empty, so callers keep their last
   *  good snapshot. */
  async list(signal?: AbortSignal): Promise<AccountQuota[]> {
    const response = await sendHttpRequest("GET", API_ROUTES.agentQuota, undefined, {
      signal,
      cache: "no-store",
    });
    if (!response.ok) throw new Error("Failed to load subscription quota.");
    const body: unknown = await response.json();
    if (!isRecord(body)) throw new Error("Invalid subscription quota response.");
    if (!("accounts" in body) && Object.keys(body).length > 0) {
      throw new Error("Invalid subscription quota response.");
    }
    if (body.accounts == null) return [];
    if (!Array.isArray(body.accounts)) throw new Error("Invalid subscription quota response.");
    return body.accounts.map(readAccountQuota);
  },
};

function readAccountQuota(value: unknown): AccountQuota {
  if (!isRecord(value) || typeof value.provider !== "string" || !value.provider.trim()) {
    throw new Error("Invalid subscription quota provider.");
  }
  const accountId = value.accountId ?? "";
  if (typeof accountId !== "string") throw new Error("Invalid subscription quota account.");
  return {
    provider: value.provider,
    accountId,
    session: readWindow(value.session, "session"),
    weekly: readWindow(value.weekly, "weekly"),
  };
}

function readWindow(value: unknown, kind: QuotaWindowKind): QuotaWindow | undefined {
  if (value == null) return undefined;
  if (!isRecord(value)) throw new Error("Invalid subscription quota window.");
  if (value.window !== undefined && value.window !== kind) {
    throw new Error("Invalid subscription quota window.");
  }
  return {
    window: kind,
    usedPercent: finiteNumber(value.usedPercent),
    resetsAt: positiveTimestamp(value.resetsAt),
    status: typeof value.status === "string" ? value.status : undefined,
    measuredAt: positiveTimestamp(value.measuredAt) ?? 0,
  };
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function positiveTimestamp(value: unknown): number | undefined {
  const timestamp = finiteNumber(value);
  return timestamp !== undefined && timestamp > 0 ? timestamp : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
