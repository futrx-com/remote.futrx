export function createParentBridge() {
  const parentOrigin = document.referrer
    ? new URL(document.referrer).origin
    : null;
  const embedded = Boolean(parentOrigin) && window.parent !== window;
  return {
    embedded,
    post(message) {
      if (!embedded) return;
      window.parent.postMessage(
        { source: "remote-file-manager", ...message },
        parentOrigin,
      );
    },
    accepts(event) {
      return (
        embedded &&
        event.source === window.parent &&
        event.origin === parentOrigin &&
        event.data?.source === "remote-file-manager-host"
      );
    },
  };
}
