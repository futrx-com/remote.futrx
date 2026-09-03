import type { HttpMethod } from "../types/transport";
import {
  DEFAULT_HTTP_CREDENTIALS,
  HTTP_JSON_CONTENT_TYPE,
} from "../config/transport.ts";

export async function sendHttpRequest(
  method: HttpMethod,
  url: string,
  body?: unknown,
  init?: RequestInit
): Promise<Response> {
  const headers = new Headers(init?.headers);
  let payload: BodyInit | undefined;

  if (body instanceof FormData) {
    payload = body;
  } else if (body !== undefined) {
    headers.set("Content-Type", HTTP_JSON_CONTENT_TYPE);
    payload = JSON.stringify(body);
  }

  return fetch(url, {
    ...init,
    method,
    headers,
    body: payload,
    credentials: init?.credentials ?? DEFAULT_HTTP_CREDENTIALS,
  });
}
