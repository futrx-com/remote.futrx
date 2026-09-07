import { openMountControls } from "./mountControls.js";
import { createUploadSync } from "./uploadSync.js";

export default function activate(remote) {
  const uploads = createUploadSync(remote);
  uploads.start();

  const ownInstance = context => context.instance?.imageId === remote.image.id;
  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Mount controls",
    when: ownInstance,
    onClick: context => openMountControls(remote, context, uploads),
  });
}
