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

export function formatCommandResults(results) {
  return Object.entries(results).map(([key, value]) =>
    `${key}\n${typeof value === "object" && value !== null
      ? [value.output, value.error].filter(Boolean).join("\n")
      : value}`).join("\n\n");
}

export function openMountControls(remote, context, uploads) {
  const api = mountApi(remote, context.instance.id);
  remote.ui.openPopup({
    title: "S3 mount",
    width: 640,
    mount(body) {
      let closed = false;
      let timer;
      body.innerHTML = `<p data-status role="status">Loading mount status…</p>
        <div data-actions></div>
        <p data-progress role="status"></p>
        <pre data-output style="white-space:pre-wrap;overflow-wrap:anywhere;max-height:55vh;overflow:auto"></pre>`;
      const status = body.querySelector("[data-status]");
      const progress = body.querySelector("[data-progress]");
      const output = body.querySelector("[data-output]");
      // Only long-running mount operations share the busy gate.
      const operationButtons = [];
      let busy = false;
      function setBusy(value) {
        busy = value;
        for (const button of operationButtons) button.disabled = value;
      }
      function display(result) {
        output.textContent = formatCommandResults(result);
      }
      async function refresh() {
        const result = await api.status();
        if (closed) return;
        status.textContent = `${result.mounted ? "Mounted" : "Not mounted"} at ${result.mountpoint}`;
        display({ service: result.service, mountCheck: result.mountCheck, statistics: result.stats });
      }
      async function poll() {
        try {
          const job = await api.operation();
          if (closed) return;
          setBusy(job.running);
          progress.textContent = job.action
            ? `${job.action}: ${job.running ? "running…" : job.result.error || "completed"}`
            : "";
          if (job.running) {
            timer = setTimeout(poll, 1500);
          } else if (job.action) {
            await refresh();
            if (!closed && job.result.output) progress.textContent += ` — ${job.result.output}`;
          }
        } catch (error) {
          if (!closed) {
            progress.textContent = `Could not read operation status: ${error.message}. Reopen controls to check again.`;
          }
        }
      }
      async function runMountOperation(start) {
        setBusy(true);
        await start();
        await poll();
      }
      const actions = [
        { label: "Refresh", perform: refresh },
        { label: "Diagnostics", perform: async () => {
          const result = await api.diagnostics();
          if (!closed) display(result);
        } },
        { label: "Attachment copies", perform: async () => {
          // The copies are fire-and-forget by design, so this is the only
          // place they are visible. It reads what this tab did, not the bucket.
          output.textContent = describeRecent(uploads ? uploads.recent() : []);
        } },
        { label: "Flush mount writes", mountOperation: true, perform: () => runMountOperation(api.startSync) },
        { label: "Restart mount", mountOperation: true, perform: () => runMountOperation(api.startRestart) },
      ];
      for (const action of actions) {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = action.label;
        button.style.margin = "0 8px 8px 0";
        if (action.mountOperation) operationButtons.push(button);
        button.addEventListener("click", async () => {
          if (busy && action.mountOperation) return;
          button.disabled = true;
          try {
            await action.perform();
          } catch (error) {
            if (!closed) {
              progress.textContent = error.message;
              if (action.mountOperation) await poll();
            }
          } finally {
            if (!closed) button.disabled = Boolean(action.mountOperation) && busy;
          }
        });
        body.querySelector("[data-actions]").appendChild(button);
      }
      void refresh().then(poll).catch((error) => {
        if (!closed) status.textContent = `Mount controls unavailable: ${error.message}`;
      });
      return () => { closed = true; clearTimeout(timer); };
    },
  });
}
