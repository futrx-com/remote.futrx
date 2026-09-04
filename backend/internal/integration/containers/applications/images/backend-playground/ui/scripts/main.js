// Entry module for the Backend Playground fixture.
//
// ui-playground answers "what can a plugin do to the Remote UI". This one
// answers the other half: "what can a plugin do on the server, and how does
// the UI reach it". Everything it draws is a way to call `remote.backend`:
//
//   panel    live plugin health, the route table the plugin advertises, and
//            the self-test button
//   console  a popup that runs any route and shows the raw answer
//   card     the same console, from the fixture's own application card
//
// Run the self-test after changing the backend plugin contract.

import { runBackendSelfTest } from "./selftest.js";

const SERVER_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round">' +
  '<rect x="3" y="4" width="18" height="6" rx="1.5"/>' +
  '<rect x="3" y="14" width="18" height="6" rx="1.5"/>' +
  '<path d="M7 7h.01M7 17h.01"/></svg>';

export default function activate(remote) {
  remote.log(
    `activating; backend ${remote.backend.available ? "available" : "unavailable"}`,
    remote.backend.instances,
  );

  // The chrome slots that carry a per-surface context are where a backend call
  // most often needs to know which install it is talking to, so the console
  // opens from there and passes the surface's project through.
  for (const slot of [remote.slots.chatHeaderActions, remote.slots.composerActions]) {
    remote.ui.addIconButton(slot, {
      icon: SERVER_ICON,
      label: "Backend Playground: open the plugin console",
      title: "Backend Playground — call the Go plugin",
      order: -98,
      onClick: (context) => openConsole(remote, context),
    });
  }

  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Console",
    title: "Call this image's Go plugin",
    variant: "solid",
    order: -10,
    when: (context) => context.instance?.imageId === remote.image.id,
    onClick: (context) => openConsole(remote, context),
  });

  remote.ui.register(
    remote.slots.applicationsPanel,
    (host, context) => renderPanel(remote, host, context),
    { order: 21 },
  );
}

// ---- the panel --------------------------------------------------------------

function renderPanel(remote, host, context) {
  let cancelled = false;
  let timer = null;

  void remote.views.load("panel").then((html) => {
    if (cancelled) return;
    host.innerHTML = html;

    const logo = host.querySelector('[data-field="logo"]');
    if (logo) logo.src = remote.assets.url("assets/logo.svg");

    host
      .querySelector('[data-action="console"]')
      ?.addEventListener("click", () => openConsole(remote, context));
    host
      .querySelector('[data-action="selftest"]')
      ?.addEventListener("click", () => showSelfTest(remote, context));

    const refresh = () => {
      if (cancelled) return;
      void refreshPanel(remote, host, context);
    };
    refresh();
    // Polling health is the visible proof that the plugin is a process: the
    // uptime climbs, and it resets to zero when the app is stopped and started.
    timer = setInterval(refresh, 5000);
  });

  return () => {
    cancelled = true;
    if (timer) clearInterval(timer);
  };
}

async function refreshPanel(remote, host, context) {
  const status = host.querySelector('[data-field="status"]');
  const detail = host.querySelector('[data-field="detail"]');
  const routes = host.querySelector('[data-field="routes"]');
  if (!remote.backend.available) {
    setStatus(status, false, "no running backend");
    if (detail) detail.textContent = "Install and start this image to run its plugin.";
    return;
  }

  const target = { projectId: context.projectId };
  try {
    const health = await remote.backend.call("health", target);
    setStatus(status, true, `pid ${health.pid}`);
    if (detail) {
      detail.textContent =
        `${health.goVersion} on ${health.os}/${health.arch} · ` +
        `up ${health.uptimeSeconds}s · ${health.requests} requests`;
    }
  } catch (error) {
    setStatus(status, false, "unreachable");
    if (detail) detail.textContent = String(error.message ?? error);
    return;
  }

  if (!routes || routes.dataset.filled === "yes") return;
  try {
    const described = await remote.backend.describe(target);
    routes.replaceChildren();
    for (const route of described.descriptor.routes ?? []) {
      const item = document.createElement("li");
      item.innerHTML =
        `<span class="bp-method">${escapeHtml(route.method)}</span>` +
        `<span class="bp-route">${escapeHtml(route.path)}</span>` +
        `<span class="bp-note">${escapeHtml(route.description ?? "")}</span>`;
      routes.appendChild(item);
    }
    routes.dataset.filled = "yes";
  } catch (error) {
    remote.log("describe failed", error);
  }
}

