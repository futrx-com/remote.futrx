// Entry module for the UI Sandbox fixture.
//
// A second extension that contributes to the same slots as UI Playground, so
// the two can be run together to check that they share those surfaces. Install
// them at *different* scopes — one global, one in a project — and this one also
// demonstrates the scoping rule: a project-installed extension appears only
// while you are in that project.
//
// It orders its contributions after UI Playground's so the pair renders in a
// stable, readable order rather than by whichever loaded first.

const CUBE_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="M12 2.5 21 7v10l-9 4.5L3 17V7l9-4.5Z"/>' +
  '<path d="M3 7l9 4.5L21 7M12 11.5V21"/></svg>';

export default function activate(remote) {
  remote.log("activating", describeInstall(remote));

  for (const slot of [
    remote.slots.sidebarHeaderActions,
    remote.slots.sidebarSearchActions,
    remote.slots.projectRowActions,
    remote.slots.chatHeaderActions,
    remote.slots.composerActions,
  ]) {
    remote.ui.addIconButton(slot, {
      icon: CUBE_ICON,
      label: `UI Sandbox: inspect ${slot}`,
      title: "UI Sandbox — where am I visible, and why?",
      // UI Playground uses -100; sitting just after it keeps the pair ordered
      // the same way in every slot.
      order: -99,
      onClick: (context) => showContext(remote, context),
    });
  }

  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Scope",
    title: "Where this extension is visible",
    order: -9,
    when: (context) => context.instance?.imageId === remote.image.id,
    onClick: (context) => showContext(remote, context),
  });

  remote.ui.register(
    remote.slots.applicationsPanel,
    (host, context) => renderPanel(remote, host, context),
    { order: 21 }
  );
}

/** Plain-English summary of where this copy was installed. */
function describeInstall(remote) {
  const { global, projectIds } = remote.install;
  if (global) return "globally — visible everywhere";
  if (!projectIds.length) return "nowhere (no install scope reported)";
  return `in ${projectIds.length} project${projectIds.length === 1 ? "" : "s"} — visible only there`;
}

function showContext(remote, context) {
  remote.ui.openPopup({
    title: "UI Sandbox",
    width: 520,
    mount: (body) => {
      void remote.views.load("context").then((html) => {
        body.innerHTML = html;
        set(body, "image", `${remote.image.name} (${remote.image.id})`);
        set(body, "install", describeInstall(remote));
        set(body, "why", whyVisible(remote, context));
        set(body, "context", JSON.stringify(context, null, 2));
      });
    },
  });
}

// The registry decides visibility; this only narrates the decision, which is
// the fastest way to see whether the scoping rule did what you expected.
function whyVisible(remote, context) {
  if (remote.install.global) return "installed globally, so every surface";
  const project = context.projectId;
  if (project) return `this surface belongs to project ${project}`;
  return "the project you are currently in has it installed";
}

function renderPanel(remote, host, context) {
  let cancelled = false;
  void remote.views.load("panel").then((html) => {
    if (cancelled) return;
    host.innerHTML = html;

    const logo = host.querySelector('[data-field="logo"]');
    if (logo) logo.src = remote.assets.url("assets/logo.svg");

    set(host, "scope", remote.install.global ? "global" : "project");
    set(
      host,
      "explain",
      `Installed ${describeInstall(remote)}. ` +
        (context.scope === "global"
          ? "You are looking at the server-wide Applications page."
          : "You are looking at this project's applications.")
    );
  });
  return () => {
    cancelled = true;
  };
}

function set(root, field, value) {
  const node = root.querySelector(`[data-field="${field}"]`);
  if (node) node.textContent = value;
}
