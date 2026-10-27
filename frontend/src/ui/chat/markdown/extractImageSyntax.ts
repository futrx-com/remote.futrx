// Pure string consumer for markdown image syntax `![alt](src)`. The position
// helper backs the inline lexer; the list extractor wraps it with the same
// `\!` skipping the inline parser does not currently do, so unit tests pin
// the exact behaviour callers should expect.

export interface ImageSyntax {
  start: number;
  /** exclusive — points past the closing ")" */
  end: number;
  alt: string;
  src: string;
}

/**
 * Try to read an `![alt](src)` span starting exactly at `index`. Returns
 * `null` when `index` does not point at `![` or the syntax is malformed —
 * unclosed brackets, a missing `(`, or an empty src. Does not honour
 * `\!`; callers that need to skip a backslash-escaped bang do so in their
 * own walk, which is what the inline parser does (it never produced
 * `\!` literal handling before this helper existed).
 */
export function findImageSyntaxAt(text: string, index: number): ImageSyntax | null {
  if (index < 0 || index >= text.length) return null;
  if (text[index] !== "!" || text[index + 1] !== "[") return null;

  const labelEnd = findUnescaped(text, "]", index + 2);
  if (labelEnd < 0) return null;
  if (text[labelEnd + 1] !== "(") return null;

  const hrefEnd = findUnescaped(text, ")", labelEnd + 2);
  if (hrefEnd < 0) return null;

  const alt = text.slice(index + 2, labelEnd);
  const rawSrc = text.slice(labelEnd + 2, hrefEnd).trim();
  if (!rawSrc) return null;

  return { start: index, end: hrefEnd + 1, alt, src: rawSrc };
}

/**
 * Walk the whole string and return every well-formed `![alt](src)` span.
 * `\!` is treated as a literal `!` and does not start a syntax.
 */
export function extractImageSyntaxes(text: string): ImageSyntax[] {
  const out: ImageSyntax[] = [];
  let i = 0;
  while (i < text.length) {
    if (text[i] === "\\" && text[i + 1] === "!") {
      i += 2;
      continue;
    }
    const found = findImageSyntaxAt(text, i);
    if (!found) {
      i++;
      continue;
    }
    out.push(found);
    i = found.end;
  }
  return out;
}

function findUnescaped(text: string, target: string, start: number): number {
  for (let i = start; i < text.length; i++) {
    if (text[i] === "\\") {
      i++;
      continue;
    }
    if (text[i] === target) return i;
  }
  return -1;
}
