import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import { PUBLIC_HOSTNAME } from "../../../config/runtime";
import type { BrowserElementCapture } from "../../../models/browser";
import type { ContainerApp } from "../../../models/project";
import { projectPreviewUrlService } from "../../../services/projects/projectPreviewUrlService.ts";
import { useAgentBrowserSession } from "../../../state/hooks/chat/useAgentBrowserSession";
import { BrowserDrawerHeader } from "./BrowserDrawerHeader";
import { BrowserFrame } from "./BrowserFrame";
import { BrowserGuiView } from "./BrowserGuiView";
import { BrowserResizeHandle } from "./BrowserResizeHandle";
import { buildInspectorUrl } from "./browserUrls";

const browserWidthKey = "remote.futrx.browserDrawerWidth";
const defaultBrowserWidth = 720;
const minBrowserWidth = 360;
const maxBrowserWidth = 1100;
const minChatWidth = 360;

function clampWidth(width: number, maxWidth = maxBrowserWidth): number {
  return Math.min(Math.max(width, minBrowserWidth), Math.max(minBrowserWidth, maxWidth));
}

function readBrowserWidth(): number {
  if (typeof window === "undefined") return defaultBrowserWidth;
  const stored = Number(window.localStorage.getItem(browserWidthKey));
  return Number.isFinite(stored) ? clampWidth(stored) : defaultBrowserWidth;
}

