// AttachmentChip — file/image chip in the composer attachment tray.
import { AttachmentChip } from "remote.futrx-web";

const noop = () => {};

const thumb =
  "data:image/svg+xml," +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="160" height="160"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#3b5bdb"/><stop offset="1" stop-color="#0f1014"/></linearGradient></defs><rect width="160" height="160" fill="url(#g)"/><circle cx="118" cy="44" r="22" fill="#f5d76e" opacity="0.9"/><path d="M0 120 L48 76 L92 116 L124 92 L160 124 L160 160 L0 160 Z" fill="#1f2733"/></svg>`
  );

export const Uploaded = () => (
  <div className="w-full max-w-md flex flex-wrap gap-2">
    <AttachmentChip
      attachment={{
        id: "att-1",
        name: "quarterly-report.pdf",
        size: 482_311,
        serverPath: "/uploads/quarterly-report.pdf",
        isImage: false,
      }}
      onRemove={noop}
    />
  </div>
);

export const Uploading = () => (
  <div className="w-full max-w-md flex flex-wrap gap-2">
    <AttachmentChip
      attachment={{
        id: "att-2",
        name: "server-logs-2026-08-15.txt",
        size: 9_812_004,
        serverPath: "",
        isImage: false,
        progress: 0.47,
      }}
      onRemove={noop}
    />
  </div>
);

export const Failed = () => (
  <div className="w-full max-w-md flex flex-wrap gap-2">
    <AttachmentChip
      attachment={{
        id: "att-3",
        name: "db-dump.sqlite",
        size: 52_428_800,
        serverPath: "",
        isImage: false,
        error: "upload exceeded the 25 MB limit",
      }}
      onRemove={noop}
    />
  </div>
);

export const ImageUploaded = () => (
  <div className="w-full max-w-md flex flex-wrap gap-2">
    <AttachmentChip
      attachment={{
        id: "att-4",
        name: "dashboard-screenshot.png",
        size: 214_500,
        serverPath: "/uploads/dashboard-screenshot.png",
        isImage: true,
        objectUrl: thumb,
      }}
      onRemove={noop}
    />
  </div>
);

export const ImageUploading = () => (
  <div className="w-full max-w-md flex flex-wrap gap-2">
    <AttachmentChip
      attachment={{
        id: "att-5",
        name: "hero-banner.jpg",
        size: 3_402_118,
        serverPath: "",
        isImage: true,
        objectUrl: thumb,
        progress: 0.62,
      }}
      onRemove={noop}
    />
  </div>
);

export const MixedTray = () => (
  <div className="w-full max-w-xl flex flex-wrap gap-2">
    <AttachmentChip
      attachment={{
        id: "att-6",
        name: "dashboard-screenshot.png",
        size: 214_500,
        serverPath: "/uploads/dashboard-screenshot.png",
        isImage: true,
        objectUrl: thumb,
      }}
      onRemove={noop}
    />
    <AttachmentChip
      attachment={{
        id: "att-7",
        name: "quarterly-report.pdf",
        size: 482_311,
        serverPath: "/uploads/quarterly-report.pdf",
        isImage: false,
      }}
      onRemove={noop}
    />
    <AttachmentChip
      attachment={{
        id: "att-8",
        name: "notes.md",
        size: 4_120,
        serverPath: "",
        isImage: false,
        progress: 0.12,
      }}
      onRemove={noop}
    />
  </div>
);
