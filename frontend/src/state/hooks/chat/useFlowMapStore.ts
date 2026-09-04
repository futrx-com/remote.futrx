import { useEffect, useState, useCallback } from "preact/hooks";
import type { FlowMapState, FlowNode, ViewMode } from "../../../models/flow";
import { flowService } from "../../../services/flow/flowService";

export function useFlowMapStore(chatId: string | null) {
  const [flowState, setFlowState] = useState<FlowMapState | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [zoom, setZoom] = useState<number>(1);
  const [viewMode, setViewMode] = useState<ViewMode>("chat");
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  const refreshFlowState = useCallback(async () => {
    if (!chatId) return;
    try {
      setLoading(true);
      const state = await flowService.getFlowState(chatId);
      setFlowState(state);
      if (state.activeTargetNodeId && !selectedNodeId) {
        setSelectedNodeId(state.activeTargetNodeId);
      }
      setError(null);
    } catch (err: any) {
      setError(err?.message || "Failed to load flow map");
    } finally {
      setLoading(false);
    }
  }, [chatId, selectedNodeId]);

  useEffect(() => {
    if (!chatId) return;
    refreshFlowState();
    // Poll for flow state updates every 3s during active computer use turns
    const interval = setInterval(refreshFlowState, 3000);
    return () => clearInterval(interval);
  }, [chatId, refreshFlowState]);

  const selectedNode: FlowNode | undefined = flowState?.nodes.find(
    (n) => n.id === selectedNodeId
  );

  return {
    flowState,
    selectedNodeId,
    setSelectedNodeId,
    selectedNode,
    zoom,
    setZoom,
    viewMode,
    setViewMode,
    loading,
    error,
    refreshFlowState,
  };
}
