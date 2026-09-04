import assert from "node:assert/strict";
import test from "node:test";
import { ANY_DATE } from "../../config/search.ts";
import { DAY_MS as DAY } from "../../config/time.ts";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { UNASSIGNED_PROJECT } from "../../models/search.ts";
import type { ChatSearchDoc, SearchFilters } from "../../models/search.ts";
import { searchFilterService } from "./searchFilterService.ts";
import { workspaceSearchService } from "./workspaceSearchService.ts";

const NOW = new Date(2026, 7, 29, 12, 0, 0).getTime();

const projects: ProjectMeta[] = [
  {
    id: "p-remote",
    name: "Remote Futrx",
    slug: "remote-futrx",
    cwd: "/var/lib/remote/projects/remote-futrx/workspace",
    containerName: "remote-futrx",
    status: "running",
    createdAt: NOW - 40 * DAY,
    updatedAt: NOW,
  },
  {
    id: "p-docs",
    name: "Docs Site",
    slug: "docs-site",
    cwd: "/var/lib/remote/projects/docs-site/workspace",
    containerName: "docs-site",
    status: "stopped",
    createdAt: NOW - 20 * DAY,
    updatedAt: NOW,
  },
];

const chats: ChatMeta[] = [
  {
    id: "c1",
    title: "Caddy TLS on-demand ask",
    projectId: "p-remote",
    provider: "claude",
    model: "opus",
    mode: "code",
    cwd: "/workspace/backend",
    createdAt: NOW - 3 * DAY,
    lastMessageAt: NOW - 1 * DAY,
    lastReadAt: NOW,
  },
  {
    id: "c2",
    title: "Sidebar search rewrite",
    projectId: "p-remote",
    provider: "claude",
    model: "sonnet",
    mode: "plan",
    cwd: "/workspace/frontend",
    createdAt: NOW - 10 * DAY,
    lastMessageAt: NOW - 2 * DAY,
    lastReadAt: 0,
    selectedSkills: [{ name: "code-review" }],
  },
  {
    id: "c3",
    title: "Publish docs",
    projectId: "p-docs",
    provider: "codex",
    model: "gpt-5.5",
    createdAt: NOW - 45 * DAY,
    lastMessageAt: NOW - 40 * DAY,
    lastReadAt: NOW,
    running: true,
  },
  {
    id: "c4",
    title: "Scratch notes",
    createdAt: NOW - 2 * DAY,
    lastMessageAt: NOW - 2 * DAY,
    lastReadAt: NOW,
  },
];

const docs = workspaceSearchService.buildIndex(chats, projects);

function filters(overrides: Partial<SearchFilters> = {}): SearchFilters {
  return { facets: searchFilterService.emptyFacetSelections(), date: ANY_DATE, ...overrides };
}

function idsFor(query: string, override: Partial<SearchFilters> = {}): string[] {
  return workspaceSearchService.run(docs, filters(override), query, "relevance", NOW).hits.map(
    (hit) => hit.doc.chat.id
  );
}


/** The options one facet offers, which is what the filter menu renders. */
function offeredValues(
  over: readonly ChatSearchDoc[],
  active: SearchFilters,
  facetId: string,
  withCounts = true
): string[] {
  const outcome = workspaceSearchService.run(over, active, "", "relevance", NOW, { withCounts });
  return workspaceSearchService
    .facetViews(over, active, outcome)
    .find((view) => view.id === facetId)!
    .options.map((option) => option.value);
}

// The old `.includes()` filter failed all four of these.
test("matches words out of order across separators", () => {
  assert.deepEqual(idsFor("futrx remote"), ["c1", "c2"]);
});

test("matches despite a typo", () => {
  assert.deepEqual(idsFor("sidbar"), ["c2"]);
});

test("names the field a hit matched on, so the row can say why", () => {
  const hits = workspaceSearchService.run(docs, filters(), "workspace", "relevance", NOW).hits;
  assert.ok(hits.length > 1);
  assert.equal(hits[0].matchedField, "path");
});

