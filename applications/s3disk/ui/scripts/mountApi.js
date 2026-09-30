// Route names are this application's backend contract; the popup never builds them.
export function mountApi(remote, instanceId) {
  const target = { instanceId };
  return {
    status: () => remote.backend.call("status", target),
    diagnostics: () => remote.backend.call("diagnostics", target),
    operation: () => remote.backend.call("operation", target),
    startSync: () => remote.backend.call("sync", { ...target, method: "POST" }),
    startRestart: () => remote.backend.call("restart", { ...target, method: "POST" }),
  };
}
