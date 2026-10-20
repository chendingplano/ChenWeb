import type { CanUseTool, PermissionResult, Query, SDKMessage, SDKUserMessage, query } from "@anthropic-ai/claude-agent-sdk";
import { toBlocks, type Decision, type FrontendPermissionMode, type HarnessEvent } from "./protocol.ts";

export type QueryFn = typeof query;
export type Emit = (event: HarnessEvent) => void;

export interface SessionEnvironment {
  query: QueryFn;
  /** The workspace folder Claude Code runs in. */
  cwd: string;
  /** Claude Code's process environment. */
  env: Record<string, string | undefined>;
  /** A session with no turn for this long is closed; it can be resumed later. */
  idleMs: number;
}

/** Claude Code's own options for a session, as a terminal or IDE would start it. */
export function claudeCodeOptions(env: SessionEnvironment) {
  return {
    cwd: env.cwd,
    env: env.env,
    systemPrompt: { type: "preset", preset: "claude_code" } as const,
    tools: { type: "preset", preset: "claude_code" } as const,
    settingSources: ["user", "project", "local"] as ("user" | "project" | "local")[],
  };
}

export class SessionBusyError extends Error {}
export class SessionClosedError extends Error {}

/** Feeds user messages to Claude Code as they arrive, like typing into a terminal. */
class InputQueue implements AsyncIterable<SDKUserMessage> {
  #items: SDKUserMessage[] = [];
  #wake?: () => void;
  #closed = false;
  push(text: string) { this.#items.push({ type: "user", message: { role: "user", content: text }, parent_tool_use_id: null }); this.#wake?.(); }
  close() { this.#closed = true; this.#wake?.(); }
  async *[Symbol.asyncIterator](): AsyncGenerator<SDKUserMessage> {
    while (true) {
      const item = this.#items.shift();
      if (item) { yield item; continue; }
      if (this.#closed) return;
      await new Promise<void>((resolve) => { this.#wake = resolve; });
      this.#wake = undefined;
    }
  }
}

interface Turn { emit: Emit; finish: () => void; interrupted: boolean }

/**
 * One Claude Code session, kept running between turns. Claude Code owns the
 * system prompt, tools, settings, memory and transcript; this class only
 * passes messages in, events out, and permission decisions back.
 */
export class LiveSession {
  id?: string;
  #env: SessionEnvironment;
  #input = new InputQueue();
  #abort = new AbortController();
  #query: Query;
  #turn?: Turn;
  #pending = new Map<string, { input: Record<string, unknown>; resolve: (result: PermissionResult) => void }>();
  #mode: FrontendPermissionMode;
  #model?: string;
  #idle?: ReturnType<typeof setTimeout>;
  #closed = false;
  #onId: (id: string) => void;
  #onClose: () => void;

  constructor(env: SessionEnvironment, options: { resume?: string; permissionMode: FrontendPermissionMode; model?: string; onId: (id: string) => void; onClose: () => void }) {
    this.#env = env;
    this.#mode = options.permissionMode;
    this.#model = options.model;
    this.#onId = options.onId;
    this.#onClose = options.onClose;
    this.id = options.resume;
    this.#query = env.query({ prompt: this.#input, options: {
      // Claude Code as it runs in a terminal or IDE: its own prompt, tools and settings.
      ...claudeCodeOptions(env),
      abortController: this.#abort,
      permissionMode: options.permissionMode,
      ...(options.model ? { model: options.model } : {}),
      canUseTool: this.#canUseTool,
      includePartialMessages: true,
      ...(options.resume ? { resume: options.resume } : {}),
    } });
    void this.#pump();
  }

  get busy(): boolean { return this.#turn !== undefined; }
  get closed(): boolean { return this.#closed; }

  /** Sends one message; resolves when Claude Code reports the turn's result. */
  async runTurn(message: string, permissionMode: FrontendPermissionMode | undefined, model: string | undefined, emit: Emit): Promise<void> {
    if (this.#closed) throw new SessionClosedError("session is closed");
    if (this.#turn) throw new SessionBusyError("a turn is already running");
    clearTimeout(this.#idle);
    const done = new Promise<void>((resolve) => { this.#turn = { emit, finish: resolve, interrupted: false }; });
    if (permissionMode && permissionMode !== this.#mode) {
      this.#mode = permissionMode;
      await this.#query.setPermissionMode(permissionMode);
    }
    // Like /model in a terminal: the session continues on the chosen model.
    if (model && model !== this.#model) {
      this.#model = model;
      await this.#query.setModel(model);
    }
    this.#input.push(message);
    return done;
  }

  async interrupt(): Promise<boolean> {
    if (!this.#turn) return false;
    this.#turn.interrupted = true;
    this.#denyPending("The user interrupted this turn.");
    await this.#query.interrupt();
    return true;
  }

  decide(requestId: string, decision: Decision): boolean {
    const pending = this.#pending.get(requestId);
    if (!pending) return false;
    this.#pending.delete(requestId);
    pending.resolve(decision.behavior === "allow"
      ? { behavior: "allow", updatedInput: decision.updatedInput ?? pending.input }
      : { behavior: "deny", message: decision.message ?? "The user denied this action." });
    return true;
  }

  close(): void {
    if (this.#closed) return;
    this.#closed = true;
    clearTimeout(this.#idle);
    this.#denyPending("The session was closed.");
    this.#input.close();
    this.#abort.abort();
    this.#onClose();
  }

  // Claude Code's permission rules run first; only calls it would prompt for
  // in a terminal arrive here, and the user answers them in ChenWeb.
  #canUseTool: CanUseTool = (toolName, input, options) => {
    const turn = this.#turn;
    if (!turn) return Promise.resolve({ behavior: "deny", message: "No one is attending this session." });
    const requestId = options.toolUseID;
    return new Promise<PermissionResult>((resolve) => {
      this.#pending.set(requestId, { input, resolve });
      options.signal.addEventListener("abort", () => { if (this.#pending.delete(requestId)) resolve({ behavior: "deny", message: "The request was cancelled." }); });
      turn.emit({ type: "permission_request", requestId, toolName, input, title: options.title, description: options.description, reason: options.decisionReason });
    });
  };

  #denyPending(message: string) {
    for (const [id, pending] of this.#pending) { this.#pending.delete(id); pending.resolve({ behavior: "deny", message }); }
  }

  async #pump(): Promise<void> {
    try {
      for await (const message of this.#query) this.#handle(message);
    } catch {
      // Reported below as the end of the session.
    }
    if (this.#turn) {
      this.#turn.emit({ type: "error", message: this.#closed ? "The session was closed." : "Claude Code stopped unexpectedly." });
      this.#endTurn();
    }
    this.close();
  }

  #handle(message: SDKMessage) {
    const emit = this.#turn?.emit;
    if (message.type === "system" && message.subtype === "init") {
      if (!this.id || this.id !== message.session_id) { this.id = message.session_id; this.#onId(message.session_id); }
      emit?.({ type: "session", sessionId: message.session_id, model: message.model, cwd: message.cwd, permissionMode: message.permissionMode });
      return;
    }
    if (!emit) return;
    if (message.type === "stream_event") {
      const event = message.event;
      if (message.parent_tool_use_id === null && event.type === "content_block_delta" && event.delta.type === "text_delta" && event.delta.text) emit({ type: "text", text: event.delta.text });
      return;
    }
    if (message.type === "assistant") {
      for (const block of toBlocks(message.message.content)) {
        if (block.type === "tool_use") emit({ type: "tool_use", id: block.id, name: block.name, input: block.input, parentToolUseId: message.parent_tool_use_id });
      }
      return;
    }
    if (message.type === "user") {
      for (const block of toBlocks(message.message.content)) {
        if (block.type === "tool_result") emit({ type: "tool_result", toolUseId: block.toolUseId, isError: block.isError, text: block.text });
      }
      return;
    }
    if (message.type === "system" && message.subtype === "api_retry") {
      emit({ type: "status", status: "retrying", attempt: Number(message.attempt) || 0 });
      return;
    }
    if (message.type === "result") {
      const u = message.usage;
      const status = this.#turn?.interrupted ? "interrupted" : message.subtype === "success" && !message.is_error ? "completed" : message.subtype === "error_max_turns" ? "max_turns" : "error";
      emit({ type: "result", status, sessionId: message.session_id, durationMs: message.duration_ms, numTurns: message.num_turns, costUsd: message.total_cost_usd,
        usage: { inputTokens: Number(u?.input_tokens) || 0, outputTokens: Number(u?.output_tokens) || 0, cacheReadTokens: Number(u?.cache_read_input_tokens) || 0, cacheWriteTokens: Number(u?.cache_creation_input_tokens) || 0 },
        ...(message.subtype === "success" ? { text: message.result } : {}) });
      this.#endTurn();
    }
  }

  #endTurn() {
    const turn = this.#turn;
    this.#turn = undefined;
    this.#denyPending("The turn ended.");
    turn?.finish();
    if (!this.#closed) this.#idle = setTimeout(() => this.close(), this.#env.idleMs);
  }
}
