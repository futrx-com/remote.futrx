async function get(route, signal) {
  const response = await fetch(route, { signal });
  if (!response.ok)
    throw new Error(`Unable to load files (${response.status}). Try Refresh.`);
  return response.json();
}

export const filesApi = {
  list(path, signal) {
    return get(
      path ? `/api/list?path=${encodeURIComponent(path)}` : "/api/list",
      signal,
    );
  },
  search(query, signal) {
    return get(`/api/search?q=${encodeURIComponent(query)}`, signal);
  },
};

export function downloadUrl(node) {
  return `/api/${node.isDir ? "archive" : "download"}?path=${encodeURIComponent(node.path)}`;
}