export function BrowserDrawer({
  open,
  projectId,
  projectName,
  projectSlug,
  apps,
  appsLoading,
  selectedPort,
  onSelectPort,
  onRefreshApps,
  onCaptureElement,
  onClose,
}: {
  open: boolean;
  projectId: string;
  projectName: string;
  projectSlug: string;
  apps: ContainerApp[];
  appsLoading: boolean;
  selectedPort: number | null;
  onSelectPort: (port: number | null) => void;
  onRefreshApps: () => void;
  onCaptureElement: (capture: BrowserElementCapture) => void;
  onClose: () => void;
}) {
  const [reloadKey, setReloadKey] = useState(0);
  const [browserWidth, setBrowserWidth] = useState(readBrowserWidth);
  const [resizing, setResizing] = useState(false);
  const [inspectMode, setInspectMode] = useState(false);
  const [useInspectorFrame, setUseInspectorFrame] = useState(false);
  const [guiMode, setGuiMode] = useState(false);
  const asideRef = useRef<HTMLElement>(null);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const guiIframeRef = useRef<HTMLIFrameElement>(null);

  const gui = useAgentBrowserSession({ projectId, enabled: open && guiMode });

  const url = useMemo(
    () => projectPreviewUrlService.build(projectSlug, selectedPort, PUBLIC_HOSTNAME),
    [projectSlug, selectedPort],
  );
  const inspectorUrl = useMemo(() => buildInspectorUrl(url), [url]);
  const iframeUrl = useInspectorFrame && inspectorUrl ? inspectorUrl : url;
  const frameOrigin = useMemo(() => {
    if (!url) return "";
    try {
      return new URL(url).origin;
    } catch {
      return "";
    }
  }, [url]);
  const canLoad = !!url;

  useEffect(() => {
    window.localStorage.setItem(browserWidthKey, String(browserWidth));
  }, [browserWidth]);

  // Drawer closing or switching chats resets transient view modes.
  useEffect(() => {
    if (!open) {
      setInspectMode(false);
      setUseInspectorFrame(false);
      setGuiMode(false);
    }
  }, [open]);

  useEffect(() => {
    setGuiMode(false);
  }, [projectId]);

  useEffect(() => {
    if (!canLoad) {
      setInspectMode(false);
      setUseInspectorFrame(false);
    }
  }, [canLoad]);

  useEffect(() => {
    setInspectMode(false);
    setUseInspectorFrame(false);
  }, [url]);

  useEffect(() => {
    if (!inspectMode || !frameOrigin) return;
    function onMessage(event: MessageEvent) {
      if (event.origin !== frameOrigin) return;
      const data = event.data as { type?: string; payload?: BrowserElementCapture };
      if (!data || typeof data.type !== "string") return;
      if (data.type === "remote-inspector:ready") {
        postInspectState(inspectMode);
      } else if (data.type === "remote-inspector:element-selected" && data.payload) {
        onCaptureElement(data.payload);
        setInspectMode(false);
      }
    }
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [frameOrigin, inspectMode, onCaptureElement]);

  useEffect(() => {
    postInspectState(inspectMode);
  }, [inspectMode, iframeUrl]);

  useEffect(() => {
    if (!open) return;
    function clampToContainer() {
      const container = asideRef.current?.parentElement;
      const bounds = container?.getBoundingClientRect();
      if (!bounds) return;
      const availableWidth = Math.min(maxBrowserWidth, Math.max(minBrowserWidth, bounds.width - minChatWidth));
      setBrowserWidth((width) => clampWidth(width, availableWidth));
    }
    clampToContainer();
    window.addEventListener("resize", clampToContainer);
    return () => window.removeEventListener("resize", clampToContainer);
  }, [open]);

  function handleResizeStart(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    setResizing(true);

    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    function finishResize() {
      setResizing(false);
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener("pointermove", resize);
      window.removeEventListener("pointerup", finishResize);
      window.removeEventListener("pointercancel", finishResize);
    }

    function resize(moveEvent: PointerEvent) {
      const container = asideRef.current?.parentElement;
      const bounds = container?.getBoundingClientRect();
      if (!bounds) return;

      const availableWidth = Math.min(maxBrowserWidth, Math.max(minBrowserWidth, bounds.width - minChatWidth));
      const next = bounds.right - moveEvent.clientX;
      setBrowserWidth(clampWidth(next, availableWidth));
    }

    window.addEventListener("pointermove", resize, { passive: false });
    window.addEventListener("pointerup", finishResize);
    window.addEventListener("pointercancel", finishResize);
  }

  function postInspectState(enabled: boolean) {
    if (!frameOrigin) return;
    iframeRef.current?.contentWindow?.postMessage(
      { type: "remote-inspector:set-enabled", enabled },
      frameOrigin
    );
  }

  function toggleInspectMode() {
    setInspectMode((enabled) => {
      const next = !enabled;
      if (next) setUseInspectorFrame(true);
      return next;
    });
  }

  // One refresh action for the toolbar: re-scan the container's listening
  // apps and reload the active frame together, so there is a single refresh
  // control instead of two look-alike buttons.
  function handleRefresh() {
    if (!guiMode) onRefreshApps();
    setReloadKey((value) => value + 1);
  }

  // Agent Browser is mutually exclusive with the app preview's inspect mode.
  function toggleGuiMode() {
    setGuiMode((on) => {
      const next = !on;
      if (next) {
        setInspectMode(false);
        setUseInspectorFrame(false);
      }
      return next;
    });
  }

  return (
    <aside
      ref={asideRef}
      id="workspace-browser-pane"
      class={`workspace-pane workspace-browser-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line
              ${resizing ? "transition-none" : "transition-[width,opacity] duration-200 ease-out"}
              ${open ? "opacity-100 shadow-2xl" : "opacity-0 border-l-0 shadow-none pointer-events-none"}`}
      style={`--workspace-browser-width: ${browserWidth}px; --workspace-browser-max-width: max(${minBrowserWidth}px, calc(100% - ${minChatWidth}px));`}
      aria-hidden={!open}
      aria-label="Browser preview"
    >
      <BrowserResizeHandle resizing={resizing} onPointerDown={handleResizeStart} />
      <div
        class={`h-full min-h-0 w-full flex flex-col transition-transform duration-200 ease-out
                ${open ? "translate-x-0" : "translate-x-full"}`}
      >
        <BrowserDrawerHeader
          projectName={projectName}
          apps={apps}
          appsLoading={appsLoading}
          selectedPort={selectedPort}
          url={url}
          canLoad={canLoad}
          inspectMode={inspectMode}
          guiMode={guiMode}
          guiStatus={gui.status}
          onSelectPort={onSelectPort}
          onToggleInspectMode={toggleInspectMode}
          onToggleGuiMode={toggleGuiMode}
          onStopGui={gui.stop}
          onRefresh={handleRefresh}
          onClose={onClose}
        />
        {guiMode ? (
          <BrowserGuiView
            status={gui.status}
            url={gui.guiUrl}
            error={gui.error}
            reloadKey={reloadKey}
            projectName={projectName}
            resizing={resizing}
            iframeRef={guiIframeRef}
          />
        ) : (
          <BrowserFrame
            canLoad={canLoad}
            iframeRef={iframeRef}
            iframeUrl={iframeUrl}
            reloadKey={reloadKey}
            projectName={projectName}
            resizing={resizing}
            inspectMode={inspectMode}
            onFrameLoad={postInspectState}
          />
        )}
      </div>
    </aside>
  );
}
