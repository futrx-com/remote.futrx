import { fileAction } from "./fileAction.js";

// Each mounted drawer owns its latest Code Server check. A click stays
// synchronous so the browser can still allow a new editor tab.
export function createFileOpeningService(
  { remoteFiles, codeServerRunning, urls, navigation },
  context,
  onBlocked,
) {
  let codeServer = false;
  const refreshCodeServer = () => {
    if (remoteFiles.available()) return;
    void codeServerRunning(context.projectId).then((running) => {
      codeServer = running;
    });
  };
  refreshCodeServer();

  function openLegacyFile({ chatId, cwd, path }, codeServer) {
    const relative = path.slice("/workspace/".length);
    const action = fileAction(path);
    if (action === "media") {
      const url = urls.media(chatId, path);
      return navigation.openTab(url) ? "opened" : { blocked: urls.absolute(url) };
    }
    if (action === "editor") {
      // A Code Server link also lets Scratch Tab offer its "Open in IDE" button.
      const ide = codeServer ? urls.editor(cwd, path) : null;
      if (navigation.claimLink(ide ?? urls.scratch(chatId, path))) return "opened";
      if (ide) return navigation.openTab(ide) ? "opened" : { blocked: ide };
    }
    navigation.download(urls.download(chatId, relative));
    return "download";
  }

  return (path) => {
    const request = {
      chatId: context.chatId,
      projectId: context.projectId,
      cwd: context.cwd,
      path: `/workspace/${path}`,
    };
    if (!remoteFiles.available()) {
      const result = openLegacyFile(request, codeServer);
      if (result.blocked) onBlocked(path, result.blocked);
      // Pick up Code Server being installed or stopped for the next click.
      refreshCodeServer();
      return;
    }
    if (remoteFiles.open(request) === "blocked") {
      const url = remoteFiles.editorUrl(request);
      if (url) onBlocked(path, url);
    }
  };
}
