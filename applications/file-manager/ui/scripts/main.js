import { fileManagerUrl } from "./api/applicationUrls.js";
import { codeServerRunning } from "./api/projectApplicationsApi.js";
import { createRemoteFilesApi } from "./api/remoteFilesApi.js";
import { createFileUrls } from "./api/fileUrls.js";
import { ICON } from "./config/drawer.js";
import { mountFilesDrawer } from "./controllers/filesDrawerController.js";
import { syncDrawerTheme } from "./presentation/drawerTheme.js";
import { createFileOpeningService } from "./services/fileOpeningService.js";
import { browserNavigation } from "./transport/browserNavigation.js";

export default function activate(remote) {
  const dependencies = {
    remoteFiles: createRemoteFilesApi(remote),
    codeServerRunning,
    urls: createFileUrls(window.location.origin),
    navigation: browserNavigation,
  };
  const urlFor = (target) => fileManagerUrl(
    target.cwd,
    target.chatId,
    remote.application.web?.subdomain,
  );
  remote.ui.registerDrawer({
    id: "files",
    title: "Files",
    label: "Workspace files",
    icon: ICON,
    when: (target) => Boolean(target.projectId && urlFor(target)),
    mount: (body, context) => mountFilesDrawer(body, context, {
      url: urlFor(context),
      syncTheme: syncDrawerTheme,
      createFileOpener: (target, onBlocked) =>
        createFileOpeningService(dependencies, target, onBlocked),
    }),
  });
}
