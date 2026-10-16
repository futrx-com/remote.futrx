import { pdfContentStream, pdfDocument, type PdfLine } from "../files/pdfDocument.ts";

export type RecoveryCodeFileFormat = "txt" | "pdf";

/** What a saved recovery-code file is called and what it holds. */
export interface RecoveryCodeFile {
  filename: string;
  mimeType: string;
  content: Uint8Array;
}

// The recovery codes a user is shown once, rendered as a file they can keep:
// a plain .txt and a PDF built here rather than by a library, because a PDF
// of fifteen short ASCII lines is a page of text objects and a cross-reference
// table - not worth a dependency, and worth being able to read.
//
// Leaf service: it produces bytes and a filename. Handing those to the browser
// is platform/fileDownloadService's job.
class RecoveryCodeFileService {
  private readonly productName = "remote.futrx";
  private readonly heading = "Two-factor recovery codes";
  private readonly notes = [
    "Each code can be used once to sign in if you lose your authenticator app.",
    "Keep this file somewhere only you can reach. Anyone holding a code can",
    "complete two-factor authentication as you.",
    "Generating new codes invalidates every code listed here.",
  ];

  build(codes: string[], format: RecoveryCodeFileFormat, generatedAt: Date): RecoveryCodeFile {
    return format === "pdf"
      ? {
          filename: this.filename("pdf", generatedAt),
          mimeType: "application/pdf",
          content: this.pdfBytes(codes, generatedAt),
        }
      : {
          filename: this.filename("txt", generatedAt),
          mimeType: "text/plain;charset=utf-8",
          content: new TextEncoder().encode(this.text(codes, generatedAt)),
        };
  }

  /** "remote.futrx-recovery-codes-2026-09-04.txt" - dated, so a second
   *  download after regenerating does not overwrite the first silently. */
  filename(format: RecoveryCodeFileFormat, generatedAt: Date): string {
    return `${this.productName}-recovery-codes-${this.isoDate(generatedAt)}.${format}`;
  }

  private text(codes: string[], generatedAt: Date): string {
    return [
      `${this.productName} - ${this.heading}`,
      `Generated ${this.isoDate(generatedAt)}`,
      "",
      ...codes.map((code, index) => `${String(index + 1).padStart(2, " ")}. ${code}`),
      "",
      ...this.notes,
      "",
    ].join("\n");
  }

  private pdfBytes(codes: string[], generatedAt: Date): Uint8Array {
    const lines = this.pdfLines(codes, generatedAt);
    const content = pdfContentStream(lines);
    const objects = [
      "<< /Type /Catalog /Pages 2 0 R >>",
      "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
      "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] " +
        "/Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> /Contents 4 0 R >>",
      `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
      "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
      "<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>",
    ];
    return new TextEncoder().encode(pdfDocument(objects));
  }

  /** The page as positioned lines, top-down: title, date, the codes in two
   *  columns of a monospaced face, then the notes. */
  private pdfLines(codes: string[], generatedAt: Date): PdfLine[] {
    const lines: PdfLine[] = [
      { text: `${this.productName} - ${this.heading}`, x: 56, y: 780, font: "F1", size: 16 },
      { text: `Generated ${this.isoDate(generatedAt)}`, x: 56, y: 758, font: "F1", size: 10 },
    ];
    const rows = Math.ceil(codes.length / 2);
    codes.forEach((code, index) => {
      lines.push({
        text: `${String(index + 1).padStart(2, " ")}.  ${code}`,
        x: index < rows ? 56 : 300,
        y: 716 - (index % rows) * 24,
        font: "F2",
        size: 13,
      });
    });
    let y = 716 - rows * 24 - 24;
    for (const note of this.notes) {
      lines.push({ text: note, x: 56, y, font: "F1", size: 10 });
      y -= 15;
    }
    return lines;
  }

  private isoDate(date: Date): string {
    return [
      date.getFullYear(),
      String(date.getMonth() + 1).padStart(2, "0"),
      String(date.getDate()).padStart(2, "0"),
    ].join("-");
  }
}

export const recoveryCodeFileService = new RecoveryCodeFileService();