function setStatus(element, ok, text) {
  if (!element) return;
  element.dataset.ok = String(ok);
  element.textContent = text;
}

// ---- the console ------------------------------------------------------------

// Each entry is one call, spelled out so the popup doubles as a worked example
// of the `remote.backend.call` options.
//
// The failing routes are a separate group rather than a button among the
// others. They are the point of the fixture — a panic and a timeout are how
// you see that neither takes the process down — but mixed in with the working
// ones they are just an alarming red result from a button someone clicked
// while trying everything, and nothing on screen says it was meant to happen.
function consoleGroups() {
  return [
    {
      title: "Routes",
      calls: [
        { label: "health", path: "health" },
        { label: "instance", path: "instance" },
        { label: "echo", path: "echo", options: { method: "POST", body: { hello: "world" }, query: { from: "console" } } },
        { label: "kv write", path: "kv/greeting", options: { method: "POST", body: { value: "hello from the browser" } } },
        { label: "kv read", path: "kv/greeting" },
        { label: "kv list", path: "kv" },
        { label: "note write", path: "notes", options: { method: "POST", body: { note: "written at " + new Date().toISOString() } } },
        { label: "note read", path: "notes" },
        { label: "compute", path: "compute", options: { method: "POST", body: { n: 30 } } },
        { label: "slow 200ms", path: "slow", options: { query: { ms: 200 } } },
        { label: "admin only", path: "admin", note: "403 unless you are an administrator" },
      ],
    },
    {
      title: "Meant to fail — the process survives all three",
      expected: true,
      calls: [
        { label: "panic", path: "boom", note: "the route panics; health keeps the same pid" },
        { label: "timeout (11s)", path: "slow", options: { query: { ms: 11000 } }, note: "exceeds the image's 10s timeoutMs" },
        { label: "unknown route", path: "no/such/route", note: "404 from the plugin's mux" },
      ],
    },
  ];
}

function openConsole(remote, context) {
  remote.ui.openPopup({
    title: "Backend Playground — plugin console",
    width: 620,
    mount: (body) => {
      void remote.views.load("console").then((html) => {
        body.innerHTML = html;
        wireConsole(remote, body, context);
      });
    },
  });
}