test("reports title spans for highlighting", () => {
  const hit = workspaceSearchService.run(docs, filters(), "caddy", "relevance", NOW).hits[0];
  assert.deepEqual(hit.titleSpans, [{ start: 0, end: 5 }]);
  assert.equal("Caddy TLS on-demand ask".slice(0, 5), "Caddy");
});

test("selecting several projects ORs within the facet", () => {
  const facets = searchFilterService.emptyFacetSelections();
  facets.project = ["p-remote", "p-docs"];
  assert.deepEqual(idsFor("", { facets }).sort(), ["c1", "c2", "c3"]);
});

test("unassigned chats are selectable as their own project option", () => {
  const facets = searchFilterService.emptyFacetSelections();
  facets.project = [UNASSIGNED_PROJECT];
  assert.deepEqual(idsFor("", { facets }), ["c4"]);
});

test("a deleted project does not become a filter option of its own", () => {
  const orphanDocs = workspaceSearchService.buildIndex(
    [
      ...chats,
      {
        id: "c5",
        title: "Left behind",
        projectId: "p-deleted",
        createdAt: NOW - DAY,
        lastMessageAt: NOW - DAY,
        lastReadAt: NOW,
      },
    ],
    projects
  );
  const values = offeredValues(orphanDocs, filters(), "project", false);
  // The raw id would otherwise show up as a project the user never created.
  assert.equal(values.includes("p-deleted"), false);
  assert.deepEqual(values, ["p-docs", "p-remote", UNASSIGNED_PROJECT]);

  // And the chat is still reachable, filed with the rest of the unassigned.
  const facets = searchFilterService.emptyFacetSelections();
  facets.project = [UNASSIGNED_PROJECT];
  const hits = workspaceSearchService.run(orphanDocs, filters({ facets }), "", "relevance", NOW).hits;
  assert.deepEqual(hits.map((hit) => hit.doc.chat.id).sort(), ["c4", "c5"]);
});

test("different facets AND together", () => {
  const facets = searchFilterService.emptyFacetSelections();
  facets.project = ["p-remote"];
  facets.model = ["sonnet"];
  assert.deepEqual(idsFor("", { facets }), ["c2"]);
});

test("status facet finds unread and running chats", () => {
  const unread = searchFilterService.emptyFacetSelections();
  unread.status = ["unread"];
  assert.deepEqual(idsFor("", { facets: unread }), ["c2"]);

  const running = searchFilterService.emptyFacetSelections();
  running.status = ["running"];
  assert.deepEqual(idsFor("", { facets: running }), ["c3"]);
});

test("date filter bounds by the selected field", () => {
  const recent = workspaceSearchService.run(
    docs,
    filters({ date: { preset: "7d", field: "lastMessageAt" } }),
    "",
    "recent",
    NOW
  );
  assert.deepEqual(recent.hits.map((hit) => hit.doc.chat.id), ["c1", "c2", "c4"]);

  const created = workspaceSearchService.run(
    docs,
    filters({ date: { preset: "7d", field: "createdAt" } }),
    "",
    "recent",
    NOW
  );
  assert.deepEqual(created.hits.map((hit) => hit.doc.chat.id), ["c1", "c4"]);
});

test("custom date ranges are inclusive of both endpoint days", () => {
  const range = searchFilterService.resolveDateRange(
    { preset: "custom", field: "lastMessageAt", from: "2026-08-01", to: "2026-08-01" },
    NOW
  );
  assert.equal(range.from, new Date(2026, 7, 1, 0, 0, 0, 0).getTime());
  assert.equal(range.to, new Date(2026, 7, 1, 23, 59, 59, 999).getTime());
});

test("a malformed custom date is ignored rather than matching nothing", () => {
  const range = searchFilterService.resolveDateRange(
    { preset: "custom", field: "lastMessageAt", from: "2026-02-31" },
    NOW
  );
  assert.equal(range.from, null);
});

test("facet counts are computed against the other active facets", () => {
  const facets = searchFilterService.emptyFacetSelections();
  facets.project = ["p-remote"];
  const outcome = workspaceSearchService.run(docs, filters({ facets }), "", "recent", NOW, { withCounts: true });

  // Model counts respect the project filter...
  assert.equal(outcome.counts.model.get("opus"), 1);
  assert.equal(outcome.counts.model.get("gpt-5.5"), undefined);
  // ...but the project facet's own counts ignore it, so you can see what
  // ticking another project would add.
  assert.equal(outcome.counts.project.get("p-docs"), 1);
});

