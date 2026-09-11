// The `remote.backend` half of the extension API: the client an image's `ui/`
// uses to call the Go plugin the same image ships in its `plugin/` directory.
//
// An image can be installed in more than one place, and each install runs its
// own plugin process, so every call has to resolve to an instance before it
// has a URL. That resolution is the only real logic here; the rest is a thin,
// same-origin `fetch` that reports a plugin's own error message rather than a
// status code.

import type {
  AppBackendDescriptor,
  AppBackendInstance,
  AppImage,
} from "../../models/application.ts";
import type {
  ExtensionBackendApi,
  ExtensionBackendCallOptions,
  ExtensionBackendTarget,
} from "../../models/extension.ts";
import { ExtensionBackendTargets } from "./extensionBackendTarget.ts";

export function createBackendApi(
  image: AppImage,
  backends: AppBackendInstance[],
): ExtensionBackendApi {
  const targets = new ExtensionBackendTargets(image.id, backends);

  const request = async (
    path: string,
    options: ExtensionBackendCallOptions = {},
  ): Promise<Response> => {
    const { method, body, query, headers, signal, ...target } = options;
    const hasBody = body !== undefined && body !== null;
    const init: RequestInit = {
      method: method ?? (hasBody ? "POST" : "GET"),
      credentials: "same-origin",
      headers: { ...headers },
      signal,
    };
    if (hasBody) {
      if (typeof body === "string") {
        init.body = body;
      } else {
        init.body = JSON.stringify(body);
        init.headers = { "Content-Type": "application/json", ...init.headers };
      }
    }
    return fetch(targets.url(path, target) + queryString(query), init);
  };

  return {
    available: Boolean(image.backend) && targets.instances.length > 0,
    instances: targets.instances,
    url: (path, target) => targets.url(path, target),
    fetch: request,
    call: async <T,>(path: string, options?: ExtensionBackendCallOptions) => {
      const response = await request(path, options);
      return readSuccessfulBody<T>(response);
    },
    describe: async (target?: ExtensionBackendTarget) => {
      const response = await request("", target);
      return readSuccessfulBody<AppBackendDescriptor>(response);
    },
  };
}

function queryString(
  query?: Record<string, string | number | boolean | undefined>,
): string {
  if (!query) return "";
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined) continue;
    params.set(key, String(value));
  }
  const encoded = params.toString();
  return encoded ? `?${encoded}` : "";
}

/**
 * Plugins are free to answer with anything, so a body is read as JSON when it
 * says it is one and as text otherwise. A plugin's error text is far more use
 * than "500", which is why it survives all the way to the thrown Error.
 */
async function readBody(response: Response): Promise<unknown> {
  const contentType = response.headers.get("Content-Type") ?? "";
  if (contentType.includes("json")) {
    try {
      return await response.json();
    } catch {
      return null;
    }
  }
  return response.text();
}

async function readSuccessfulBody<T>(response: Response): Promise<T> {
  const payload = await readBody(response);
  if (!response.ok) {
    throw new Error(errorMessage(payload, response.status));
  }
  return payload as T;
}

function errorMessage(payload: unknown, status: number): string {
  if (typeof payload === "string" && payload.trim()) return payload;
  if (payload && typeof payload === "object" && "error" in payload) {
    const message = (payload as { error?: unknown }).error;
    if (typeof message === "string" && message) return message;
  }
  return `backend call failed: ${status}`;
}
