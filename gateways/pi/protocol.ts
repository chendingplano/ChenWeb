// Harness front-end protocol v1: what ChenWeb sends to a harness gateway and
// the events it gets back. The protocol is harness-neutral; this copy matches
// gateways/claude-code/protocol.ts, and this gateway implements it for Pi.

export const PERMISSION_MODES = ["default", "acceptEdits", "plan"] as const;
export type FrontendPermissionMode = typeof PERMISSION_MODES[number];

export interface TurnRequest { sessionId?: string; message: string; permissionMode?: FrontendPermissionMode; model?: string }
/** A model the harness offers, as its own model picker shows it. */
export interface ModelOption { value: string; resolvedModel?: string; displayName: string; description: string }
export interface Decision { behavior: "allow" | "deny"; updatedInput?: Record<string, unknown>; message?: string }

/** One event of a turn, written to ChenWeb as a line of NDJSON. */
export type HarnessEvent =
  | { type: "session"; sessionId: string; model: string; cwd: string; permissionMode: string }
  | { type: "text"; text: string }
  | { type: "tool_use"; id: string; name: string; input: unknown; parentToolUseId: string | null }
  | { type: "tool_result"; toolUseId: string; isError: boolean; text: string }
  | { type: "permission_request"; requestId: string; toolName: string; input: unknown; title?: string; description?: string; reason?: string }
  | { type: "status"; status: "retrying"; attempt: number }
  | { type: "result"; status: "completed" | "interrupted" | "max_turns" | "error"; sessionId: string; durationMs: number; numTurns: number; costUsd: number; usage: Usage; text?: string }
  | { type: "error"; message: string };

export interface Usage { inputTokens: number; outputTokens: number; cacheReadTokens: number; cacheWriteTokens: number }

/** A content block as ChenWeb sees it, in live events and in session history alike. */
export type Block =
  | { type: "text"; text: string }
  | { type: "tool_use"; id: string; name: string; input: unknown }
  | { type: "tool_result"; toolUseId: string; isError: boolean; text: string }
  | { type: "image" };

export interface HistoryMessage { uuid: string; role: "user" | "assistant"; parentToolUseId: string | null; blocks: Block[] }

export const MAX_TOOL_RESULT_TEXT = 8 * 1024;
const MODEL = /^[A-Za-z0-9._:\[\]-]{1,128}$/;
const SESSION_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export const isSessionId = (value: unknown): value is string => typeof value === "string" && SESSION_ID.test(value);

export function validateTurnRequest(value: unknown): TurnRequest {
  const r = (value ?? {}) as Partial<TurnRequest>;
  if (typeof r !== "object" || typeof r.message !== "string" || !r.message.trim() || r.message.length > 100_000) throw new Error("invalid turn request");
  if (r.sessionId !== undefined && !isSessionId(r.sessionId)) throw new Error("invalid session id");
  if (r.permissionMode !== undefined && !PERMISSION_MODES.includes(r.permissionMode)) throw new Error("invalid permission mode");
  if (r.model !== undefined && (typeof r.model !== "string" || !MODEL.test(r.model))) throw new Error("invalid model");
  return { sessionId: r.sessionId, message: r.message, permissionMode: r.permissionMode, model: r.model };
}

export function validateDecision(value: unknown): Decision {
  const d = (value ?? {}) as Partial<Decision>;
  if (typeof d !== "object" || (d.behavior !== "allow" && d.behavior !== "deny")) throw new Error("invalid decision");
  if (d.updatedInput !== undefined && (typeof d.updatedInput !== "object" || d.updatedInput === null || Array.isArray(d.updatedInput))) throw new Error("invalid decision");
  if (d.message !== undefined && (typeof d.message !== "string" || d.message.length > 2000)) throw new Error("invalid decision");
  return { behavior: d.behavior, updatedInput: d.updatedInput, message: d.message };
}

export function capped(text: string): string {
  return text.length > MAX_TOOL_RESULT_TEXT ? `${text.slice(0, MAX_TOOL_RESULT_TEXT)}\n[truncated]` : text;
}
