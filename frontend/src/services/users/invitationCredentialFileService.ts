import { pdfContentStream, pdfDocument, type PdfLine } from "../files/pdfDocument.ts";

/** What a saved credentials file is called and what it holds. */
export interface InvitationCredentialFile {
  filename: string;
  mimeType: string;
  content: Uint8Array;
}

export interface InvitationCredentialDetails {
  email: string;
  temporaryPassword: string;
  /** Where the person signs in; printed so the page stands on its own. */
  signInUrl: string;
}

// Printable ASCII is the only text this hand-built PDF can carry faithfully.
// The shared writer would replace anything else with "-", which for a
// password or an address means a document that looks right and does not work.
const PRINTABLE_ASCII = /^[\x20-\x7e]*$/;

/** Thrown instead of writing a file whose text would be altered. */
export class UnsupportedCredentialTextError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "UnsupportedCredentialTextError";
  }
}

// The credentials of a freshly invited or reset account, rendered as a PDF the
// admin can hand over. Built here rather than by a library for the same reason
// as the recovery-code file: a few ASCII lines are not worth a dependency.
//
// Leaf service: it produces bytes and a filename. Handing those to the browser
// is platform/fileDownloadService's job.
class InvitationCredentialFileService {
  private readonly productName = "remote.futrx";
  private readonly heading = "Your sign-in details";
  private readonly notes = [
    "Sign in with the email and temporary password above.",
    "You will be asked to choose a new password the first time you sign in.",
    "Keep this file private and delete it once you have signed in.",
  ];

  /** Why a PDF cannot be made for these details, or null when it can. */
  unsupportedReason(details: Pick<InvitationCredentialDetails, "email">): string | null {
    return PRINTABLE_ASCII.test(details.email)
      ? null
      : "The PDF can only hold plain English letters, digits and symbols, and this email address has other characters. Copy the credentials instead.";
  }

  build(details: InvitationCredentialDetails, generatedAt: Date): InvitationCredentialFile {
    for (const text of [details.email, details.temporaryPassword, details.signInUrl]) {
      if (!PRINTABLE_ASCII.test(text)) {
        throw new UnsupportedCredentialTextError(
          "Credentials with non-ASCII characters cannot be written to a PDF faithfully."
        );
      }
    }
    return {
      filename: `${this.productName}-credentials-${this.isoDate(generatedAt)}.pdf`,
      mimeType: "application/pdf",
      content: this.pdfBytes(details, generatedAt),
    };
  }

  private pdfBytes(details: InvitationCredentialDetails, generatedAt: Date): Uint8Array {
    const content = pdfContentStream(this.pdfLines(details, generatedAt));
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

  private pdfLines(details: InvitationCredentialDetails, generatedAt: Date): PdfLine[] {
    const lines: PdfLine[] = [
      { text: `${this.productName} - ${this.heading}`, x: 56, y: 780, font: "F1", size: 16 },
      { text: `Generated ${this.isoDate(generatedAt)}`, x: 56, y: 758, font: "F1", size: 10 },
      { text: "Sign in at", x: 56, y: 716, font: "F1", size: 10 },
      { text: details.signInUrl, x: 56, y: 698, font: "F2", size: 13 },
      { text: "Email", x: 56, y: 662, font: "F1", size: 10 },
      { text: details.email, x: 56, y: 644, font: "F2", size: 13 },
      { text: "Temporary password", x: 56, y: 608, font: "F1", size: 10 },
      { text: details.temporaryPassword, x: 56, y: 590, font: "F2", size: 15 },
    ];
    let y = 540;
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

export const invitationCredentialFileService = new InvitationCredentialFileService();
