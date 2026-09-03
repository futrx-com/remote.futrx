import { useCallback, useEffect, useState } from "preact/hooks";
import type { RefObject } from "preact";
import type { BrowserElementCapture } from "../../../models/browser";
import type { ChatMeta } from "../../../models/chat";
import type { ContainerApp, ProjectMeta } from "../../../models/project";
import type { ChatMessageBlock } from "../../../models/chatMessage";
import { projectApi } from "../../../api/projectApi";
import { projectPreviewUrlService } from "../../../services/projects/projectPreviewUrlService.ts";
import { chatBrowserState } from "./chatBrowserState";

export function useChatBrowserController({
  chat,
  projects,
  blocks,
  text,
  setText,
  textareaRef,
}: {
  chat: ChatMeta;
  projects: ProjectMeta[];
  blocks: ChatMessageBlock[];
  text: string;
  setText: (text: string) => void;
  textareaRef: RefObject<HTMLTextAreaElement>;
}) {
  const [browserOpen, setBrowserOpen] = useState(false);
  const [containerApps, setContainerApps] = useState<ContainerApp[]>([]);
  const [appsLoading, setAppsLoading] = useState(false);
  const [selectedAppPort, setSelectedAppPort] = useState<number | null>(null);
  const browserProject = chat.projectId
    ? projects.find((project) => project.id === chat.projectId) ?? null
    : null;
  const browserUrl = browserProject
    ? chatBrowserState.latestPublicDevUrl(blocks, browserProject.slug)
    : "";

  useEffect(() => {
    setBrowserOpen(false);
  }, [chat.id]);

  const loadContainerApps = useCallback(async () => {
    if (!chat.projectId) {
      setContainerApps([]);
      setSelectedAppPort(null);
      return;
    }
    setAppsLoading(true);
    try {
      const apps = await projectApi.listApps(chat.projectId);
      setContainerApps(apps);
      setSelectedAppPort((prev) => {
        if (apps.length === 0) return null;
        if (prev != null && apps.some((app) => app.port === prev)) return prev;
        const hinted = projectPreviewUrlService.port(browserUrl);
        if (hinted != null && apps.some((app) => app.port === hinted)) return hinted;
        return apps[apps.length - 1].port;
      });
    } catch {
      setContainerApps([]);
      setSelectedAppPort(null);
    } finally {
      setAppsLoading(false);
    }
  }, [chat.projectId, browserUrl]);

  function openBrowserDrawer() {
    if (!chat.projectId) {
      alert("This chat is not attached to a project container.");
      return;
    }
    setBrowserOpen(true);
    void loadContainerApps();
  }

  function insertBrowserElementContext(capture: BrowserElementCapture) {
    const insertion = `\n\n${chatBrowserState.formatElementCapture(capture)}\n\n`;
    const textarea = textareaRef.current;
    const start = textarea?.selectionStart ?? text.length;
    const end = textarea?.selectionEnd ?? start;
    const next = `${text.slice(0, start)}${insertion}${text.slice(end)}`;
    setText(next);
    setTimeout(() => {
      textareaRef.current?.focus();
      const pos = start + insertion.length;
      textareaRef.current?.setSelectionRange(pos, pos);
    }, 0);
  }

  return {
    browserOpen,
    browserProject,
    containerApps,
    appsLoading,
    selectedAppPort,
    setSelectedAppPort,
    openBrowserDrawer,
    closeBrowserDrawer: () => setBrowserOpen(false),
    loadContainerApps,
    insertBrowserElementContext,
  };
}
