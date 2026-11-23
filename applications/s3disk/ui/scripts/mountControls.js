import { describeRecent } from "./uploadSync.js";
import { mountApi } from "./mountApi.js";
import { createSession } from "./mountSession.js";

// renderBody writes the popup's markup and hands back the parts that change.
function renderBody(body) {
  body.innerHTML = `<p data-status role="status">Loading mount status…</p>
        <div data-actions></div>
        <p data-progress role="status"></p>
        <pre data-output style="white-space:pre-wrap;overflow-wrap:anywhere;max-height:55vh;overflow:auto"></pre>`;
  return {
    status: body.querySelector("[data-status]"),
    actions: body.querySelector("[data-actions]"),
    progress: body.querySelector("[data-progress]"),
    output: body.querySelector("[data-output]"),
  };
}

// What the popup offers, in the order it offers it. A mount operation is one
// the plugin runs in the background and this polls for; the rest answer here.
function mountActions(api, session, view, uploads) {
  return [
    { label: "Refresh", perform: session.refresh },
    {
      label: "Diagnostics",
      perform: async () => {
        const result = await api.diagnostics();
        if (!session.isClosed()) session.display(result);
      },
    },
    {
      label: "Attachment copies",
      perform: async () => {
        // The copies are fire-and-forget by design, so this is the only
        // place they are visible. It reads what this tab did, not the bucket.
        view.output.textContent = describeRecent(uploads.recent());
      },
    },
    {
      label: "Flush mount writes",
      mountOperation: true,
      perform: () => session.runOperation(api.startSync),
    },
    {
      label: "Restart mount",
      mountOperation: true,
      perform: () => session.runOperation(api.startRestart),
    },
  ];
}

function addActionButton(container, action, session) {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = action.label;
  button.style.margin = "0 8px 8px 0";
  if (action.mountOperation) session.gate(button);
  button.addEventListener("click", async () => {
    if (session.isBusy() && action.mountOperation) return;
    button.disabled = true;
    try {
      await action.perform();
    } catch (error) {
      if (!session.isClosed()) {
        session.showProgress(error.message);
        if (action.mountOperation) await session.poll();
      }
    } finally {
      // A mount operation's button stays disabled while the operation runs;
      // the gate owns it from here.
      if (!session.isClosed()) {
        button.disabled = Boolean(action.mountOperation) && session.isBusy();
      }
    }
  });
  container.appendChild(button);
}

export function openMountControls(remote, context, uploads) {
  const api = mountApi(remote, context.instance.id);
  remote.ui.openPopup({
    title: "S3 mount",
    width: 640,
    mount(body) {
      const view = renderBody(body);
      const session = createSession(api, view);
      for (const action of mountActions(api, session, view, uploads)) {
        addActionButton(view.actions, action, session);
      }
      session.open();
      return session.close;
    },
  });
}
