/** An error response the server produced deliberately, with its status. */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/**
 * Whether the server rejected the request itself, as opposed to failing while
 * handling it. A definitive rejection repeats on every retry until something
 * else changes, so background work must stop rather than try again; a network
 * failure or 5xx may succeed next time. 401 never reaches callers — the
 * request layer reloads into the sign-in flow instead of throwing.
 */
export function isDefinitiveRejection(cause: unknown): boolean {
  return cause instanceof ApiError && cause.status >= 400 && cause.status < 500;
}
