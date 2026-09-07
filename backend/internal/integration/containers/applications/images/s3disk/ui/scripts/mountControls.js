import { describeRecent } from "./uploadSync.js";

// Route names are this image's plugin contract; the popup never builds them.
function mountApi(remote, instanceId) {
  const target = { instanceId };
  return {
    status: () => remote.backend.call("status", target),
    diagnostics: () => remote.backend.call("diagnostics", target),
    operation: () => remote.backend.call("operation", target),
    startSync: () => remote.backend.call("sync", { ...target, method: "POST" }),
    startRestart: () => remote.backend.call("restart", { ...target, method: "POST" }),
  };
}

function formatCommandResults(results) {
  return Object.entries(results).map(([key, value]) =>
    `${key}\n${typeof value === "object" && value !== null
      ? [value.output, value.error].filter(Boolean).join("\n")
      : value}`).join("\n\n");
}

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

// createSession owns everything that outlives one click: the poll timer, the
// gate that stops a second mount operation from starting, and whether the
// popup is still open — a reply that arrives after it closed must not write
// into a body nobody is looking at, and must not schedule another poll.
function createSession(api, view) {
  let closed = false;
  let timer;
  // Only long-running mount operations share the busy gate.
  let busy = false;
  const operationButtons = [];

  function setBusy(value) {
    busy = value;
    for (const button of operationButtons) button.disabled = value;
  }

  function display(result) {
    view.output.textContent = formatCommandResults(result);
  }

  async function refresh() {
    const result = await api.status();
    if (closed) return;
    view.status.textContent = `${result.mounted ? "Mounted" : "Not mounted"} at ${result.mountpoint}`;
    display({ service: result.service, mountCheck: result.mountCheck, statistics: result.stats });
  }

  async function poll() {
    try {
      const job = await api.operation();
      if (closed) return;
      setBusy(job.running);
      view.progress.textContent = job.action
        ? `${job.action}: ${job.running ? "running…" : job.result.error || "completed"}`
        : "";
      if (job.running) {
        timer = setTimeout(poll, 1500);
      } else if (job.action) {
        await refresh();
        if (!closed && job.result.output) view.progress.textContent += ` — ${job.result.output}`;
      }
    } catch (error) {
      if (!closed) {
        view.progress.textContent = `Could not read operation status: ${error.message}. Reopen controls to check again.`;
      }
    }
  }

  return {
    display,
    refresh,
    poll,
    isBusy: () => busy,
    isClosed: () => closed,
    // gate marks a button as one the running operation disables.
    gate: (button) => operationButtons.push(button),
    async runOperation(start) {
      setBusy(true);
      await start();
      await poll();
    },
    showProgress(text) {
      view.progress.textContent = text;
    },
    open() {
      void refresh().then(poll).catch((error) => {
        if (!closed) view.status.textContent = `Mount controls unavailable: ${error.message}`;
      });
    },
    close() {
      closed = true;
      clearTimeout(timer);
    },
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
