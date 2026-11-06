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
