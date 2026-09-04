// Assertions against the extension API, run from the playground panel.
//
// These are the checks a plugin author most wants answered before writing real
// code: does this slot exist, does that URL resolve, does a disposer actually
// dispose. Kept in its own module so the entry stays about contributions, and
// so relative imports inside `ui/scripts/` are exercised too.

/**
 * @param {object} remote  the extension API
 * @returns {Promise<Array<{name: string, ok: boolean, detail: string}>>}
 */
export async function runSelfTest(remote) {
  const results = [];
  const check = async (name, fn) => {
    try {
      const detail = await fn();
      results.push({ name, ok: true, detail: detail ?? "" });
    } catch (error) {
      results.push({ name, ok: false, detail: String(error?.message ?? error) });
    }
  };

  await check("apiVersion is a positive integer", () => {
    assert(Number.isInteger(remote.apiVersion), `got ${remote.apiVersion}`);
    assert(remote.apiVersion >= 1, `got ${remote.apiVersion}`);
    return `v${remote.apiVersion}`;
  });

  await check("image identity is this image", () => {
    assert(remote.image.id === "ui-playground", `got ${remote.image.id}`);
    return `${remote.image.name} ${remote.image.version ?? ""}`.trim();
  });

  await check("every advertised slot has a name", () => {
    const names = Object.values(remote.slots);
    assert(names.length > 0, "no slots advertised");
    for (const name of names) {
      assert(typeof name === "string" && name.length > 0, `bad slot ${name}`);
    }
    return `${names.length} slots`;
  });

  await check("views.url resolves declared views only", () => {
    const url = remote.views.url("panel");
    assert(url?.includes(remote.image.id), `got ${url}`);
    assert(remote.views.url("does-not-exist") === null, "unknown view resolved");
    return url;
  });

  await check("views.load fetches a declared view", async () => {
    const html = await remote.views.load("context");
    assert(html.includes('data-section="selftest"'), "unexpected markup");
    return `${html.length} bytes`;
  });

  await check("views.load rejects an unknown view", async () => {
    try {
      await remote.views.load("does-not-exist");
    } catch {
      return "rejected";
    }
    throw new Error("resolved instead of rejecting");
  });

  await check("assets.url serves a non-view asset", async () => {
    const url = remote.assets.url("assets/logo.svg");
    const response = await fetch(url, { credentials: "same-origin" });
    assert(response.ok, `HTTP ${response.status}`);
    return url;
  });

  await check("assets outside ui/ are refused", async () => {
    const response = await fetch(remote.assets.url("../install.sh"), {
      credentials: "same-origin",
    });
    assert(!response.ok, `HTTP ${response.status} — traversal was served`);
    return `HTTP ${response.status}`;
  });

  await check("an unknown slot is dropped, not thrown", () => {
    // Logs a warning by design; the point is that activation survives it.
    const dispose = remote.ui.register("slot.that.does.not.exist", () => {});
    assert(typeof dispose === "function", "no disposer returned");
    dispose();
    return "dropped";
  });

  await check("a disposer is idempotent", () => {
    const dispose = remote.ui.register(remote.slots.applicationsPanel, () => {});
    dispose();
    dispose();
    return "disposed twice";
  });

  return results;
}

function assert(condition, detail) {
  if (!condition) throw new Error(detail || "assertion failed");
}
