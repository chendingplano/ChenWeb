import { query, type EffortLevel, type Options, type SDKMessage, type SDKUserMessage } from "@anthropic-ai/claude-agent-sdk";
import { MCP_SERVER, createRunTools, type GatewayFetch, type RunTools } from "./tools.ts";
import type { PermissionGate } from "./permission-gate.ts";
import type { HistoryMessage, PublicEvent, RunRequest, SummaryRequest, ThinkingLevel } from "./protocol.ts";

export type QueryFn = typeof query;
export type TerminalStatus = "completed" | "failed" | "cancelled" | "timed_out" | "limit_reached";

/** What a run needs from its environment; injectable so tests need no model. */
export interface RunEnvironment {
  query: QueryFn;
  baseUrl: string;
  gatewaySecret: string;
  /** An empty directory used as the run's working and home directory. */
  workDir: string;
  /** Model credentials passed to the Claude Code process, and nothing else. */
  credentials: Record<string, string>;
  fetch?: GatewayFetch;
}

export const SUMMARY_HEADING = "## Summary of earlier turns in this conversation";
const HISTORY_NOTE = "The user's message may begin with earlier turns of this conversation inside <earlier_turns>. Treat them as context only, and answer the question inside <current_question>.";

const EFFORT: Record<ThinkingLevel, EffortLevel> = { off: "low", minimal: "low", low: "low", medium: "medium", high: "high", xhigh: "xhigh", max: "max" };

/**
 * Process environment for Claude Code: model credentials and a home
 * directory of its own, so no user settings, memory, CLAUDE.md, plugins or
 * the gateway's own secrets reach the run.
 */
export function claudeProcessEnv(env: RunEnvironment, maxOutputTokens: number): Record<string, string> {
  return {
    ...env.credentials,
    PATH: process.env.PATH ?? "/usr/bin:/bin",
    HOME: env.workDir,
    CLAUDE_CONFIG_DIR: `${env.workDir}/.claude`,
    TMPDIR: env.workDir,
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: String(maxOutputTokens),
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
    CLAUDE_AGENT_SDK_CLIENT_APP: "semos-claude-code-gateway/0.1.0",
  };
}

/** The Agent SDK options that confine a run to SemOS's tool boundary. */
export function runOptions(run: RunRequest, env: RunEnvironment, tools: RunTools, abortController: AbortController): Options {
  return {
    abortController,
    cwd: env.workDir,
    env: claudeProcessEnv(env, run.profile.maxOutputTokens),
    // No built-in tools (files, shell, web, subagents): only the SemOS server.
    tools: [],
    allowedTools: [],
    mcpServers: { [MCP_SERVER]: tools.server },
    strictMcpConfig: true,
    settingSources: [],
    skills: [],
    agents: {},
    plugins: [],
    persistSession: false,
    permissionMode: "default",
    canUseTool: tools.canUseTool,
    systemPrompt: systemPrompt(run),
    model: run.profile.model,
    thinking: { type: "adaptive" },
    effort: EFFORT[run.profile.thinkingLevel],
    maxTurns: run.profile.maxToolCalls + 1,
    includePartialMessages: true,
  };
}

export function systemPrompt(run: RunRequest): string {
  return [
    run.profile.systemPrompt,
    ...(run.knowledge ? [run.knowledge.promptContext] : []),
    ...(run.historySummary ? [`${SUMMARY_HEADING}\n\n${run.historySummary}`] : []),
    ...(run.history.length > 0 ? [HISTORY_NOTE] : []),
  ].join("\n\n");
}

/**
 * The Agent SDK cannot be seeded with earlier assistant turns, so history is
 * replayed as a transcript ahead of the question. Earlier tool calls are
 * shown with their stored results, as the Pi gateway does for tools a run
 * no longer offers.
 */
export function promptText(run: RunRequest): string {
  if (run.history.length === 0) return run.message;
  const turns = run.history.map(renderTurn).join("\n\n");
  return `<earlier_turns>\n${turns}\n</earlier_turns>\n\n<current_question>\n${run.message}\n</current_question>`;
}

function renderTurn(message: HistoryMessage): string {
  if (message.role === "user") return `[User]\n${message.content}`;
  const calls = (message.toolCalls ?? []).map((c) => `[Tool call ${c.id}: ${c.name} ${JSON.stringify(c.arguments)}${c.isError ? " (failed)" : ""}]\n${c.result}`);
  return [...calls, `[Assistant]\n${message.content}`].join("\n\n");
}

export interface ActiveRun { cancel(): void; done: Promise<TerminalStatus> }

