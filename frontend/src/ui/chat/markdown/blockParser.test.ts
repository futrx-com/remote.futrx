import assert from "node:assert/strict";
import test from "node:test";
import { parseMarkdown, parseStreamingMarkdown } from "./blockParser.ts";

test("holds an unfinished paragraph until its Markdown and inline syntax are complete", () => {
  assert.deepEqual(parseStreamingMarkdown("A **bold"), []);
  assert.deepEqual(parseStreamingMarkdown("A **bold**\n"), []);
  assert.deepEqual(parseStreamingMarkdown("A **bold**\n\n"), [
    { type: "paragraph", text: "A **bold**" },
  ]);
});

test("reveals completed blocks while keeping the next block buffered", () => {
  const text = "# Result\nThe first paragraph.\n\n- one\n- two";
  assert.deepEqual(parseStreamingMarkdown(text), [
    { type: "heading", level: 1, text: "Result" },
    { type: "paragraph", text: "The first paragraph." },
  ]);
  assert.deepEqual(parseStreamingMarkdown(`${text}\n\n`), parseMarkdown(text));
});

test("does not reveal an incomplete table header as a paragraph", () => {
  assert.deepEqual(parseStreamingMarkdown("Name | Value\n--- | --"), []);
  assert.deepEqual(parseStreamingMarkdown("Name | Value\n--- | ---\nA | 1"), [
    { type: "table", header: ["Name", "Value"], rows: [] },
  ]);
  assert.deepEqual(parseStreamingMarkdown("Name | Value\n--- | ---\nA | 1\nB | 2"), [
    { type: "table", header: ["Name", "Value"], rows: [["A", "1"]] },
  ]);
  assert.deepEqual(parseStreamingMarkdown("Name | Value\n--- | ---\nA | 1\n\n"), [
    { type: "table", header: ["Name", "Value"], rows: [["A", "1"]] },
  ]);
});

test("reveals fenced code only when its matching closing fence arrives", () => {
  const open = "```ts\nconst answer = 42;\n\n";
  assert.deepEqual(parseStreamingMarkdown(open), []);
  assert.deepEqual(parseStreamingMarkdown(`${open}~~~`), []);
  const closed = `${open}\`\`\`\n`;
  assert.deepEqual(parseStreamingMarkdown(closed), [
    { type: "code", lang: "ts", text: "const answer = 42;\n" },
  ]);
  assert.deepEqual(parseStreamingMarkdown(`${closed}\nNext paragraph`), parseStreamingMarkdown(closed));
});

test("settling flushes the unfinished final block, including an open fence", () => {
  for (const text of ["Final paragraph **done**", "```js\nconst x = 1;"]) {
    assert.deepEqual(parseStreamingMarkdown(text, true), parseMarkdown(text));
  }
});

test("reveals a heading at the end of its line and a quote after its separator", () => {
  assert.deepEqual(parseStreamingMarkdown("# مرحبا"), []);
  assert.deepEqual(parseStreamingMarkdown("# مرحبا\n"), [
    { type: "heading", level: 1, text: "مرحبا" },
  ]);
  const quote = "> Quoted **text**\n\n";
  assert.deepEqual(parseStreamingMarkdown(quote), parseMarkdown(quote));
});

// Check every transport split, especially ambiguous list and table markers.
test("visible blocks never retract during character-by-character streaming", () => {
  const documents = [
    "# Result\n\nSummary **done**.\n\n- first\n- second\n  continued\n- third\n\nNext.\n\n",
    "1. first\n2. second\n10. third\n\n### Next\n",
    "Name | Value\n--- | ---\nA | 1\nB | 2\n\nDone.\n\n",
    "> first\n> second\n\n---\n\n```ts\nconst a = 1;\n\n```\n\nAfter.\n\n",
    "# العربية\r\n\r\n- واحد\r\n- اثنان\r\n\r\n",
  ];
  for (const document of documents) {
    let previous = [] as ReturnType<typeof parseStreamingMarkdown>;
    for (let end = 0; end <= document.length; end++) {
      const blocks = parseStreamingMarkdown(document.slice(0, end));
      assert.ok(blocks.length >= previous.length, `retracted at ${JSON.stringify(document.slice(0, end))}`);
      previous.forEach((block, index) => {
        const current = blocks[index];
        if (block.type === "list" && current.type === "list") {
          assert.equal(current.ordered, block.ordered);
          assert.equal(current.start, block.start);
          assert.deepEqual(current.items.slice(0, block.items.length), block.items);
        } else if (block.type === "table" && current.type === "table") {
          assert.deepEqual(current.header, block.header);
          assert.deepEqual(current.rows.slice(0, block.rows.length), block.rows);
        } else {
          assert.deepEqual(current, block, `changed at ${JSON.stringify(document.slice(0, end))}`);
        }
      });
      previous = blocks;
    }
    assert.deepEqual(parseStreamingMarkdown(document, true), parseMarkdown(document));
    assert.deepEqual(previous, parseMarkdown(document));
  }
});

test("holds a list while the next marker is incomplete", () => {
  for (const suffix of ["-", "- ", "- second"]) {
    assert.deepEqual(parseStreamingMarkdown(`- first\n${suffix}`), []);
  }
  assert.deepEqual(parseStreamingMarkdown("- first\n- second\n"), [
    { type: "list", ordered: false, start: undefined, items: [{ text: "first" }] },
  ]);
  assert.deepEqual(parseStreamingMarkdown("- first\n- second\n- third\n"), [
    { type: "list", ordered: false, start: undefined, items: [{ text: "first" }, { text: "second" }] },
  ]);
});
