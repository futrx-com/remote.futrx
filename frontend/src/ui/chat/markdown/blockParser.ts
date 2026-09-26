import type { ListItem, MarkdownBlock } from "./types";

const fencePattern = /^(`{3,}|~{3,})\s*([A-Za-z0-9_+.-]*)?.*$/;
const headingPattern = /^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/;
const hrPattern = /^\s{0,3}([-*_])(?:\s*\1){2,}\s*$/;
const listPattern = /^(\s*)(?:([-*+])|(\d+)[.)])\s+(.*)$/;

export function parseMarkdown(markdown: string): MarkdownBlock[] {
  return parseMarkdownWithEnding(markdown).blocks;
}

// Only newline-terminated lines can establish a block boundary. A partial
// marker ("-", "1.", "|") can change type as more characters arrive; parsing
// it as a new block would reveal and then retract the preceding list/table.
export function parseStreamingMarkdown(markdown: string, settled = false): MarkdownBlock[] {
  if (settled) return parseMarkdown(markdown);
  // Hold a trailing CR until we know whether the provider is sending CRLF.
  const normalized = markdown.replace(/\r$/, "").replace(/\r\n?/g, "\n");
  const complete = normalized.slice(0, normalized.lastIndexOf("\n") + 1);
  const { blocks, lastClosedFence, lastOpenFence } = parseMarkdownWithEnding(complete);
  if (blocks.length === 0) return blocks;
  const last = blocks[blocks.length - 1];
  const separated = !lastOpenFence && /\n[ \t]*\n$/.test(complete);
  const singleLineComplete = last.type === "heading" || last.type === "hr";
  if (lastClosedFence || separated || singleLineComplete) return blocks;
  if (last.type === "table") {
    // The delimiter establishes the table; each newline-terminated row is
    // complete and can be shown without waiting for the final blank line.
    return blocks;
  }
  if (last.type === "list") {
    // Each item is stable once the next complete item starts. Publish those
    // items now while keeping the final item buffered for continuation lines.
    const items = last.items.slice(0, -1);
    return items.length ? [...blocks.slice(0, -1), { ...last, items }] : blocks.slice(0, -1);
  }
  return blocks.slice(0, -1);
}

function parseMarkdownWithEnding(markdown: string): {
  blocks: MarkdownBlock[];
  lastClosedFence: boolean;
  lastOpenFence: boolean;
} {
  const lines = markdown.replace(/\r\n?/g, "\n").split("\n");
  const blocks: MarkdownBlock[] = [];
  let index = 0;
  let lastClosedFence = false;
  let lastOpenFence = false;

  while (index < lines.length) {
    const line = lines[index];
    if (isBlank(line)) {
      index++;
      continue;
    }

    const fence = line.match(fencePattern);
    if (fence) {
      const marker = fence[1];
      const lang = fence[2]?.trim() || undefined;
      const code: string[] = [];
      index++;
      while (index < lines.length && !isClosingFence(lines[index], marker)) {
        code.push(lines[index]);
        index++;
      }
      const closed = index < lines.length;
      if (closed) index++;
      blocks.push({ type: "code", lang, text: code.join("\n") });
      lastClosedFence = closed;
      lastOpenFence = !closed;
      continue;
    }

    lastClosedFence = false;
    lastOpenFence = false;

    const heading = line.match(headingPattern);
    if (heading) {
      blocks.push({
        type: "heading",
        level: heading[1].length as 1 | 2 | 3 | 4 | 5 | 6,
        text: heading[2],
      });
      index++;
      continue;
    }

    if (hrPattern.test(line)) {
      blocks.push({ type: "hr" });
      index++;
      continue;
    }

    if (isTableStart(lines, index)) {
      const { block, next } = parseTable(lines, index);
      blocks.push(block);
      index = next;
      continue;
    }

    if (line.trimStart().startsWith(">")) {
      const quoteLines: string[] = [];
      while (index < lines.length && lines[index].trimStart().startsWith(">")) {
        quoteLines.push(lines[index].trimStart().replace(/^>\s?/, ""));
        index++;
      }
      blocks.push({ type: "blockquote", children: parseMarkdown(quoteLines.join("\n")) });
      continue;
    }

    const list = line.match(listPattern);
    if (list) {
      const { block, next } = parseList(lines, index, !!list[3]);
      blocks.push(block);
      index = next;
      continue;
    }

    const paragraph: string[] = [line.trim()];
    index++;
    while (index < lines.length && !isBlank(lines[index]) && !isBlockStart(lines, index)) {
      paragraph.push(lines[index].trim());
      index++;
    }
    blocks.push({ type: "paragraph", text: paragraph.join("\n") });
  }

  return { blocks, lastClosedFence, lastOpenFence };
}

function parseList(lines: string[], startIndex: number, ordered: boolean): { block: MarkdownBlock; next: number } {
  const items: ListItem[] = [];
  const first = lines[startIndex].match(listPattern);
  const start = ordered && first?.[3] ? Number(first[3]) : undefined;
  let index = startIndex;

  while (index < lines.length) {
    const match = lines[index].match(listPattern);
    if (!match || !!match[3] !== ordered) break;

    const item = parseListItem(match[4]);
    index++;
    while (index < lines.length && /^\s{2,}\S/.test(lines[index]) && !listPattern.test(lines[index])) {
      item.text += "\n" + lines[index].trim();
      index++;
    }
    items.push(item);
  }

  return { block: { type: "list", ordered, start, items }, next: index };
}

function parseListItem(raw: string): ListItem {
  const task = raw.match(/^\[([ xX])\]\s+(.*)$/);
  if (!task) return { text: raw };
  return { text: task[2], checked: task[1].toLowerCase() === "x" };
}

function parseTable(lines: string[], startIndex: number): { block: MarkdownBlock; next: number } {
  const header = splitTableRow(lines[startIndex]);
  const rows: string[][] = [];
  let index = startIndex + 2;

  while (index < lines.length && lines[index].includes("|") && !isBlank(lines[index])) {
    rows.push(normalizeRow(splitTableRow(lines[index]), header.length));
    index++;
  }

  return { block: { type: "table", header, rows }, next: index };
}

function normalizeRow(row: string[], length: number): string[] {
  if (row.length === length) return row;
  if (row.length > length) return row.slice(0, length);
  return [...row, ...Array.from({ length: length - row.length }, () => "")];
}

function isBlockStart(lines: string[], index: number): boolean {
  const line = lines[index];
  return (
    fencePattern.test(line) ||
    headingPattern.test(line) ||
    hrPattern.test(line) ||
    line.trimStart().startsWith(">") ||
    listPattern.test(line) ||
    isTableStart(lines, index)
  );
}

function isTableStart(lines: string[], index: number): boolean {
  if (index + 1 >= lines.length || !lines[index].includes("|")) return false;
  const header = splitTableRow(lines[index]);
  const delimiter = splitTableRow(lines[index + 1]);
  return (
    header.length > 1 &&
    delimiter.length === header.length &&
    delimiter.every((cell) => /^:?-{3,}:?$/.test(cell))
  );
}

function splitTableRow(line: string): string[] {
  let trimmed = line.trim();
  if (trimmed.startsWith("|")) trimmed = trimmed.slice(1);
  if (trimmed.endsWith("|")) trimmed = trimmed.slice(0, -1);
  return trimmed.split("|").map((cell) => cell.trim());
}

function isClosingFence(line: string, marker: string): boolean {
  const fence = marker[0].repeat(marker.length);
  return line.trimStart().startsWith(fence);
}

function isBlank(line: string): boolean {
  return line.trim().length === 0;
}
