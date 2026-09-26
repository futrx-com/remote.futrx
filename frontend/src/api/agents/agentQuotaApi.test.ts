import assert from "node:assert/strict";
import test from "node:test";
import { agentQuotaApi } from "./agentQuotaApi.ts";

test("quota requests bypass caches, carry cancellation, and preserve order, accounts and zero", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  const controller = new AbortController();
  globalThis.fetch = async (input, init) => {
    assert.equal(input, "/api/agent-quota");
    assert.equal(init?.method, "GET");
    assert.equal(init?.cache, "no-store");
    assert.equal(init?.signal, controller.signal);
    return Response.json({ accounts: [
      { provider: "codex", accountId: "work", session: { usedPercent: 0, measuredAt: 1000 } },
      { provider: "claude", weekly: { status: "allowed", measuredAt: 2000 } },
    ] });
  };

  const quotas = await agentQuotaApi.list(controller.signal);
  assert.deepEqual(quotas.map((quota) => [quota.provider, quota.accountId]), [["codex", "work"], ["claude", ""]]);
  assert.equal(quotas[0].session?.usedPercent, 0);
  assert.equal(quotas[0].session?.window, "session");
  assert.equal(quotas[1].weekly?.usedPercent, undefined);
  assert.equal(quotas[1].weekly?.window, "weekly");
});

test("a failed live read reaches the page with the account it belongs to", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  globalThis.fetch = async () => Response.json({ accounts: [
    { provider: "claude", accountId: "work", error: " sign-in expired " },
    { provider: "claude", accountId: "home", error: 42, session: { usedPercent: 10, measuredAt: 1000 } },
  ] });

  const [failed, readable] = await agentQuotaApi.list();
  assert.deepEqual([failed.accountId, failed.error, failed.session], ["work", "sign-in expired", undefined]);
  assert.deepEqual([readable.accountId, readable.error, readable.session?.usedPercent], ["home", undefined, 10]);
});

test("missing optional quota lists remain valid empty snapshots", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  for (const body of [{}, { accounts: null }, { accounts: [] }]) {
    globalThis.fetch = async () => Response.json(body);
    assert.deepEqual(await agentQuotaApi.list(), []);
  }
});

test("unsuccessful and malformed responses reject instead of erasing a good snapshot", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  for (const status of [401, 403, 500, 503]) {
    globalThis.fetch = async () => new Response("unavailable", { status });
    await assert.rejects(agentQuotaApi.list(), /Failed to load subscription quota/);
  }
  for (const body of [
    null,
    [],
    { error: "unavailable" },
    // A per-provider response names no account, so it cannot be attributed.
    { agents: [{ provider: "claude", session: { measuredAt: 1 } }] },
    { accounts: {} },
    { accounts: [null] },
    { accounts: [{ provider: "" }] },
    { accounts: [{ provider: "claude", accountId: 7 }] },
    { accounts: [{ provider: "claude", session: [] }] },
    { accounts: [{ provider: "claude", session: { window: "weekly" } }] },
    { accounts: [{ provider: "claude", weekly: { window: "monthly" } }] },
    { accounts: [{ provider: "claude", weekly: { window: null } }] },
  ]) {
    globalThis.fetch = async () => Response.json(body);
    await assert.rejects(agentQuotaApi.list(), /Invalid subscription quota/);
  }
  globalThis.fetch = async () => new Response("{broken JSON");
  await assert.rejects(agentQuotaApi.list(), SyntaxError);
});

test("optional malformed reading fields cannot become a false zero or fresh timestamp", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  globalThis.fetch = async () => Response.json({ accounts: [{
    provider: "claude",
    accountId: null,
    session: { usedPercent: "0", measuredAt: "2000", resetsAt: -1, status: {} },
  }] });

  const [quota] = await agentQuotaApi.list();
  assert.equal(quota.accountId, "");
  assert.equal(quota.session?.usedPercent, undefined);
  assert.equal(quota.session?.measuredAt, 0);
  assert.equal(quota.session?.resetsAt, undefined);
  assert.equal(quota.session?.status, undefined);
});
