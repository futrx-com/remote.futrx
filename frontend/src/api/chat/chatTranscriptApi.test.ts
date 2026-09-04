import assert from "node:assert/strict";
import test from "node:test";
import { fetchTranscript } from "./chatTranscriptApi.ts";

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
