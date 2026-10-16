/** One positioned line of text on a PDF page. */
export interface PdfLine {
  text: string;
  x: number;
  y: number;
  font: string;
  size: number;
}

// Minimal PDF assembly shared by anything that renders a short page of text:
// a content stream of positioned lines and a document with its
// cross-reference table. Page layout and font objects stay with the caller.

export function pdfContentStream(lines: PdfLine[]): string {
  return lines
    .map(
      (line) =>
        `BT /${line.font} ${line.size} Tf 1 0 0 1 ${line.x} ${line.y} Tm ` +
        `(${escapePdfText(line.text)}) Tj ET`
    )
    .join("\n");
}

/** Assembles numbered objects into a document, recording each one's byte
 *  offset in the cross-reference table a reader uses to find it. */
export function pdfDocument(objects: string[]): string {
  const header = "%PDF-1.4\n";
  let body = "";
  const offsets: number[] = [];
  objects.forEach((object, index) => {
    offsets.push(header.length + body.length);
    body += `${index + 1} 0 obj\n${object}\nendobj\n`;
  });
  const xrefOffset = header.length + body.length;
  const xref = [
    "xref",
    `0 ${objects.length + 1}`,
    "0000000000 65535 f ",
    ...offsets.map((offset) => `${String(offset).padStart(10, "0")} 00000 n `),
  ].join("\n");
  const trailer =
    `\ntrailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\n` +
    `startxref\n${xrefOffset}\n%%EOF\n`;
  return header + body + xref + trailer;
}

/** Backslash, parentheses and non-ASCII bytes cannot travel raw inside a
 *  PDF string literal. */
function escapePdfText(text: string): string {
  return text
    .replace(/\\/g, "\\\\")
    .replace(/\(/g, "\\(")
    .replace(/\)/g, "\\)")
    .replace(/[^\x20-\x7e]/g, "-");
}
