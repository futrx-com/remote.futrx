// File Manager's backend routes on the drawer's project installation. Every
// download streams through Remote's own chat download route: the backend only
// builds a folder's ZIP into the workspace and says where it is.
export async function listFiles(remote, projectId, path, signal) {
  const response = await remote.backend.fetch("list", {
    method: "GET",
    query: path ? { path } : undefined,
    signal,
    projectId,
  });
  if (!response.ok)
    throw new Error(`Unable to load files (${response.status}). Try Refresh.`);
  return response.json();
}

export function createFilesApi(remote, { projectId, chatId }) {
  return {
    list: (path, signal) => listFiles(remote, projectId, path, signal),
    async search(query, signal) {
      const response = await remote.backend.fetch("search", {
        method: "GET",
        query: { q: query },
        signal,
        projectId,
      });
      if (!response.ok)
        throw new Error(`Unable to load files (${response.status}). Try Refresh.`);
      return response.json();
    },
    downloadUrl: (path) => fileDownloadUrl(chatId, path),
    /** Builds the folder's ZIP and resolves to the URL the browser fetches it from. */
    async archive(path) {
      const response = await remote.backend.fetch("archive", {
        method: "POST",
        query: path ? { path } : undefined,
        projectId,
      });
      if (!response.ok)
        throw new Error(await backendError(response, `Unable to prepare the zip (${response.status}).`));
      const result = await response.json();
      return fileDownloadUrl(chatId, result.path);
    },
  };
}

function fileDownloadUrl(chatId, path) {
  return `/api/chats/${encodeURIComponent(chatId)}/files/download?path=${encodeURIComponent(path)}`;
}

// The backend explains a refused ZIP (too large, busy) better than a status.
async function backendError(response, fallback) {
  try {
    const body = await response.json();
    if (typeof body?.error === "string" && body.error) return body.error;
  } catch {}
  return fallback;
}