function wireConsole(remote, body, context) {
  const log = body.querySelector('[data-field="log"]');
  const target = body.querySelector('[data-field="target"]');
  const actions = body.querySelector('[data-field="actions"]');

  if (target) {
    target.textContent = remote.backend.available
      ? describeTarget(remote, context)
      : "no running backend — install and start this image first";
  }

  for (const group of consoleGroups()) {
    const heading = document.createElement("p");
    heading.className = "bp-group";
    heading.dataset.expected = String(Boolean(group.expected));
    heading.textContent = group.title;
    actions?.appendChild(heading);

    const row = document.createElement("div");
    row.className = "bp-actions bp-actions-wrap";
    for (const call of group.calls) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = group.expected ? "bp-call bp-call-expected" : "bp-call";
      button.textContent = call.label;
      if (call.note) button.title = call.note;
      button.addEventListener("click", () =>
        void run({ ...call, expected: Boolean(group.expected) }, button));
      row.appendChild(button);
    }
    actions?.appendChild(row);
  }

  body
    .querySelector('[data-action="selftest"]')
    ?.addEventListener("click", () => showSelfTest(remote, context));
  body
    .querySelector('[data-action="clear"]')
    ?.addEventListener("click", () => log?.replaceChildren());

  async function run(call, button) {
    // Only this button is disabled, so several calls can be in flight at once
    // — which is the point: one plugin process serves them concurrently. That
    // is exactly why each result has to say which call it belongs to, and why
    // they append rather than overwrite a shared pane. A shared pane would
    // show whichever call finished last and name none of them.
    button.disabled = true;
    const label = `${(call.options?.method ?? "GET").toUpperCase()} ${call.path}`;
    const started = Date.now();
    try {
      const result = await remote.backend.call(call.path, {
        ...call.options,
        projectId: context.projectId,
      });
      record(label, true, false, Date.now() - started, JSON.stringify(result, null, 2));
    } catch (error) {
      // A failed call is a result too. Whether it is alarming depends entirely
      // on which button produced it, which is why that travels with the call.
      record(label, false, call.expected, Date.now() - started,
        String(error.message ?? error));
    } finally {
      button.disabled = false;
    }
  }

  // Newest first, and capped: the log is for watching a handful of calls, not
  // for accumulating a session's worth of them.
  function record(label, ok, expected, elapsedMs, detail) {
    if (!log) return;
    const entry = document.createElement("div");
    entry.className = "bp-entry";
    entry.dataset.ok = String(ok);
    entry.dataset.expected = String(expected);
    entry.innerHTML =
      `<div class="bp-entry-head">` +
      `<span class="bp-mark">${ok ? "✓" : expected ? "!" : "✕"}</span>` +
      `<span class="bp-entry-label">${escapeHtml(label)}</span>` +
      (expected ? `<span class="bp-entry-tag">expected</span>` : "") +
      `<span class="bp-entry-time">${elapsedMs}ms</span>` +
      `</div><pre></pre>`;
    entry.querySelector("pre").textContent = detail;
    log.prepend(entry);
    while (log.childElementCount > 20) log.lastElementChild.remove();
  }
}

function describeTarget(remote, context) {
  const instances = remote.backend.instances;
  const inProject = context.projectId
    && instances.find((candidate) => candidate.projectId === context.projectId);
  const chosen = inProject
    ?? instances.find((candidate) => candidate.scope === "global")
    ?? instances[0];
  const where = chosen.scope === "project" ? `project ${chosen.projectId}` : "global";
  return `${instances.length} running · calling ${chosen.instanceId} (${where})`;
}

// ---- the self-test ----------------------------------------------------------

function showSelfTest(remote, context) {
  remote.ui.openPopup({
    title: "Backend API self-test",
    width: 560,
    mount: (body) => {
      void remote.views.load("console").then(async (html) => {
        body.innerHTML = html;
        const section = body.querySelector('[data-section="selftest"]');
        if (section) section.hidden = false;
        body.querySelector('[data-section="console"]')?.setAttribute("hidden", "");

        const list = body.querySelector('[data-field="results"]');
        const summary = body.querySelector('[data-field="summary"]');
        if (summary) summary.textContent = "running…";

        const results = await runBackendSelfTest(remote, context);
        for (const result of results) {
          const item = document.createElement("li");
          item.dataset.ok = String(result.ok);
          item.innerHTML =
            `<span class="bp-mark">${result.ok ? "✓" : "✕"}</span>` +
            `<span>${escapeHtml(result.name)}` +
            (result.detail ? `<span class="bp-detail">${escapeHtml(result.detail)}</span>` : "") +
            `</span>`;
          list?.appendChild(item);
        }
        const passed = results.filter((result) => result.ok).length;
        if (summary) summary.textContent = `${passed} of ${results.length} checks passed.`;
        remote.log("backend self-test", `${passed}/${results.length}`);
      });
    },
  });
}

function escapeHtml(value) {
  return String(value).replace(
    /[&<>"']/g,
    (char) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char],
  );
}
