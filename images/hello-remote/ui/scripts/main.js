// Entry module for the Hello Remote example.
//
// The default export runs once, after sign-in, with the extension API. It adds
// one card action and one panel, and both of them do the same thing: call this
// image's own Go plugin and show what came back. That round trip — browser to
// a process the server compiled out of plugin/ — is the whole example.

const WAVE_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" ' +
  'stroke-linecap="round" stroke-linejoin="round" width="14" height="14">' +
  '<path d="M12 3v9M8.5 6.5v7M15.5 6.5v7M5 10v3.5a7 7 0 0 0 14 0V10"/></svg>';

export default function activate(remote) {
  // Per-instance action. The slot renders for every installed application, so
  // `when` is what keeps this button on this image's cards.
  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Say hello",
    title: "Call this install's Go plugin",
    icon: WAVE_ICON,
    order: -10,
    when: (context) => context.instance?.imageId === remote.image.id,
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
      body.textContent = "Calling the plugin…";
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
            `${reply.scope} install · answered by a process the server built from plugin/`;
          body.append(line, note);
        })
        .catch((error) => {
          body.textContent = `The plugin did not answer: ${error.message}`;
        });
    },
  });
}

// The slot host calls render synchronously and keeps whatever it returns as
// the cleanup for that contribution — so a render function that is itself
// `async` returns a promise, not a disposer, and has nothing to unwind with.
// This one stays synchronous and does its awaiting inside.
function renderPanel(host, remote, context) {
  // available is false when no install of this image is running — including on
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
      if (reply.message) message.textContent = reply.message;
      if (typeof reply.visits === "number") count.textContent = String(reply.visits);
    };
    const fail = (error) => {
      if (disposed) return;
      message.textContent = `The plugin did not answer: ${error.message}`;
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
    detach = () => button.removeEventListener("click", onClick);

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
