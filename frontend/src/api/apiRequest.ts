import { sendHttpRequest } from "../transport/http";
import type { HttpMethod } from "../types/transport";
import { API_RESPONSE_STATUS } from "../config/api";
import { ApiError } from "./apiError.ts";

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
    throw new ApiError(msg, response.status);
  }
  if (response.status === API_RESPONSE_STATUS.noContent) return undefined as T;
  return response.json() as Promise<T>;
}
