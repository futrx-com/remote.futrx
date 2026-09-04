/**
 * Mirrors backend project.MaxSlugLen so the create-project preview matches
 * what the server will actually create.
 */
export const PROJECT_MAX_SLUG_LEN = 32;

/**
 * Every project port is published at `<slug>--<port>.dev.<public hostname>`.
 * Mirrors the URL the backend builds in project_handler.go — change both
 * together, or the browser drawer will link to hosts the router does not serve.
 */
export const PROJECT_PREVIEW_URL = {
  scheme: "https",
  /** Separates the slug from the port inside the leftmost hostname label. */
  portSeparator: "--",
  /** The label between the project host and the public hostname. */
  subdomain: "dev",
} as const;

/**
 * The ports the backend will answer a preview request for. Its TLS-ask handler
 * (project_handler.go) rejects anything outside this range, so a link to
 * 1023 or 65536 resolves to nothing — we must not offer one.
 */
export const PROJECT_PREVIEW_PORT_RANGE = { min: 1024, max: 65535 } as const;
