function formatCommandResults(results) {
  return Object.entries(results).map(([key, value]) =>
    `${key}\n${typeof value === "object" && value !== null
      ? [value.output, value.error].filter(Boolean).join("\n")
      : value}`).join("\n\n");
}

// createSession owns everything that outlives one click: the poll timer, the
// gate that stops a second mount operation from starting, and whether the
// popup is still open — a reply that arrives after it closed must not write
// into a body nobody is looking at, and must not schedule another poll.
export function createSession(api, view) {
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
