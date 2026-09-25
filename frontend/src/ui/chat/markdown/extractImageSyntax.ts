// Pulled out for unit testing: a small helper that walks markdown inline
// content and reports the spans of markdown-image syntax `![alt](src)`.
// Implemented as a pure string consumer so it does not need a DOM.

export interface ImageSyntax {
  start: number;
  end: number; // exclusive, points past the closing ")"
  alt: string;
  src: string;
}

export function extractImageSyntaxes(text: string): ImageSyntax[] {
  const out: ImageSyntax[] = [];
  let i = 0;
  while (i < text.length) {
    if (text[i] === "\\" && text[i + 1] === "!") {
      i += 2;
      continue;
    }
    if (text[i] !== "!" || text[i + 1] !== "[") {
      i++;
      continue;
    }
    const labelEnd = findUnescaped(text, "]", i + 2);
    if (labelEnd < 0) {
      i++;
      continue;
    }
    if (text[labelEnd + 1] !== "(") {
      i++;
      continue;
    }
    const hrefEnd = findUnescaped(text, ")", labelEnd + 2);
    if (hrefEnd < 0) {
      i++;
      continue;
    }
    const alt = text.slice(i + 2, labelEnd);
    const rawSrc = text.slice(labelEnd + 2, hrefEnd).trim();
    if (!rawSrc) {
      i++;
      continue;
    }
    out.push({ start: i, end: hrefEnd + 1, alt, src: rawSrc });
    i = hrefEnd + 1;
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
