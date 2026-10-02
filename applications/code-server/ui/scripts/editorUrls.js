// Chat cwd is the host path to the bind-mounted project workspace.
const WORKSPACE = /^\/var\/lib\/remote\/projects\/([a-z0-9][a-z0-9-]*)\/workspace(?:\/(.*))?$/;

export function workspaceIdeUrl(cwd, installed, subdomain, origin = window.location.origin) {
  const match = WORKSPACE.exec(cwd || "");
  if (!match || !installed || match[1].length > 63 ||
      match[1].endsWith("-") || match[1].includes("--")) {
    return null;
  }
  if (!/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(subdomain || "") ||
      subdomain.includes("--")) {
    return null;
  }
  if (subdomain.length + 2 + match[1].length > 63) return null;
  const base = new URL(origin);
  base.hostname = `${subdomain}--${match[1]}.${base.hostname}`;
  base.pathname = "/";
  base.searchParams.set("folder", match[2] ? `/workspace/${match[2]}` : "/workspace");
  return base.toString();
}

export function fileIdeUrl({ cwd, path, line, column }, installed, subdomain, origin = window.location.origin) {
  const url = workspaceIdeUrl(cwd, installed, subdomain, origin);
  if (!url || (path !== "/workspace" && !path.startsWith("/workspace/"))) return null;
  if (path === "/workspace") return url;
  const result = new URL(url);
  // The application page builds the editor payload on its own origin.
  result.pathname += "_static/remote-open.html";
  result.searchParams.set("file", path);
  if (line && line > 0) {
    result.searchParams.set("line", String(line));
    if (column && column > 0) result.searchParams.set("column", String(column));
  }
  return result.toString();
}
