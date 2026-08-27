import { useEffect, useState } from "preact/hooks";
import { workspaceApi } from "../../../api/workspaceApi";
import type { WorkspaceMessage } from "../../../types/workspaceApi";
import type { ChatMeta } from "../../../models/chat";
import type { ProjectMeta } from "../../../models/project";
import { workspaceDataProjector } from "../../workspace/workspaceDataProjector";

export function useWorkspaceData(enabled: boolean) {
  const [chats, setChats] = useState<ChatMeta[]>([]);
  const [projects, setProjects] = useState<ProjectMeta[]>([]);
  // Until the first snapshot lands, an empty chat list means "not loaded yet",
  // not "no chats" — callers must not act on the difference before then.
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!enabled) {
      setChats((current) => workspaceDataProjector.replaceChats([], current));
      setProjects((current) => workspaceDataProjector.replaceProjects([], current));
      setLoaded(false);
      return;
    }

    return workspaceApi.subscribe(applyWorkspaceMessage);
  }, [enabled]);

  function applyWorkspaceMessage(message: WorkspaceMessage) {
    switch (message.type) {
      case "workspace.snapshot":
        setChats((current) => workspaceDataProjector.replaceChats(message.chats, current));
        setProjects((current) =>
          workspaceDataProjector.replaceProjects(message.projects, current)
        );
        setLoaded(true);
        break;
      case "chat.upsert":
        setChats((current) => workspaceDataProjector.upsertChat(current, message.chat));
        break;
      case "chat.delete":
        setChats((current) => workspaceDataProjector.removeChat(current, message.id));
        break;
      case "project.upsert":
        setProjects((current) => workspaceDataProjector.upsertProject(current, message.project));
        break;
      case "project.delete":
        setProjects((current) => workspaceDataProjector.removeProject(current, message.id));
        break;
    }
  }

  return {
    chats,
    projects,
    loaded,
  };
}
