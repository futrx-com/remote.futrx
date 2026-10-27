// Handing bytes the app already holds to the browser as a file the user keeps.
// Nothing here is fetched: the caller built the content, and this only wraps
// the object-URL/anchor dance every browser still requires.
//
// Leaf service: it knows about the browser, never about what is being saved.
class FileDownloadService {
  save(content: Uint8Array | string, filename: string, mimeType: string): void {
    const url = URL.createObjectURL(new Blob([content as BlobPart], { type: mimeType }));
    try {
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = filename;
      anchor.rel = "noopener";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
    } finally {
      // Safari needs the URL to outlive the click, and every browser leaks the
      // blob until it is revoked.
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
  }
}

export const fileDownloadService = new FileDownloadService();
