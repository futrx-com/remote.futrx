import { workspaceLocation } from "./api/applicationUrls.js";
import { createFilesApi, listFiles } from "./api/filesApi.js";
import { codeServerRunning } from "./api/projectApplicationsApi.js";
import { createRemoteFilesApi } from "./api/remoteFilesApi.js";
import { createFileUrls } from "./api/fileUrls.js";
import { ICON } from "./config/drawer.js";
import { mountFilesDrawer } from "./controllers/filesDrawerController.js";
import {
  createFileBrowserView,
  createFilesLayout,
} from "./presentation/fileBrowserView.js";
import { whenPaneSettled } from "./presentation/paneSettle.js";
import { createFileBrowser } from "./services/fileBrowserService.js";
import { createFileOpeningService } from "./services/fileOpeningService.js";
import { createRootPrefetch, withPrefetchedRoot } from "./services/rootPrefetchService.js";
import { createFileBrowserState } from "./state/fileBrowserState.js";
import { browserNavigation } from "./transport/browserNavigation.js";

export default function activate(remote) {
  const dependencies = {
    remoteFiles: createRemoteFilesApi(remote),
    codeServerRunning,
    urls: createFileUrls(window.location.origin),
    navigation: browserNavigation,
  };
  const prefetched = createRootPrefetch((projectId) => listFiles(remote, projectId));
  // The drawer reads the project's own installation, never another project's.
  const reachable = (target) =>
    Boolean(
      target.projectId &&
        target.chatId &&
        workspaceLocation(target.cwd) &&
        remote.backend.instances.some(
          (instance) => instance.projectId === target.projectId,
        ),
    );
  remote.ui.registerDrawer({
    id: "files",
    title: "Files",
    label: "Workspace files",
    icon: ICON,
    when: (target) => {
      if (!reachable(target)) return false;
      // The toggle is showing, so the drawer may open next.
      prefetched.warm(target.projectId);
      return true;
    },
    mount: (body, context) => mountFilesDrawer(body, context, {
      api: reachable(context)
        ? withPrefetchedRoot(createFilesApi(remote, context), prefetched, context.projectId)
        : null,
      createLayout: createFilesLayout,
      createBrowser: (api, changed) =>
        createFileBrowser(api, createFileBrowserState(changed), browserNavigation.download),
      createView: createFileBrowserView,
      createFileOpener: (target, onBlocked) =>
        createFileOpeningService(dependencies, target, onBlocked),
      whenSettled: whenPaneSettled,
    }),
  });
}
