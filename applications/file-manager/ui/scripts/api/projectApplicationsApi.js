import { CODE_SERVER } from "../config/files.js";

/** Whether Code Server runs in the project, from Remote's own API. */
export async function codeServerRunning(projectId, fetcher = fetch) {
  if (!projectId) return false;
  try {
    const response = await fetcher(
      `/api/projects/${encodeURIComponent(projectId)}/applications`,
      { credentials: "same-origin" },
    );
    if (!response.ok) return false;
    const instances = await response.json();
    return (
      Array.isArray(instances) &&
      instances.some(
        (instance) =>
          instance?.applicationId === CODE_SERVER.id &&
          instance.status === "running",
      )
    );
  } catch {
    return false;
  }
}
