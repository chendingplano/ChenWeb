import type { Connect, RequestId, RpcConnection } from "./acp.ts";
import { RpcError } from "./acp.ts";
import { capped, type Decision, type HarnessEvent, type HistoryMessage, type ModelOption, type TurnRequest, type Usage } from "./protocol.ts";
import { conversed, historyMessages, inWorkspace, readLog, summary, UnsupportedLogError, type LogHeader } from "./sessionlog.ts";

export type Emit = (event: HarnessEvent) => void;

export interface HarnessEnvironment {
  connect: Connect;
  /** The workspace folder dsh runs in. */
  cwd: string;
  /** dsh's session-log root (`$DSH_HOME/sessions`, by default `~/.dsh/sessions`). */
  sessionsRoot: string;
  /** A session with no turn for this long is closed in dsh; it can be resumed later. */
  idleMs: number;
}

export class SessionBusyError extends Error {}

export interface SessionSummary { sessionId: string; title: string; firstPrompt: string; lastModified: number; live: boolean }

/** One model choice of an ACP session: dsh's opaque option value and the protocol value ChenWeb uses. */
interface ModelChoice { raw: string; value: string; provider: string; model: string; name: string; description: string }

interface ToolCall { name: string; input: unknown; done: boolean }

interface Turn {
  emit: Emit;
  interrupted: boolean;
  startedAt: number;
  calls: Map<string, ToolCall>;
  /** The message whose text was sent last, so separate messages get a paragraph break. */
  textMessage?: string;
  lastWasText: boolean;
  finalText?: string;
  /** Open permission prompts; resolving with undefined answers dsh "cancelled". */
  pending: Map<string, (decision: Decision | undefined) => void>;
}

interface Session { id: string; models: ModelChoice[]; model?: string; turn?: Turn; idle?: ReturnType<typeof setTimeout> }

const emptyUsage = (): Usage => ({ inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 });
const NO_MCP: unknown[] = [];

/**
 * ChenWeb's view of DeepSeek Harness. dsh owns the instructions, tools,
 * sandbox, approval rules, skills and session history; this class only passes
 * messages in, updates out, and the user's approvals back.
 */
export class DeepSeekHarness {
  #env: HarnessEnvironment;
  #connection?: Promise<RpcConnection>;
  #sessions = new Map<string, Session>();
  #busy = new Set<string>();
  #requests = 0;
  #models?: Promise<ModelOption[]>;

