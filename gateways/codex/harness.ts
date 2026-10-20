import type { Connect, RequestId, RpcConnection } from "./appserver.ts";
import { historyMessages, toolCall, toolResult, type CodexItem } from "./items.ts";
import type { Decision, FrontendPermissionMode, HarnessEvent, HistoryMessage, ModelOption, TurnRequest, Usage } from "./protocol.ts";

export type Emit = (event: HarnessEvent) => void;

export interface HarnessEnvironment {
  connect: Connect;
  /** The workspace folder Codex runs in. */
  cwd: string;
  /** A session with no turn for this long is unloaded from the app-server; it can be resumed later. */
  idleMs: number;
}

export class SessionBusyError extends Error {}

export interface SessionSummary { sessionId: string; title: string; firstPrompt: string; lastModified: number; gitBranch?: string; live: boolean }

interface Turn {
  id?: string;
  emit: Emit;
  finish: () => void;
  interrupted: boolean;
  startedAt: number;
  items: Map<string, CodexItem>;
  /** The agent message whose text was streamed last, so separate messages get a paragraph break. */
  textItem?: string;
  lastWasText: boolean;
  finalText?: string;
  usage: Usage;
  retries: number;
  pending: Map<string, { rpcId: RequestId; resolve: (decision: Decision) => void }>;
}

interface Thread { id: string; model?: string; mode: FrontendPermissionMode; defaultPolicy?: unknown; turn?: Turn; idle?: ReturnType<typeof setTimeout> }

// Codex's sandbox for each front-end mode. "default" leaves Codex's own
// configuration (config.toml, project trust) in charge.
const SANDBOX_MODE: Record<FrontendPermissionMode, string | undefined> = { default: undefined, acceptEdits: "workspace-write", plan: "read-only" };
const SANDBOX_POLICY: Record<FrontendPermissionMode, unknown> = {
  default: undefined,
  acceptEdits: { type: "workspaceWrite", writableRoots: [], networkAccess: false, excludeTmpdirEnvVar: false, excludeSlashTmp: false },
  plan: { type: "readOnly", networkAccess: false },
};
// Sessions started in a terminal, VS Code or through an app-server client such as this gateway.
const SOURCE_KINDS = ["cli", "vscode", "appServer"];
const emptyUsage = (): Usage => ({ inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 });

/**
 * ChenWeb's view of Codex. Codex owns the instructions, tools, sandbox,
 * approval rules, memory and thread history; this class only passes messages
 * in, events out, and the user's approvals back.
 */
export class CodexHarness {
  #env: HarnessEnvironment;
  #connection?: Promise<RpcConnection>;
  #threads = new Map<string, Thread>();
  #busy = new Set<string>();
  #requests = 0;

