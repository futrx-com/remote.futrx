import assert from "node:assert/strict";
import test from "node:test";
import { fetchFullTranscriptContent, fetchTranscript, searchTranscript } from "./chatTranscriptApi.ts";

test("fetches and flattens transcript turns without changing cursor metadata", async (t) => {
  const page = {
    turns: [
      {
        id: "turn-1",
        startSeq: 1,
        endSeq: 3,
        events: [
          { seq: 1, t: 1, type: "user", text: "older question" },
          { seq: 2, t: 2, type: "assistant_text", text: "older answer" },
          { seq: 3, t: 3, type: "complete" },
        ],
      },
      {
        id: "turn-2",
        startSeq: 4,
        endSeq: 6,
        events: [
          { seq: 4, t: 4, type: "user", text: "new question" },
          { seq: 5, t: 5, type: "assistant_text", text: "new answer" },
          { seq: 6, t: 6, type: "complete" },
        ],
      },
    ],
    nextBefore: 1,
    lastSeq: 6,
    hasMore: true,
  };
  const originalFetch = globalThis.fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });
  globalThis.fetch = async (input, init) => {
    assert.equal(input, "/api/chats/chat%2F1/transcript?limit=20&before=4");
    assert.equal(init?.method, "GET");
    return Response.json(page);
  };

  assert.deepEqual(await fetchTranscript("chat/1", { limit: 20, before: 4 }), {
    events: [...page.turns[0].events, ...page.turns[1].events],
    nextBefore: 1,
    lastSeq: 6,
    hasMore: true,
  });
});

test("loads oversized transcript content through complete cursor pages", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });
  const requests: string[] = [];
  globalThis.fetch = async (input) => {
    requests.push(String(input));
    if (requests.length === 1) {
      return Response.json({
        contentId: "content-1",
        content: "first ",
        nextAfter: 6,
        totalBytes: 12,
        complete: false,
      });
    }
    return Response.json({
      contentId: "content-1",
      content: "second",
      totalBytes: 12,
      complete: true,
    });
  };

  assert.equal(await fetchFullTranscriptContent("chat/1", "content-1"), "first second");
  assert.deepEqual(requests, [
    "/api/chats/chat%2F1/transcript/content?id=content-1",
    "/api/chats/chat%2F1/transcript/content?id=content-1&after=6",
  ]);
});

test("searches saved history independently of loaded transcript pages", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  const hits = [{ turnId: "old-turn", itemId: "item", startSeq: 1, endSeq: 3, text: "old result", highlights: [{ start: 0, end: 3 }] }];
  globalThis.fetch = async (input, init) => {
    assert.equal(input, "/api/chats/chat%2F1/search?q=old+result&limit=100");
    assert.equal(init?.method, "GET");
    return Response.json(hits);
  };
  assert.deepEqual(await searchTranscript("chat/1", "old result"), hits);
});

test("continues a search below the previous page's oldest match", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  globalThis.fetch = async (input) => {
    assert.equal(input, "/api/chats/chat%2F1/search?q=test&limit=100&before=42");
    return Response.json([]);
  };
  assert.deepEqual(await searchTranscript("chat/1", "test", 42), []);
});

test("requests a window of turns around an event and reports newer turns", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  globalThis.fetch = async (input) => {
    assert.equal(input, "/api/chats/chat%2F1/transcript?limit=21&anchor=500&newer=10");
    return Response.json({ turns: [], lastSeq: 900, hasMore: true, nextBefore: 400, newerFrom: 610 });
  };
  const page = await fetchTranscript("chat/1", { limit: 21, anchor: 500, newer: 10 });
  assert.equal(page.newerFrom, 610);
  assert.equal(page.nextBefore, 400);
});
