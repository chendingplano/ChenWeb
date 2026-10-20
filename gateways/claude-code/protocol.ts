// The SemOS gateway protocol, as ChenWeb's harness adapters send it. It is
// the same request shape the Pi gateway (ThirdParty/pi/gateway/types.ts)
// accepts; the limits below must stay in step with that file.

export const KNOWLEDGE_TOOLS = ["search_knowledge", "read_source_passages", "get_artifact_details", "get_document_context", "find_related_knowledge"] as const;
export type KnowledgeToolName = typeof KNOWLEDGE_TOOLS[number];
/** Returns the full stored result of an earlier tool call; reads the conversation, not a store. */
export const SAVED_RESULT_TOOL = "get_saved_tool_result";
export const GATEWAY_TOOLS = [...KNOWLEDGE_TOOLS, SAVED_RESULT_TOOL] as const;
export type GatewayToolName = typeof GATEWAY_TOOLS[number];
export const isKnowledgeTool = (tool: unknown): tool is KnowledgeToolName => KNOWLEDGE_TOOLS.includes(tool as KnowledgeToolName);
export const isGatewayTool = (tool: unknown): tool is GatewayToolName => GATEWAY_TOOLS.includes(tool as GatewayToolName);

export type ThinkingLevel = "off" | "minimal" | "low" | "medium" | "high" | "xhigh" | "max";
export interface GatewayProfile {
  slug: string; version: string; provider: string; model: string; systemPrompt: string;
  thinkingLevel: ThinkingLevel; permissionDefault: "ask" | "auto"; allowedTools: GatewayToolName[];
  maxToolCalls: number; maxElapsedMs: number; maxOutputTokens: number; maxEvidenceBytes: number;
}
export interface HistoryToolCall { id: string; name: string; arguments: Record<string, unknown>; result: string; isError: boolean }
export interface HistoryMessage { role: "user" | "assistant"; content: string; toolCalls?: HistoryToolCall[] }
export interface KnowledgeStore { id: string; name: string }
export interface KnowledgeContext { stores: KnowledgeStore[]; defaultStoreId?: string; promptContext: string }
export interface RunRequest { runId: string; conversationId: string; userId: string; message: string; capability: string; profile: GatewayProfile; history: HistoryMessage[]; knowledge?: KnowledgeContext; historySummary?: string }
export interface SummaryRequest { provider: string; model: string; systemPrompt: string; previousSummary?: string; messages: HistoryMessage[]; maxTokens: number }
/** One normalized run event, written to ChenWeb as a line of NDJSON. */
export type PublicEvent = { type: string; [key: string]: unknown };

const THINKING_LEVELS: readonly unknown[] = ["off", "minimal", "low", "medium", "high", "xhigh", "max"];
const text = (value: unknown, max: number): value is string => typeof value === "string" && value.trim().length > 0 && value.length <= max;
const positive = (value: unknown, max: number): value is number => Number.isInteger(value) && (value as number) > 0 && (value as number) <= max;

export function validateRunRequest(value: unknown): RunRequest {
  if (!value || typeof value !== "object") throw new Error("invalid run request");
  const r = value as Partial<RunRequest>;
  const p = r.profile as Partial<GatewayProfile> | undefined;
  if (!text(r.runId, 128) || !text(r.conversationId, 128) || !text(r.userId, 256) || !text(r.message, 16_000) || !text(r.capability, 16_000) || !p) throw new Error("invalid run request");
  if (!text(p.slug, 128) || !text(p.version, 64) || !text(p.provider, 128) || !text(p.model, 256) || !text(p.systemPrompt, 64_000)) throw new Error("invalid profile");
  // The Agent SDK talks only to Anthropic models.
  if (p.provider !== "anthropic") throw new Error("invalid profile provider");
  if (!THINKING_LEVELS.includes(p.thinkingLevel) || !(["ask", "auto"] as unknown[]).includes(p.permissionDefault)) throw new Error("invalid profile");
  if (!Array.isArray(p.allowedTools) || p.allowedTools.length > GATEWAY_TOOLS.length || p.allowedTools.some((tool) => !isGatewayTool(tool))) throw new Error("invalid profile tools");
  if (!positive(p.maxToolCalls, 100) || !positive(p.maxElapsedMs, 10 * 60_000) || !positive(p.maxOutputTokens, 100_000) || !positive(p.maxEvidenceBytes, 128 * 1024)) throw new Error("invalid profile limits");
  const history = r.history ?? [];
  if (!Array.isArray(history) || history.length > 100 || history.some((m) => !m || !(["user", "assistant"] as unknown[]).includes(m.role) || !text(m.content, 16_000) || !validToolCalls(m))) throw new Error("invalid history");
  if (r.historySummary !== undefined && (typeof r.historySummary !== "string" || r.historySummary.length > 16_000)) throw new Error("invalid history summary");
  const knowledge = validateKnowledge(r.knowledge);
  if (p.allowedTools.some(isKnowledgeTool) && !knowledge?.stores.length) throw new Error("invalid knowledge context");
  return { ...(r as RunRequest), message: r.message.trim(), capability: r.capability.trim(), history, knowledge, historySummary: r.historySummary?.trim() || undefined };
}

export function validateSummaryRequest(value: unknown): SummaryRequest {
  if (!value || typeof value !== "object") throw new Error("invalid summary request");
  const r = value as Partial<SummaryRequest>;
  if (r.provider !== "anthropic" || !text(r.model, 256) || !text(r.systemPrompt, 64_000) || !positive(r.maxTokens, 8_000)) throw new Error("invalid summary request");
  if (r.previousSummary !== undefined && (typeof r.previousSummary !== "string" || r.previousSummary.length > 16_000)) throw new Error("invalid summary request");
  if (!Array.isArray(r.messages) || r.messages.length === 0 || r.messages.length > 100 || r.messages.some((m) => !m || !(["user", "assistant"] as unknown[]).includes(m.role) || !text(m.content, 16_000))) throw new Error("invalid summary request");
  return { provider: r.provider, model: r.model, systemPrompt: r.systemPrompt, previousSummary: r.previousSummary?.trim() || undefined, messages: r.messages.map((m) => ({ role: m.role, content: m.content })), maxTokens: r.maxTokens };
}

function validToolCalls(m: HistoryMessage): boolean {
  if (m.toolCalls === undefined) return true;
  if (m.role !== "assistant" || !Array.isArray(m.toolCalls) || m.toolCalls.length > 100) return false;
  return m.toolCalls.every((c) => c && text(c.id, 128) && text(c.name, 64) && typeof c.isError === "boolean" && typeof c.result === "string" && c.result.length <= 64_000
    && !!c.arguments && typeof c.arguments === "object" && !Array.isArray(c.arguments) && JSON.stringify(c.arguments).length <= 16_000);
}

function validateKnowledge(value: unknown): KnowledgeContext | undefined {
  if (value === undefined || value === null) return undefined;
  const k = value as Partial<KnowledgeContext>;
  if (typeof k !== "object" || !Array.isArray(k.stores) || k.stores.length > 32 || !text(k.promptContext, 8_000)) throw new Error("invalid knowledge context");
  if (k.stores.some((s) => !s || !text(s.id, 128) || typeof s.name !== "string" || s.name.length > 256)) throw new Error("invalid knowledge context");
  if (k.defaultStoreId !== undefined && !k.stores.some((s) => s.id === k.defaultStoreId)) throw new Error("invalid knowledge context");
  return { stores: k.stores.map((s) => ({ id: s.id, name: s.name })), defaultStoreId: k.defaultStoreId, promptContext: k.promptContext };
}