  constructor(env: HarnessEnvironment) { this.#env = env; }

  get cwd(): string { return this.#env.cwd; }
  isLive(id: string): boolean { return this.#threads.has(id); }
  isBusy(id: string): boolean { return this.#busy.has(id); }

  /** The models Codex offers this account, as its model picker lists them; its default first. */
  async models(): Promise<ModelOption[]> {
    const conn = await this.#connect();
    const models: { model: string; displayName: string; description: string; hidden: boolean; isDefault: boolean }[] = [];
    let cursor: string | null = null;
    do {
      const page: { data: typeof models; nextCursor: string | null } = await conn.request("model/list", { cursor, limit: 100 });
      models.push(...page.data);
      cursor = page.nextCursor;
    } while (cursor);
    return models.filter((m) => !m.hidden).sort((a, b) => Number(b.isDefault) - Number(a.isDefault))
      .map((m) => ({ value: m.model, resolvedModel: m.model, displayName: m.displayName, description: m.description }));
  }

  /** Codex's threads in the workspace folder, most recently updated first. */
  async listSessions(limit: number, offset: number): Promise<SessionSummary[]> {
    const conn = await this.#connect();
    const threads: any[] = [];
    let cursor: string | null = null;
    do {
      const page: { data: any[]; nextCursor: string | null } = await conn.request("thread/list", { cursor, limit: Math.min(offset + limit, 100), sortKey: "updated_at", cwd: this.#env.cwd, sourceKinds: SOURCE_KINDS });
      threads.push(...page.data);
      cursor = page.nextCursor;
    } while (cursor && threads.length < offset + limit);
    return threads.slice(offset, offset + limit).map((t) => ({
      sessionId: t.id, title: t.name || t.preview, firstPrompt: t.preview, lastModified: t.updatedAt * 1000,
      ...(t.gitInfo?.branch ? { gitBranch: t.gitInfo.branch } : {}), live: this.#threads.has(t.id),
    }));
  }

  /** Whether a thread belongs to the workspace folder, so it may be read or resumed. */
  async known(id: string): Promise<boolean> {
    if (this.#threads.has(id)) return true;
    const thread = await this.#read(id, false);
    return thread !== undefined && thread.cwd === this.#env.cwd;
  }

  async history(id: string, limit: number, offset: number): Promise<HistoryMessage[] | undefined> {
    const thread = await this.#read(id, true);
    if (!thread || thread.cwd !== this.#env.cwd) return undefined;
    return historyMessages(thread.turns ?? []).slice(offset, offset + limit);
  }

  async #read(id: string, includeTurns: boolean): Promise<any | undefined> {
    const conn = await this.#connect();
    try { return (await conn.request("thread/read", { threadId: id, includeTurns })).thread; } catch { return undefined; }
  }

  /**
   * Runs one turn: starts or resumes the thread, sends the message, and
   * resolves when Codex reports the turn completed. Aborting the signal
   * interrupts the turn.
   */
  async runTurn(request: TurnRequest, emit: Emit, signal: AbortSignal): Promise<void> {
    const mode = request.permissionMode ?? "default";
    let id = request.sessionId;
    if (id) {
      if (this.#busy.has(id)) throw new SessionBusyError("a turn is already running");
      this.#busy.add(id);
    }
    try {
      const conn = await this.#connect();
      let thread = id ? this.#threads.get(id) : undefined;
      let model: string | undefined;
      let policy: unknown;
      if (!thread) {
        const sandbox = SANDBOX_MODE[mode];
        const params = { ...(request.model ? { model: request.model } : {}), ...(sandbox ? { sandbox } : {}) };
        const started = id
          ? await conn.request("thread/resume", { threadId: id, ...params })
          : await conn.request("thread/start", { cwd: this.#env.cwd, ...params });
        id = started.thread.id as string;
        this.#busy.add(id);
        thread = { id, model: started.model, mode, defaultPolicy: mode === "default" ? started.sandbox : undefined };
        this.#threads.set(id, thread);
      } else {
        // Like changing the model or sandbox in Codex's own UI: this turn and later ones use it.
        if (request.model && request.model !== thread.model) model = thread.model = request.model;
        if (mode !== thread.mode) { policy = SANDBOX_POLICY[mode] ?? thread.defaultPolicy; thread.mode = mode; }
      }
      clearTimeout(thread.idle);
      emit({ type: "session", sessionId: thread.id, model: thread.model ?? "", cwd: this.#env.cwd, permissionMode: mode });

      const done = new Promise<void>((resolve) => {
        thread.turn = { emit, finish: resolve, interrupted: false, startedAt: Date.now(), items: new Map(), lastWasText: false, usage: emptyUsage(), retries: 0, pending: new Map() };
      });
      const turn = thread.turn!;
      const onAbort = () => void this.#interrupt(thread, turn);
      signal.addEventListener("abort", onAbort);
      try {
        const started = await conn.request("turn/start", {
          threadId: id, input: [{ type: "text", text: request.message, text_elements: [] }],
          ...(model ? { model } : {}), ...(policy ? { sandboxPolicy: policy } : {}),
        });
        turn.id ??= started.turn.id;
        if (turn.interrupted) void conn.request("turn/interrupt", { threadId: id, turnId: turn.id }).catch(() => undefined);
      } catch (error) {
        if (thread.turn === turn) thread.turn = undefined;
        throw error;
      }
      await done;
      signal.removeEventListener("abort", onAbort);
    } finally {
      if (id) this.#busy.delete(id);
    }
  }

  async interrupt(id: string): Promise<boolean> {
    const thread = this.#threads.get(id);
    if (!thread?.turn) return false;
    await this.#interrupt(thread, thread.turn);
    return true;
  }

  async #interrupt(thread: Thread, turn: Turn) {
    if (turn.interrupted || thread.turn !== turn) return;
    turn.interrupted = true;
    this.#denyPending(turn, "The user interrupted this turn.");
    // Before turn/start answers, the turn has no ID yet; runTurn interrupts it then.
    if (turn.id) await (await this.#connect()).request("turn/interrupt", { threadId: thread.id, turnId: turn.id }).catch(() => undefined);
  }

  decide(id: string, requestId: string, decision: Decision): boolean {
    const pending = this.#threads.get(id)?.turn?.pending.get(requestId);
    if (!pending) return false;
    this.#threads.get(id)!.turn!.pending.delete(requestId);
    pending.resolve(decision);
    return true;
  }

  close(): void {
    void this.#connection?.then((conn) => conn.close(), () => undefined);
  }

  #connect(): Promise<RpcConnection> {
    this.#connection ??= this.#env.connect({
      notification: (method, params) => this.#notification(method, params),
      request: (method, params, rpcId) => this.#serverRequest(method, params, rpcId),
      closed: () => this.#lost(),
    }).catch((error) => { this.#connection = undefined; throw error; });
    return this.#connection;
  }

  // The app-server exited: running turns end, and every thread must be resumed again.
  #lost() {
    this.#connection = undefined;
    for (const thread of this.#threads.values()) {
      clearTimeout(thread.idle);
      if (thread.turn) {
        thread.turn.emit({ type: "error", message: "Codex stopped unexpectedly." });
        this.#endTurn(thread, false);
      }
    }
    this.#threads.clear();
  }

  #turnOf(params: any): { thread: Thread; turn: Turn } | undefined {
    const thread = this.#threads.get(params?.threadId);
    const turn = thread?.turn;
    if (!thread || !turn) return undefined;
    const turnId = params.turnId ?? params.turn?.id;
    if (turnId && turn.id && turnId !== turn.id) return undefined;
    return { thread, turn };
  }

  #notification(method: string, params: any) {
    if (method === "thread/closed") {
      const thread = this.#threads.get(params?.threadId);
      if (thread && !thread.turn) { clearTimeout(thread.idle); this.#threads.delete(thread.id); }
      return;
    }
    if (method === "serverRequest/resolved") {
      // Codex settled a request itself (for example when the turn ended); drop its card's pending state.
      const turn = this.#threads.get(params?.threadId)?.turn;
      for (const [id, pending] of turn?.pending ?? []) {
        if (pending.rpcId === params.requestId) { turn!.pending.delete(id); pending.resolve({ behavior: "deny" }); }
      }
      return;
    }
    const target = this.#turnOf(params);
    if (!target) return;
    const { thread, turn } = target;
    switch (method) {
      case "turn/started":
        turn.id ??= params.turn.id;
        return;
      case "item/agentMessage/delta":
        if (!params.delta) return;
        // Codex can send several agent messages in one turn; keep them apart.
        if (turn.lastWasText && turn.textItem !== params.itemId) turn.emit({ type: "text", text: "\n\n" });
        turn.textItem = params.itemId;
        turn.lastWasText = true;
        turn.emit({ type: "text", text: params.delta });
        return;
      case "item/started": {
        const item = params.item as CodexItem;
        turn.items.set(item.id, item);
        const call = toolCall(item);
        if (call) { turn.lastWasText = false; turn.emit({ type: "tool_use", ...call, parentToolUseId: null }); }
        return;
      }
      case "item/completed": {
        const item = params.item as CodexItem;
        turn.items.set(item.id, item);
        if (item.type === "agentMessage") {
          turn.finalText = item.text;
          if (turn.textItem !== item.id && item.text) {
            // Not streamed as deltas: send it whole.
            if (turn.lastWasText) turn.emit({ type: "text", text: "\n\n" });
            turn.textItem = item.id;
            turn.lastWasText = true;
            turn.emit({ type: "text", text: item.text });
          }
          return;
        }
        const result = toolResult(item);
        if (result) turn.emit({ type: "tool_result", toolUseId: item.id, ...result });
        return;
      }
      case "thread/tokenUsage/updated": {
        // `last` is one model request's usage; a turn is the sum of its requests.
        const last = params.tokenUsage?.last ?? {};
        turn.usage.inputTokens += Number(last.inputTokens) || 0;
        turn.usage.outputTokens += Number(last.outputTokens) || 0;
        turn.usage.cacheReadTokens += Number(last.cachedInputTokens) || 0;
        turn.usage.cacheWriteTokens += Number(last.cacheWriteInputTokens) || 0;
        return;
      }
      case "error":
        if (params.willRetry) turn.emit({ type: "status", status: "retrying", attempt: ++turn.retries });
        return;
      case "turn/completed": {
        const t = params.turn;
        const status = turn.interrupted || t.status === "interrupted" ? "interrupted" : t.status === "completed" ? "completed" : "error";
        if (status === "error" && t.error?.message) turn.emit({ type: "error", message: String(t.error.message) });
        turn.emit({ type: "result", status, sessionId: thread.id, durationMs: t.durationMs ?? Date.now() - turn.startedAt, numTurns: 1, costUsd: 0, usage: turn.usage,
          ...(status === "completed" && turn.finalText !== undefined ? { text: turn.finalText } : {}) });
        this.#endTurn(thread, true);
        return;
      }
    }
  }

  // Codex's approval rules run first; only what it would ask about in its own
  // UI arrives here, and the user answers in ChenWeb.
  async #serverRequest(method: string, params: any, rpcId: RequestId): Promise<unknown> {
    const target = this.#turnOf(params);
    const ask = (request: { toolName: string; input: unknown; title?: string; description?: string; reason?: string }): Promise<Decision> => {
      if (!target) return Promise.resolve({ behavior: "deny", message: "No one is attending this session." });
      const requestId = `codex-${++this.#requests}`;
      return new Promise((resolve) => {
        target.turn.pending.set(requestId, { rpcId, resolve });
        target.turn.emit({ type: "permission_request", requestId, ...request });
      });
    };
    const reason = params?.reason ?? undefined;
    switch (method) {
      case "item/commandExecution/requestApproval": {
        const decision = await ask({ toolName: "Shell", input: { command: params.command ?? target?.turn.items.get(params.itemId)?.command, cwd: params.cwd ?? undefined }, reason });
        return { decision: decision.behavior === "allow" ? "accept" : "decline" };
      }
      case "item/fileChange/requestApproval": {
        const item = target?.turn.items.get(params.itemId);
        const input = item ? toolCall(item)!.input : { path: params.grantRoot ?? "" };
        const decision = await ask({ toolName: "Edit", input, reason });
        return { decision: decision.behavior === "allow" ? "accept" : "decline" };
      }
      case "item/permissions/requestApproval": {
        const decision = await ask({ toolName: "Permissions", input: { cwd: params.cwd, permissions: params.permissions }, reason });
        const granted = decision.behavior === "allow" ? Object.fromEntries(Object.entries(params.permissions ?? {}).filter(([, v]) => v !== null)) : {};
        return { permissions: granted, scope: "turn" };
      }
      case "item/tool/requestUserInput": {
        // Shown like Claude Code's AskUserQuestion; ChenWeb answers by question text.
        const questions = (params.questions ?? []) as { id: string; header: string; question: string; options: { label: string; description: string }[] | null }[];
        const decision = await ask({ toolName: "AskUserQuestion", input: { questions: questions.map((q) => ({ question: q.question, header: q.header, multiSelect: false, options: q.options ?? [] })) } });
        const answers = (decision.behavior === "allow" ? decision.updatedInput?.answers : undefined) as Record<string, unknown> | undefined;
        return { answers: Object.fromEntries(questions.flatMap((q) => (typeof answers?.[q.question] === "string" && answers[q.question] ? [[q.id, { answers: [answers[q.question]] }]] : []))) };
      }
      case "mcpServer/elicitation/request":
        // MCP forms and links cannot be filled in from ChenWeb yet.
        return { action: "decline", content: null, _meta: null };
      default:
        throw new Error(`${method} is not supported by the ChenWeb gateway`);
    }
  }

  #denyPending(turn: Turn, message: string) {
    for (const [id, pending] of turn.pending) { turn.pending.delete(id); pending.resolve({ behavior: "deny", message }); }
  }

  #endTurn(thread: Thread, keep: boolean) {
    const turn = thread.turn;
    thread.turn = undefined;
    if (!turn) return;
    this.#denyPending(turn, "The turn ended.");
    turn.finish();
    if (keep) thread.idle = setTimeout(() => this.#unload(thread), this.#env.idleMs);
  }

  #unload(thread: Thread) {
    if (thread.turn || this.#threads.get(thread.id) !== thread) return;
    this.#threads.delete(thread.id);
    void this.#connection?.then((conn) => conn.request("thread/unsubscribe", { threadId: thread.id })).catch(() => undefined);
  }
}