  constructor(env: HarnessEnvironment) { this.#env = env; }

  get cwd(): string { return this.#env.cwd; }
  isLive(id: string): boolean { return this.#sessions.has(id); }
  isBusy(id: string): boolean { return this.#busy.has(id); }

  /**
   * The models dsh offers. ACP lists them only as a session's options, so this
   * opens a session and closes it again. dsh keeps that session as an empty
   * log, so the list is read once per dsh process: dsh's ACP profile reads its
   * configuration only at start, so the list cannot change while it runs.
   */
  models(): Promise<ModelOption[]> {
    this.#models ??= this.#listModels().catch((error) => { this.#models = undefined; throw error; });
    return this.#models;
  }

  async #listModels(): Promise<ModelOption[]> {
    const conn = await this.#connect();
    const opened = await conn.request("session/new", { cwd: this.#env.cwd, mcpServers: NO_MCP });
    void conn.request("session/close", { sessionId: opened.sessionId }).catch(() => undefined);
    const { choices, current } = modelChoices(opened.configOptions);
    return choices.sort((a, b) => Number(b.raw === current) - Number(a.raw === current))
      .map((c) => ({ value: c.value, resolvedModel: c.model, displayName: c.name, description: c.description }));
  }

  /** dsh's root sessions in the workspace folder that have a conversation, most recent first. */
  async listSessions(limit: number, offset: number): Promise<SessionSummary[]> {
    const conn = await this.#connect();
    // dsh lists only sessions not open on this connection; open ones are the most recent.
    const ids = [...this.#sessions.keys()].reverse();
    let cursor: string | undefined;
    do {
      const page: { sessions: { sessionId: string }[]; nextCursor?: string | null } = await conn.request("session/list", { cwd: this.#env.cwd, ...(cursor ? { cursor } : {}) });
      ids.push(...page.sessions.map((s) => s.sessionId).filter((id) => !this.#sessions.has(id)));
      cursor = page.nextCursor ?? undefined;
    } while (cursor && ids.length < offset + limit);
    const sessions: SessionSummary[] = [];
    for (const id of ids) {
      let described = { title: "", firstPrompt: "", lastModified: 0 };
      try {
        const log = readLog(this.#env.sessionsRoot, id);
        // Not the workspace's, or an empty session (such as the one opened to list models).
        if (log && (!inWorkspace(log.header, this.#env.cwd) || !conversed(log))) continue;
        if (log) described = summary(log);
      } catch { /* listed by dsh, but its log cannot be read here: keep it without a title */ }
      sessions.push({ sessionId: id, ...described, live: this.#sessions.has(id) });
      if (sessions.length >= offset + limit) break;
    }
    return sessions.slice(offset);
  }

  /** Whether a session belongs to the workspace folder, so it may be read or resumed. */
  known(id: string): boolean {
    if (this.#sessions.has(id)) return true;
    const header = this.#header(id);
    return header !== undefined && inWorkspace(header, this.#env.cwd);
  }

  /** A session's history from dsh's log; undefined outside the workspace. Throws UnsupportedLogError. */
  history(id: string, limit: number, offset: number): HistoryMessage[] | undefined {
    const log = readLog(this.#env.sessionsRoot, id);
    if (!log || !inWorkspace(log.header, this.#env.cwd)) return undefined;
    return historyMessages(log).slice(offset, offset + limit);
  }

  #header(id: string): LogHeader | undefined {
    try { return readLog(this.#env.sessionsRoot, id)?.header; } catch (error) { return error instanceof UnsupportedLogError ? error.header : undefined; }
  }

  /**
   * Runs one turn: creates or resumes the session, sends the message, and
   * resolves when dsh answers the prompt. Aborting the signal cancels the turn.
   */
  async runTurn(request: TurnRequest, emit: Emit, signal: AbortSignal): Promise<void> {
    if (request.permissionMode && request.permissionMode !== "default") {
      // dsh's ACP server has no modes; running a "plan" turn with write access would be wrong.
      emit({ type: "error", message: `DeepSeek Harness has no "${request.permissionMode}" mode here; its own permission settings apply in the default mode.` });
      return;
    }
    let id = request.sessionId;
    if (id) {
      if (this.#busy.has(id)) throw new SessionBusyError("a turn is already running");
      this.#busy.add(id);
    }
    try {
      const conn = await this.#connect();
      let session = id ? this.#sessions.get(id) : undefined;
      if (!session) {
        const opened = id
          ? await conn.request("session/resume", { sessionId: id, cwd: this.#env.cwd, mcpServers: NO_MCP })
          : await conn.request("session/new", { cwd: this.#env.cwd, mcpServers: NO_MCP });
        id ??= opened.sessionId as string;
        this.#busy.add(id);
        const { choices, current } = modelChoices(opened.configOptions);
        session = { id, models: choices, model: current };
        this.#sessions.set(id, session);
      }
      clearTimeout(session.idle);
      if (request.model) {
        // Like choosing a model in dsh's own picker: this turn and later ones use it.
        const choice = session.models.find((c) => c.value === request.model);
        if (!choice) {
          emit({ type: "error", message: `DeepSeek Harness does not offer the model ${request.model}.` });
          this.#idle(session);
          return;
        }
        if (choice.raw !== session.model) {
          const updated = await conn.request("session/set_config_option", { sessionId: id, configId: "model", value: choice.raw });
          const next = modelChoices(updated.configOptions);
          if (next.choices.length) session.models = next.choices;
          session.model = next.current ?? choice.raw;
        }
      }
      const model = session.models.find((c) => c.raw === session.model);
      emit({ type: "session", sessionId: session.id, model: model?.value ?? "", cwd: this.#env.cwd, permissionMode: "default" });

      const turn: Turn = { emit, interrupted: false, startedAt: Date.now(), calls: new Map(), lastWasText: false, pending: new Map() };
      session.turn = turn;
      const onAbort = () => this.#interrupt(session, turn);
      signal.addEventListener("abort", onAbort);
      let status: "completed" | "interrupted" | "max_turns" | "error" = "error";
      try {
        const answer: { stopReason?: string } = await conn.request("session/prompt", { sessionId: id, prompt: [{ type: "text", text: request.message }] });
        status = turn.interrupted || answer.stopReason === "cancelled" ? "interrupted"
          : answer.stopReason === "end_turn" ? "completed"
          : answer.stopReason === "max_turn_requests" ? "max_turns" : "error";
        if (answer.stopReason === "max_tokens") emit({ type: "error", message: "The reply was cut off at the model's output limit." });
        if (answer.stopReason === "refusal") emit({ type: "error", message: "The model refused to continue." });
      } catch (error) {
        if (turn.interrupted) status = "interrupted";
        else emit({ type: "error", message: error instanceof Error ? error.message : "The turn failed." });
      } finally {
        signal.removeEventListener("abort", onAbort);
      }
      // ACP carries no token counts or cost for a turn.
      emit({ type: "result", status, sessionId: session.id, durationMs: Date.now() - turn.startedAt, numTurns: 1, costUsd: 0, usage: emptyUsage(),
        ...(status === "completed" && turn.finalText !== undefined ? { text: turn.finalText } : {}) });
      if (session.turn === turn) this.#endTurn(session);
    } finally {
      if (id) this.#busy.delete(id);
    }
  }

  interrupt(id: string): boolean {
    const session = this.#sessions.get(id);
    if (!session?.turn) return false;
    this.#interrupt(session, session.turn);
    return true;
  }

  #interrupt(session: Session, turn: Turn) {
    if (turn.interrupted || session.turn !== turn) return;
    turn.interrupted = true;
    // ACP: open permission prompts of a cancelled turn are answered "cancelled".
    this.#cancelPending(turn);
    void this.#connection?.then((conn) => conn.notify("session/cancel", { sessionId: session.id }), () => undefined);
  }

  decide(id: string, requestId: string, decision: Decision): boolean {
    const turn = this.#sessions.get(id)?.turn;
    const resolve = turn?.pending.get(requestId);
    if (!turn || !resolve) return false;
    turn.pending.delete(requestId);
    resolve(decision);
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

  // dsh exited: running turns end, and every session must be resumed again.
  #lost() {
    this.#connection = undefined;
    this.#models = undefined;
    for (const session of this.#sessions.values()) {
      clearTimeout(session.idle);
      if (session.turn) {
        session.turn.emit({ type: "error", message: "DeepSeek Harness stopped unexpectedly." });
        this.#cancelPending(session.turn);
      }
    }
    this.#sessions.clear();
  }

  #notification(method: string, params: any) {
    if (method !== "session/update") return;
    // Updates outside a ChenWeb turn (dsh's autonomous work) have no one to show them to.
    const turn = this.#sessions.get(params?.sessionId)?.turn;
    const update = params?.update;
    if (!turn || !update) return;
    switch (update.sessionUpdate) {
      case "agent_message_chunk": {
        const text = update.content?.type === "text" ? String(update.content.text ?? "") : "";
        if (!text) return;
        const same = update.messageId !== undefined && update.messageId === turn.textMessage;
        // dsh sends whole committed messages; keep separate ones apart.
        if (turn.lastWasText && !same) turn.emit({ type: "text", text: "\n\n" });
        turn.finalText = same ? `${turn.finalText ?? ""}${text}` : text;
        turn.textMessage = update.messageId;
        turn.lastWasText = true;
        turn.emit({ type: "text", text });
        return;
      }
      case "tool_call": {
        const call: ToolCall = { name: String(update.title ?? update.kind ?? "tool"), input: update.rawInput ?? {}, done: false };
        turn.calls.set(update.toolCallId, call);
        turn.lastWasText = false;
        turn.emit({ type: "tool_use", id: update.toolCallId, name: call.name, input: call.input, parentToolUseId: null });
        this.#toolUpdate(turn, update);
        return;
      }
      case "tool_call_update":
        this.#toolUpdate(turn, update);
        return;
      // Thoughts are never relayed; context usage and other updates are not shown.
      default:
        return;
    }
  }

  #toolUpdate(turn: Turn, update: any) {
    const call = turn.calls.get(update.toolCallId);
    if (!call) return;
    if (update.rawInput !== undefined) call.input = update.rawInput;
    if (update.status !== "completed" && update.status !== "failed") return;
    if (call.done) return;
    call.done = true;
    turn.emit({ type: "tool_result", toolUseId: update.toolCallId, isError: update.status === "failed", text: capped(toolText(update.content)) });
  }

  // dsh's sandbox and approval policy run first; only what it would ask about
  // in its own UI arrives here, and the user answers in ChenWeb.
  async #serverRequest(method: string, params: any, _rpcId: RequestId): Promise<unknown> {
    if (method !== "session/request_permission") throw new RpcError(-32601, `${method} is not supported by the ChenWeb gateway`);
    const turn = this.#sessions.get(params?.sessionId)?.turn;
    if (!turn || turn.interrupted) return { outcome: { outcome: "cancelled" } };
    const toolCall = params.toolCall ?? {};
    const call = turn.calls.get(toolCall.toolCallId);
    const requestId = `dsh-${++this.#requests}`;
    const decision = await new Promise<Decision | undefined>((resolve) => {
      turn.pending.set(requestId, resolve);
      turn.emit({ type: "permission_request", requestId, toolName: call?.name ?? String(toolCall.title ?? "tool"), input: call?.input ?? toolCall.rawInput ?? {},
        ...(typeof call?.input === "object" && call.input && typeof (call.input as { justification?: unknown }).justification === "string" ? { reason: (call.input as { justification: string }).justification } : {}) });
    });
    if (!decision) return { outcome: { outcome: "cancelled" } };
    // One-time choices only: an "always" answer is the user's to give in dsh itself.
    const kind = decision.behavior === "allow" ? "allow_once" : "reject_once";
    const option = ((params.options ?? []) as { optionId: string; kind: string }[]).find((o) => o.kind === kind);
    return option ? { outcome: { outcome: "selected", optionId: option.optionId } } : { outcome: { outcome: "cancelled" } };
  }

  #cancelPending(turn: Turn) {
    for (const [id, resolve] of turn.pending) { turn.pending.delete(id); resolve(undefined); }
  }

  #endTurn(session: Session) {
    const turn = session.turn;
    session.turn = undefined;
    if (turn) this.#cancelPending(turn);
    this.#idle(session);
  }

  #idle(session: Session) {
    clearTimeout(session.idle);
    session.idle = setTimeout(() => this.#unload(session), this.#env.idleMs);
  }

  #unload(session: Session) {
    if (session.turn || this.#sessions.get(session.id) !== session) return;
    this.#sessions.delete(session.id);
    void this.#connection?.then((conn) => conn.request("session/close", { sessionId: session.id })).catch(() => undefined);
  }
}

/** The `model` option of an ACP session as protocol-v1 choices (`<provider>:<model>`), and its current raw value. */
export function modelChoices(configOptions: unknown): { choices: ModelChoice[]; current?: string } {
  const option = (Array.isArray(configOptions) ? configOptions : []).find((o: any) => o?.id === "model");
  if (!option) return { choices: [] };
  const choices: ModelChoice[] = [];
  const add = (entry: any, group?: string) => {
    if (typeof entry?.value !== "string") return;
    let pair: unknown;
    try { pair = JSON.parse(entry.value); } catch { return; }
    if (!Array.isArray(pair) || pair.length !== 2 || !pair.every((p) => typeof p === "string")) return;
    const [provider, model] = pair as [string, string];
    choices.push({ raw: entry.value, value: `${provider}:${model}`, provider, model, name: String(entry.name ?? model), description: String(entry.description ?? group ?? provider) });
  };
  for (const entry of option.options ?? []) {
    if (Array.isArray(entry?.options)) for (const inner of entry.options) add(inner, entry.name);
    else add(entry);
  }
  return { choices, current: typeof option.currentValue === "string" ? option.currentValue : undefined };
}

function toolText(content: unknown): string {
  if (!Array.isArray(content)) return "";
  return content.map((part: any) => {
    if (part?.type === "content" && part.content?.type === "text") return String(part.content.text ?? "");
    if (part?.type === "diff") return `Changed ${String(part.path ?? "")}`;
    return "";
  }).filter(Boolean).join("\n");
}