test("empty query with no filters returns everything, most recent first", () => {
  const outcome = workspaceSearchService.run(docs, filters(), "", "relevance", NOW);
  assert.deepEqual(outcome.hits.map((hit) => hit.doc.chat.id), ["c1", "c2", "c4", "c3"]);
  assert.equal(outcome.total, 4);
});

test("stays fast on a large workspace", () => {
  const manyChats: ChatMeta[] = [];
  for (let i = 0; i < 2000; i += 1) {
    manyChats.push({
      id: `bulk-${i}`,
      title: `Refactor the workspace sidebar search ${i}`,
      projectId: i % 2 === 0 ? "p-remote" : "p-docs",
      model: i % 3 === 0 ? "opus" : "sonnet",
      cwd: "/workspace/frontend/src/state",
      createdAt: NOW - i * 1000,
      lastMessageAt: NOW - i * 1000,
    });
  }
  const bulkDocs = workspaceSearchService.buildIndex(manyChats, projects);

  const started = process.hrtime.bigint();
  for (let run = 0; run < 20; run += 1) {
    workspaceSearchService.run(bulkDocs, filters(), "sidebar serch", "relevance", NOW);
  }
  const perRunMs = Number(process.hrtime.bigint() - started) / 1e6 / 20;

  // Measures ~5ms for 2000 chats with a typo query (the slow path); a real
  // workspace of ~500 chats is under 2ms. The ceiling is loose enough for slow
  // CI while still catching an order-of-magnitude regression.
  assert.ok(perRunMs < 40, `search took ${perRunMs.toFixed(1)}ms per run`);
});

test("picking a provider scopes the model and mode facets to it", () => {
  // Unfiltered, every model and mode any chat has ever used is on offer.
  assert.deepEqual(offeredValues(docs, filters(), "model").sort(), ["", "gpt-5.5", "opus", "sonnet"]);
  assert.deepEqual(offeredValues(docs, filters(), "mode").sort(), ["", "code", "plan"]);

  // Models and modes belong to a provider, so choosing one scopes both: Codex's
  // model rather than Claude's, and no Claude-only modes left to tick.
  const codex = searchFilterService.emptyFacetSelections();
  codex.provider = ["codex"];
  assert.deepEqual(offeredValues(docs, filters({ facets: codex }), "model"), ["gpt-5.5"]);
  assert.deepEqual(offeredValues(docs, filters({ facets: codex }), "mode"), [""]);

  const claude = searchFilterService.emptyFacetSelections();
  claude.provider = ["claude"];
  assert.deepEqual(offeredValues(docs, filters({ facets: claude }), "model").sort(), ["opus", "sonnet"]);
  assert.deepEqual(offeredValues(docs, filters({ facets: claude }), "mode").sort(), ["code", "plan"]);

  // The provider facet itself stays whole -- scoping a facet by the others
  // never scopes it by itself, or you could not change your mind.
  assert.deepEqual(offeredValues(docs, filters({ facets: codex }), "provider").sort(), ["", "claude", "codex"]);

  // A selection that can no longer match anything stays listed, so it can be
  // seen and unticked rather than silently hiding every result. Codex's own
  // model is listed beside it, because a facet is never scoped by itself: the
  // pair reads "this is why you have nothing, and here is what would work".
  const impossible = searchFilterService.emptyFacetSelections();
  impossible.provider = ["codex"];
  impossible.model = ["opus"];
  assert.deepEqual(offeredValues(docs, filters({ facets: impossible }), "model").sort(), ["gpt-5.5", "opus"]);
  assert.deepEqual(idsFor("", { facets: impossible }), []);

  // Scoping is derived from the counts, so with no filter menu open -- nothing
  // asking for counts -- the unscoped list stands in rather than collapsing to
  // the selection. The chips read it then, and only for values already ticked.
  assert.deepEqual(offeredValues(docs, filters({ facets: codex }), "model", false).sort(), [
    "",
    "gpt-5.5",
    "opus",
    "sonnet",
  ]);
});
