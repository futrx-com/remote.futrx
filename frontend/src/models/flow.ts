export type FlowNodeType = "checkpoint" | "order" | "tree_node" | "gate";

export type FlowNodeStatus = "created" | "processing" | "completed" | "failed" | "gated";

export interface AuditEntry {
  verb: string;
  actor: string;
  status: string;
  timestamp: string;
  note?: string;
}

export interface FlowNode {
  id: string;
  seq: number;
  turnId?: string;
  type: FlowNodeType;
  title: string;
  description?: string;
  verb: string;
  status: FlowNodeStatus;
  icon: string;
  binaryPath?: string; // Maestro bitstring address (e.g. "0101")
  timestamp: number;
  durationMs?: number;
  parentId?: string;
  payload?: Record<string, any>;
  auditTrail?: AuditEntry[];
}

export interface FlowEdge {
  id: string;
  fromNodeId: string;
  toNodeId: string;
  label?: string;
  type: string;
}

export interface Checkpoint {
  id: string;
  name: string;
  report?: string;
  status: string;
  nodeIds: string[];
  timestamp: number;
}

export interface ProcessTreeNode {
  id: string;
  binaryPath: string;
  title: string;
  status: string;
  priority?: string;
  children?: ProcessTreeNode[];
}

export interface FlowMapState {
  chatId: string;
  activeCheckpoint?: string;
  checkpoints: Checkpoint[];
  nodes: FlowNode[];
  edges: FlowEdge[];
  processTree: ProcessTreeNode[];
  activeTargetNodeId?: string;
  lastUpdated: number;
}

export type ViewMode = "chat" | "map" | "split";
