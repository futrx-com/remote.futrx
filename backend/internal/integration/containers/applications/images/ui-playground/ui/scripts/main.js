// Entry module for the UI Playground fixture.
//
// It contributes to every slot the extension API advertises, using each of the
// three ways to contribute:
//
//   addIconButton  — one flask in each chrome slot, showing that slot's context
//   addButton      — a labelled action on this image's own application cards
//   register       — a panel that renders markup and cleans up after itself
//
// Nothing here is specific to what the container runs. This is the reference
// for "what can a plugin do to the Remote UI", and the thing to run after
// changing the extension API.

import { runSelfTest } from "./selftest.js";

const FLASK_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="M9 3h6M10 3v6.5L5.2 17.6A2 2 0 0 0 6.9 20.6h10.2a2 2 0 0 0 1.7-3L14 9.5V3"/>' +
  '<path d="M7.5 15h9"/></svg>';

export default function activate(remote) {
  remote.log(`activating against extension API v${remote.apiVersion}`);

  // --- every chrome slot gets an icon ------------------------------------
  // The slot decides the size, so the same markup fits the chat header rail
  // and the sidebar search field alike.
  for (const slot of chromeSlots(remote)) {
    remote.ui.addIconButton(slot, {
      icon: FLASK_ICON,
      label: `UI Playground: inspect ${slot}`,
      title: "UI Playground — show this slot's context",
      order: -100,
      onClick: (context) => showContext(remote, context),
    });
  }

  // --- a labelled action, on this image's cards only ----------------------
  remote.ui.addButton(remote.slots.applicationCardActions, {
    label: "Self-test",
    title: "Run the extension API self-test",
    variant: "solid",
    order: -10,
    when: (context) => context.instance?.imageId === remote.image.id,
    onClick: () => showSelfTest(remote),
  });

  // --- a panel that owns its own markup and teardown ----------------------
  remote.ui.register(remote.slots.applicationsPanel, (host, context) =>
    renderPanel(remote, host, context)
  , { order: 20 });
}

/** Every slot that takes an icon, in the order they appear down the app. */
function chromeSlots(remote) {
  return [
    remote.slots.sidebarHeaderActions,
    remote.slots.sidebarSearchActions,
    remote.slots.projectRowActions,
    remote.slots.chatHeaderActions,
    remote.slots.composerActions,
  ];
}

// ---- popups ----------------------------------------------------------------

function showContext(remote, context) {
  openPlaygroundPopup(remote, "Slot context", "context", (body) => {
    const target = body.querySelector('[data-field="context"]');
    if (target) target.textContent = JSON.stringify(context, null, 2);
  });
}

function showSelfTest(remote) {
  openPlaygroundPopup(remote, "Extension API self-test", "selftest", async (body) => {
    const list = body.querySelector('[data-field="results"]');
    const summary = body.querySelector('[data-field="summary"]');
    const results = await runSelfTest(remote);

    for (const result of results) {
      const item = document.createElement("li");
      item.dataset.ok = String(result.ok);
      item.innerHTML =
        `<span class="pg-mark">${result.ok ? "✓" : "✕"}</span>` +
        `<span>${escapeHtml(result.name)}` +
        (result.detail ? `<span class="pg-detail">${escapeHtml(result.detail)}</span>` : "") +
        `</span>`;
      list?.appendChild(item);
    }

    const passed = results.filter((r) => r.ok).length;
    if (summary) {
      summary.textContent = `${passed} of ${results.length} checks passed.`;
    }
    remote.log("self-test", `${passed}/${results.length}`);
  });
}

// Both popups share views/context.html and reveal one of its sections, so the
// fixture also demonstrates reusing a single view for more than one purpose.
function openPlaygroundPopup(remote, title, section, fill) {
  remote.ui.openPopup({
    title,
    width: 520,
    mount: (body) => {
      void remote.views.load("context").then((html) => {
        body.innerHTML = html;
        const target = body.querySelector(`[data-section="${section}"]`);
        if (target) target.hidden = false;
        return fill(body);
      });
    },
  });
}

// ---- panel -----------------------------------------------------------------

function renderPanel(remote, host, context) {
  // A ticking clock is the visible proof that cleanup runs: leave the
  // Applications tab and the interval stops rather than leaking.
  let timer = null;
  let cancelled = false;

  void remote.views.load("panel").then((html) => {
    if (cancelled) return;
    host.innerHTML = html;

    const logo = host.querySelector('[data-field="logo"]');
    if (logo) logo.src = remote.assets.url("assets/logo.svg");

    const badge = host.querySelector('[data-field="apiVersion"]');
    if (badge) badge.textContent = `api v${remote.apiVersion}`;

    const slots = host.querySelector('[data-field="slots"]');
    for (const [key, name] of Object.entries(remote.slots)) {
      const item = document.createElement("li");
      item.innerHTML = `<span>${escapeHtml(key)}</span><span>${escapeHtml(name)}</span>`;
      slots?.appendChild(item);
    }

    host
      .querySelector('[data-action="selftest"]')
      ?.addEventListener("click", () => showSelfTest(remote));

    const clock = host.querySelector('[data-field="clock"]');
    if (clock) {
      const started = Date.now();
      const tick = () => {
        clock.textContent = `mounted ${Math.round((Date.now() - started) / 1000)}s`;
      };
      tick();
      timer = setInterval(tick, 1000);
    }

    remote.log("panel mounted", context.scope, context.projectId ?? "(global)");
  });

  return () => {
    cancelled = true;
    if (timer) clearInterval(timer);
    remote.log("panel cleaned up");
  };
}

function escapeHtml(value) {
  return String(value).replace(
    /[&<>"']/g,
    (char) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]
  );
}
