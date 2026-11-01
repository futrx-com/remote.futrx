// Adapt the optional Remote file API without leaking the extension object.
export function createRemoteFilesApi(remote) {
  return {
    available: () => Boolean(remote.files?.open),
    open: (request) => remote.files.open(request),
    editorUrl: (request) => remote.files.editorUrl?.(request),
  };
}
