import { sendHttpRequest } from "../transport/http";
import type { HttpMethod } from "../types/transport";
import { API_RESPONSE_STATUS } from "../config/api";

/**
 * An HTTP failure that keeps its status code. Callers that must distinguish
 * "refused because the host is full" (409) from an ordinary failure read
 * `status`; everyone else keeps treating it as a plain Error.
 */
export interface ApiError extends Error {
  status: number;
}

export async function requestJson<T>(
  method: HttpMethod,
  url: string,
  body?: unknown
): Promise<T> {
  const response = await sendHttpRequest(method, url, body);
  if (response.status === API_RESPONSE_STATUS.unauthorized) {
    location.reload();
    return new Promise<T>(() => {});
  }
  if (!response.ok) {
    let msg = `${response.status}`;
    try {
      msg = (await response.json()).error || msg;
    } catch {}
    const error = new Error(msg) as ApiError;
    error.status = response.status;
    throw error;
  }
  if (response.status === API_RESPONSE_STATUS.noContent) return undefined as T;
  return response.json() as Promise<T>;
}
