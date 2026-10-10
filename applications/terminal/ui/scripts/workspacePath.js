// Chat cwd is the host path to the bind-mounted project workspace. Inside the
// container, where the shell runs, the same directory is /workspace.
const WORKSPACE = /^\/var\/lib\/remote\/projects\/([a-z0-9][a-z0-9-]*)\/workspace(?:\/(.*))?$/;

/** The project slug and container path a chat's working directory names. */
export function workspaceLocation(cwd) {
  const match = WORKSPACE.exec(cwd || "");
  if (!match) return null;
  return {
    slug: match[1],
    path: match[2] ? `/workspace/${match[2]}` : "/workspace",
  };
}

/** The terminal page for one chat, on the project's application origin. */
export function terminalUrl(cwd, chatId, subdomain, origin = window.location.origin) {
  const location = workspaceLocation(cwd);
  if (!location || !chatId || location.slug.length > 63 ||
      location.slug.endsWith("-") || location.slug.includes("--")) {
    return null;
  }
  if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(subdomain || "") ||
      subdomain.includes("--")) {
    return null;
  }
  if (subdomain.length + 2 + location.slug.length > 63) return null;
  const url = new URL(origin);
  url.hostname = `${subdomain}--${location.slug}.${url.hostname}`;
  url.pathname = "/";
  url.search = "";
  url.hash = "";
  // The shell belongs to the chat, so reconnecting returns to the same one.
  url.searchParams.set("session", chatId);
  url.searchParams.set("cwd", location.path);
  return url.toString();
}