/** Starts one turn. Events go to emit; the last one is always a completion. */
export function startRun(run: RunRequest, env: RunEnvironment, gate: PermissionGate, emit: (event: PublicEvent) => void): ActiveRun {
  const abortController = new AbortController();
  const flags = { cancelled: false, timedOut: false, limitReached: false };
  const stop = () => { gate.cancelAll(); abortController.abort(); };
  const tools = createRunTools({
    baseUrl: env.baseUrl, gatewaySecret: env.gatewaySecret, capability: run.capability, runId: run.runId,
    allowedTools: run.profile.allowedTools, knowledgeStoreIds: run.knowledge?.stores.map((s) => s.id) ?? [], defaultStoreId: run.knowledge?.defaultStoreId,
    maxEvidenceBytes: run.profile.maxEvidenceBytes, maxToolCalls: run.profile.maxToolCalls, gate, emit, fetch: env.fetch,
  });

  const done = (async (): Promise<TerminalStatus> => {
    let finished!: () => void;
    const resultSeen = new Promise<void>((resolve) => { finished = resolve; });
    // Streaming input keeps the session open until the result arrives, which
    // in-process MCP tools require.
    async function* prompt(): AsyncGenerator<SDKUserMessage> {
      yield { type: "user", message: { role: "user", content: promptText(run) }, parent_tool_use_id: null };
      await resultSeen;
    }
    const timer = setTimeout(() => { flags.timedOut = true; stop(); }, run.profile.maxElapsedMs);
    let outcome: TerminalStatus = "failed";
    let committed = 0, current = 0;
    try {
      for await (const message of env.query({ prompt: prompt(), options: runOptions(run, env, tools, abortController) })) {
        const result = handleMessage(message);
        if (result.answer) emit({ type: "answer_delta", text: result.answer });
        if (result.retry !== undefined) emit({ type: "activity", status: "retrying", attempt: Math.min(5, result.retry) });
        if (result.messageStart) { committed += current; current = 0; }
        if (result.outputTokens !== undefined) {
          current = result.outputTokens;
          if (committed + current >= run.profile.maxOutputTokens && !flags.limitReached) { flags.limitReached = true; stop(); }
        }
        if (result.usage) emit({ type: "usage", ...result.usage });
        if (result.outcome) { outcome = result.outcome; finished(); break; }
      }
    } catch {
      outcome = "failed";
    } finally {
      clearTimeout(timer);
      finished();
      gate.cancelAll();
    }
    const status: TerminalStatus = flags.timedOut ? "timed_out" : flags.limitReached ? "limit_reached" : flags.cancelled ? "cancelled" : outcome;
    if (status !== "completed") emit({ type: "error", message: errorMessage(status) });
    emit({ type: "completion", status });
    return status;
  })();
  return { cancel: () => { flags.cancelled = true; stop(); }, done };
}

interface HandledMessage {
  answer?: string; retry?: number; messageStart?: boolean; outputTokens?: number;
  usage?: { inputTokens: number; outputTokens: number; cacheReadTokens: number; cacheWriteTokens: number };
  outcome?: TerminalStatus;
}

// Only the main agent's text and its final result leave the gateway; hidden
// reasoning, tool payloads and SDK internals are dropped.
export function handleMessage(message: SDKMessage): HandledMessage {
  if (message.type === "stream_event") {
    if (message.parent_tool_use_id !== null) return {};
    const event = message.event;
    if (event.type === "message_start") return { messageStart: true };
    if (event.type === "content_block_delta" && event.delta.type === "text_delta" && event.delta.text) return { answer: event.delta.text };
    if (event.type === "message_delta" && typeof event.usage?.output_tokens === "number") return { outputTokens: event.usage.output_tokens };
    return {};
  }
  if (message.type === "system" && message.subtype === "api_retry") return { retry: Number(message.attempt) || 0 };
  if (message.type === "result") {
    const u = message.usage;
    const usage = { inputTokens: Number(u?.input_tokens) || 0, outputTokens: Number(u?.output_tokens) || 0, cacheReadTokens: Number(u?.cache_read_input_tokens) || 0, cacheWriteTokens: Number(u?.cache_creation_input_tokens) || 0 };
    if (message.subtype === "success" && !message.is_error) return { usage, outcome: "completed" };
    if (message.subtype === "error_max_turns") return { usage, outcome: "limit_reached" };
    return { usage, outcome: "failed" };
  }
  return {};
}

function errorMessage(status: TerminalStatus): string {
  switch (status) {
    case "timed_out": return "The agent run timed out.";
    case "cancelled": return "Run cancelled.";
    case "limit_reached": return "The agent run reached its output limit.";
    default: return "The agent run failed.";
  }
}

/** Folds earlier turns into a rolling summary with the conversation's model and no tools. */
export async function summarize(request: SummaryRequest, env: RunEnvironment): Promise<string> {
  const turns = request.messages.map((m) => `${m.role === "user" ? "User" : "Assistant"}: ${m.content}`).join("\n\n");
  const prompt = `${request.previousSummary ? `Previous summary:\n${request.previousSummary}\n\n` : ""}Turns to fold into the summary:\n\n${turns}`;
  const abortController = new AbortController();
  const timer = setTimeout(() => abortController.abort(), 60_000);
  try {
    for await (const message of env.query({ prompt, options: {
      abortController, cwd: env.workDir, env: claudeProcessEnv(env, request.maxTokens), tools: [], allowedTools: [], mcpServers: {}, strictMcpConfig: true,
      settingSources: [], skills: [], agents: {}, plugins: [], persistSession: false, systemPrompt: request.systemPrompt, model: request.model, effort: "low", maxTurns: 1,
    } })) {
      if (message.type === "result") {
        if (message.subtype !== "success" || message.is_error) throw new Error("summary failed");
        return message.result;
      }
    }
    throw new Error("summary failed");
  } finally {
    clearTimeout(timer);
  }
}
