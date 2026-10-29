import { filesApi, downloadUrl } from "./api/filesApi.js";
import { createFileBrowser } from "./services/fileBrowserService.js";
import { createFileBrowserView } from "./presentation/fileBrowserView.js";
import { mountFilesPage } from "./controllers/filesPageController.js";
import { createParentBridge } from "./transport/parentBridge.js";

mountFilesPage(
  {
    search: document.querySelector("#search"),
    clear: document.querySelector("#clear"),
    entries: document.querySelector("#entries"),
  },
  {
    createBrowser: (changed) => createFileBrowser(filesApi, changed),
    createView: createFileBrowserView,
    bridge: createParentBridge(),
    downloadUrl,
    download: (url) => window.location.assign(url),
  },
);
