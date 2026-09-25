// Entry module for the Hello Remote example.
//
// The default export runs once, after sign-in, with the extension API. It adds
// controls across every extension slot plus the applications panel. Together
// they expose the complete frontend API and call this application's own Go
// backend. That round trip — browser to the backend/api entry point the server
// compiled with its sibling backend/lifecycle package — is the center of the
// example.

import { mountContainerPanel } from "./containerPanel.js";
import { mountServicePanel } from "./servicePanel.js";
import { activateFrontendShowcase } from "./showcase.js";

const WAVE_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" ' +
  'stroke-linecap="round" stroke-linejoin="round" width="14" height="14">' +
  '<path d="M12 3v9M8.5 6.5v7M15.5 6.5v7M5 10v3.5a7 7 0 0 0 14 0V10"/></svg>';

export default function activate(remote) {
  activateFrontendShowcase(remote);

  // Per-instance action. The slot renders for every installed application, so
  // `when` is what keeps this button on this application's cards.
  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Say hello",
    title: "Call this install's Go backend",
    icon: WAVE_ICON,
    variant: "solid",
    order: -10,
    when: (context) => context.instance?.applicationId === remote.application.id,
    onClick: (context) => sayHello(remote, context),
  });

  // Scope-level panel, below the applications list. It renders under both
  // scopes; `context.projectId` is what tells a project surface from the
  // global one, and it is also what picks which install a call reaches.
  remote.ui.register(
    remote.slots.applicationsPanel,
    (host, context) => renderPanel(host, remote, context),
    { order: 10 }
  );
}

// A call resolves to one running install: the project's when `projectId` is
// given, the global one otherwise. Passing the slot's context is what makes an
// extension follow the surface the user is actually on.
function target(context) {
  return { projectId: context.projectId };
}

function sayHello(remote, context) {
  remote.ui.openPopup({
    title: "Hello Remote",
    width: 420,
    mount: (body) => {
      body.textContent = "Calling the backend…";
      remote.backend
        .call("hello", target(context))
        .then((reply) => {
          body.innerHTML = "";
          const line = document.createElement("p");
          line.className = "hello-remote__reply";
          line.textContent = reply.message;
          const note = document.createElement("p");
          note.className = "hello-remote__note";
          note.textContent =
            `${reply.scope} install · answered by a process the server built from backend/`;
          body.append(line, note);
        })
        .catch((error) => {
          body.textContent = `The backend did not answer: ${error.message}`;
        });
    },
  });
}

// The slot host calls render synchronously and keeps whatever it returns as
// the cleanup for that contribution — so a render function that is itself
// `async` returns a promise, not a disposer, and has nothing to unwind with.
// This one stays synchronous and does its awaiting inside.
function renderPanel(host, remote, context) {
  // available is false when no install of this application is running — including on
  // a surface where it was never installed. Degrading here is what keeps the
  // panel quiet instead of throwing on every render.
  if (!remote.backend.available) return;

  let disposed = false;
  let detach = () => {};

  void remote.views.load("panel").then((html) => {
    if (disposed) return;
    host.innerHTML = html;

    const message = host.querySelector("[data-hello-message]");
    const count = host.querySelector("[data-hello-count]");
    const button = host.querySelector("[data-hello-greet]");

    const show = (reply) => {
      if (disposed) return;
      if (reply.warning) {
        message.textContent = `Greeting counted with a warning: ${reply.warning}`;
      } else if (reply.message) {
        message.textContent = reply.message;
      }
      if (typeof reply.visits === "number") count.textContent = String(reply.visits);
    };
    const fail = (error) => {
      if (disposed) return;
      message.textContent = `The backend did not answer: ${error.message}`;
    };

    const onClick = () => {
      button.disabled = true;
      remote.backend
        .call("visits", { ...target(context), method: "POST" })
        .then(show)
        .catch(fail)
        .finally(() => {
          if (!disposed) button.disabled = false;
        });
    };

    button.addEventListener("click", onClick);
    const unmountContainerPanel = mountContainerPanel(
      host,
      remote.backend,
      target(context),
      () => disposed
    );
    const unmountServicePanel = mountServicePanel(
      host,
      remote.backend,
      target(context),
      () => disposed
    );
    detach = () => {
      button.removeEventListener("click", onClick);
      unmountContainerPanel();
      unmountServicePanel();
    };

    remote.backend.call("hello", target(context)).then(show).catch(fail);
  });

  // Anything a render function starts is its own to stop when the surface
  // unmounts. The flag matters as much as the listener: a call still in flight
  // must not write into a node the host has already emptied.
  return () => {
    disposed = true;
    detach();
  };
}
