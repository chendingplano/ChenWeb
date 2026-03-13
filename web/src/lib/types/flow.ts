// web/src/lib/types/flow.ts

export interface Connector {
  id: string;       // e.g. "out-text", "in-context"
  label: string;    // display name
  direction: 'input' | 'output';
}

export interface NodeType {
  id: string;           // e.g. "ai-assistant"
  label: string;
  category: string;     // "AI" | "Data" | "Actions"
  icon: string;         // lucide icon name
  inputs: string[];     // connector labels
  outputs: string[];
  defaultData: Record<string, unknown>;
}

export interface FlowNode {
  id: string;
  type: string;         // matches NodeType.id
  position: { x: number; y: number };
  data: Record<string, unknown>;
}

export interface FlowEdge {
  id: string;
  source: string;
  sourceHandle: string;
  target: string;
  targetHandle: string;
}

export interface FlowData {
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export interface Flow {
  flow_id: number;
  user_id: number;
  flow_name: string;
  flow_desc: string;
  is_default: boolean;
  is_shared: boolean;
  is_template: boolean;
  template_category: string;
  flow_data: FlowData;
  thumbnail_svg: string | null;
  created_at: string;
  updated_at: string;
}

export interface Snapshot {
  nodes: FlowNode[];
  edges: FlowEdge[];
}

export interface ApiError {
  error: { code: string; message: string };
}
