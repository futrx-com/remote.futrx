// Entry module for the MySQL image's UI extension.
//
// The default export runs once, after sign-in, with the extension API. It adds
// a "Connect" action to this image's application cards and a short connection
// cheat-sheet under the applications list. Nothing here is MySQL-specific
// machinery — any image (or plugin shipped as an image) contributes the same
// way.

import { mountConnectionPopup } from "./popup.js";

const PLUG_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" ' +
  'stroke-linecap="round" stroke-linejoin="round" width="14" height="14">' +
  '<path d="M9 2v6M15 2v6M6 8h12v4a6 6 0 0 1-12 0V8ZM12 18v4"/></svg>';

export default function activate(remote) {
  // Per-instance action. `when` keeps it on this image's cards only — the slot
  // renders for every installed application.
  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Connect",
    title: `${remote.image.name} connection details`,
    icon: PLUG_ICON,
    order: -10,
    when: (context) => context.instance?.imageId === remote.image.id,
    onClick: (context) => openConnection(remote, context.instance),
  });

  // Scope-level panel. It renders only once at least one instance exists, so
  // the section stays clean on a server with no MySQL installed.
  remote.ui.register(
    remote.slots.applicationsPanel,
    (host, context) => renderPanel(host, remote, context),
    { order: 10 }
  );
}

function openConnection(remote, instance) {
  if (!instance) return;
  remote.ui.openPopup({
    title: `${instance.name} — connection`,
    width: 460,
    mount: (body) => {
      void remote.views.load("popup").then((html) => {
        body.innerHTML = html;
        return mountConnectionPopup(body, remote, instance);
      });
    },
  });
}

async function renderPanel(host, remote, context) {
  const instances = await listInstances(context.projectId);
  const mine = instances.filter((i) => i.imageId === remote.image.id);
  if (!mine.length) return;

  const first = mine[0];
  // Project apps are also reachable inside the container on the LXD bridge, at
  // the internal port; global ones only through the mapped host port.
  const onBridge = Boolean(context.projectId);
  const html = await remote.views.load("index");
  host.innerHTML = html
    .replace("{{count}}", `${mine.length} ${mine.length === 1 ? "server" : "servers"}`)
    .replace("{{host}}", onBridge ? `${first.containerName}.lxd` : first.bindAddress)
    .replace("{{port}}", String(onBridge ? first.internalPort : first.externalPort));
}

async function listInstances(projectId) {
  const path = projectId
    ? `/api/projects/${encodeURIComponent(projectId)}/applications`
    : "/api/applications";
  const response = await fetch(path, { credentials: "same-origin" });
  if (!response.ok) return [];
  return response.json();
}
