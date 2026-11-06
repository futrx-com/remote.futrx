// File Manager's backend routes on the drawer's project installation. Single
// files download through Remote's own chat download route, which streams them;
// only folder ZIPs are built by the backend.
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
    downloadUrl(node) {
      const path = encodeURIComponent(node.path);
      if (node.isDir)
        return `${remote.backend.url("archive", { projectId })}?path=${path}`;
      return `/api/chats/${encodeURIComponent(chatId)}/files/download?path=${path}`;
    },
  };
}
