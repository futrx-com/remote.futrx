// BrowserEmptyState — placeholder inside the browser-preview pane when no dev server runs.
import { BrowserEmptyState } from "remote.futrx-web";

export const Pane = () => (
  <div className="w-full max-w-xl border border-white/10 rounded-lg overflow-hidden" style={{ height: 320 }}>
    <BrowserEmptyState />
  </div>
);

export const ShortPane = () => (
  <div className="w-full max-w-xl border border-white/10 rounded-lg overflow-hidden" style={{ height: 160 }}>
    <BrowserEmptyState />
  </div>
);
