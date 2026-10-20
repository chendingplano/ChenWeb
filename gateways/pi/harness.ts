import { capped, type Decision, type HarnessEvent, type HistoryMessage, type ModelOption, type TurnRequest, type Usage } from "./protocol.ts";
import type { PiConnection, Spawn } from "./rpc.ts";
import { conversed, findSession, historyMessages, readSession, sessionFiles, summary, textOf, UnsupportedSessionError } from "./sessionfile.ts";

export type Emit = (event: HarnessEvent) => void;

export interface HarnessEnvironment {
  spawn: Spawn;
  /** The workspace folder Pi runs in. */
  cwd: string;
  /** Pi's session folder for the workspace (`<agent dir>/sessions/--<cwd>--`). */
  sessionDir: string;
  /** A session with no turn for this long has its Pi process stopped; it can be resumed later. */
  idleMs: number;
}

export class SessionBusyError extends Error {}

export interface SessionSummary { sessionId: string; title: string; firstPrompt: string; lastModified: number; live: boolean }

interface Turn {
  emit: Emit;
  interrupted: boolean;
  startedAt: number;
  /** Pi started an agent run for this prompt (an extension command may answer without one). */
  running: boolean;
  settled: PromiseWithResolvers<void>;
  /** The current assistant message has sent no text yet, so separate messages get a paragraph break. */
  newMessage: boolean;
  sentText: boolean;
  finalText?: string;
  error?: string;
  numTurns: number;
  usage: Usage;
  costUsd: number;
  /** Open extension dialogs; resolving with undefined answers Pi "cancelled". */
  pending: Map<string, (decision: Decision | undefined) => void>;
}

interface Session { id: string; conn: PiConnection; model?: string; firstPrompt?: string; lastTurn: number; turn?: Turn; idle?: ReturnType<typeof setTimeout> }

const emptyUsage = (): Usage => ({ inputTokens: 0, outputTokens: 0, cacheReadTokens: 0, cacheWriteTokens: 0 });
const MODEL = /^[A-Za-z0-9._:\[\]-]{1,128}$/;
/** Pi's `provider/id` as a protocol-v1 model value; Pi's provider names have no colon. */
export const modelValue = (provider: string, id: string) => `${provider}:${id}`;
/** Extension UI methods that expect no answer. */
const FIRE_AND_FORGET = new Set(["notify", "setStatus", "setWidget", "setTitle", "set_editor_text"]);

/**
 * ChenWeb's view of Pi. Pi owns the instructions, tools, extensions, skills,
 * settings and session files; this class only passes messages in, events
 * out, and the user's answers to Pi's extension dialogs back.
 */
export class PiHarness {
  #env: HarnessEnvironment;
  #sessions = new Map<string, Session>();
  #busy = new Set<string>();
  #requests = 0;

  constructor(env: HarnessEnvironment) { this.#env = env; }

  get cwd(): string { return this.#env.cwd; }
  isBusy(id: string): boolean { return this.#busy.has(id); }

  /** The models Pi offers (those it has a login for), Pi's default first. Starts a Pi process that saves no session. */
  async models(): Promise<ModelOption[]> {
    let conn: PiConnection | undefined;
    conn = await this.#env.spawn(["--no-session"], { event() {}, uiRequest: (request) => { if (conn) cancelDialog(conn, request); }, closed() {} });
    try {
      const [available, state] = await Promise.all([conn.command({ type: "get_available_models" }), conn.command({ type: "get_state" })]);
      const current = state?.model ? modelValue(state.model.provider, state.model.id) : undefined;
      return ((available?.models ?? []) as { provider: string; id: string; name?: string }[])
        .map((m) => ({ value: modelValue(m.provider, m.id), resolvedModel: m.id, displayName: m.name || m.id, description: m.provider }))
        // A model id with characters outside protocol v1's model pattern (such as `/`) cannot be chosen from ChenWeb.
        .filter((m) => MODEL.test(m.value))
        .sort((a, b) => Number(b.value === current) - Number(a.value === current));
    } finally {
      conn.close();
    }
  }

  /** Pi's sessions in the workspace folder that have a conversation, most recent first. Open ones come first. */
  listSessions(limit: number, offset: number): SessionSummary[] {
    const live = [...this.#sessions.values()].sort((a, b) => b.lastTurn - a.lastTurn);
    const sessions: SessionSummary[] = [];
    const listed = new Set<string>();
    const add = (id: string, path: string | undefined, fallback?: Session) => {
      let described: { title: string; firstPrompt: string; lastModified: number } | undefined;
      try {
        const file = path ? readSession(path) : undefined;
        if (file && (file.header.cwd !== this.#env.cwd || !conversed(file))) return;
        if (file) described = summary(file);
      } catch (error) {
        // In a format this gateway does not read: keep it without a title if it is the workspace's.
        if (!(error instanceof UnsupportedSessionError) || error.header.cwd !== this.#env.cwd) return;
      }
      // Pi writes a new session's file once the first reply arrives.
      if (!described && fallback && !fallback.firstPrompt) return;
      described ??= { title: fallback?.firstPrompt ?? "", firstPrompt: fallback?.firstPrompt ?? "", lastModified: fallback?.lastTurn ?? 0 };
      listed.add(id);
      sessions.push({ sessionId: id, ...described, live: this.#sessions.has(id) });
    };
    const files = sessionFiles(this.#env.sessionDir);
    for (const session of live) add(session.id, files.find((f) => f.id === session.id)?.path, session);
    for (const file of files) {
      if (sessions.length >= offset + limit) break;
      if (!listed.has(file.id)) add(file.id, file.path);
    }
    return sessions.slice(offset, offset + limit);
  }

  /** Whether a session belongs to the workspace folder, so it may be read or resumed. */
  known(id: string): boolean {
    if (this.#sessions.has(id)) return true;
    return this.#file(id) !== undefined;
  }

  /** The workspace's session file for an id; throws UnsupportedSessionError. */
  #file(id: string) {
    const path = findSession(this.#env.sessionDir, id);
    if (!path) return undefined;
    let file;
    try { file = readSession(path); } catch (error) {
      if (error instanceof UnsupportedSessionError && error.header.cwd === this.#env.cwd) throw error;
      return undefined;
    }
    return file && file.header.cwd === this.#env.cwd && file.header.id === id ? file : undefined;
  }

  /** A session's history from Pi's session file; undefined outside the workspace. Throws UnsupportedSessionError. */
  history(id: string, limit: number, offset: number): HistoryMessage[] | undefined {
    const file = this.#file(id);
    if (!file) return this.#sessions.has(id) ? [] : undefined;
    return historyMessages(file).slice(offset, offset + limit);
  }

  /**
   * Runs one turn: starts or resumes the session's Pi process, sends the
   * message, and resolves when Pi's run has settled. Aborting the signal
   * aborts the run.
   */
  async runTurn(request: TurnRequest, emit: Emit, signal: AbortSignal): Promise<void> {
    if (request.permissionMode && request.permissionMode !== "default") {
      // Pi has no permission modes; running a "plan" turn with full tools would be wrong.
      emit({ type: "error", message: `Pi has no "${request.permissionMode}" mode; only the default mode runs.` });
      return;
    }
    let id = request.sessionId;
    if (id) {
      if (this.#busy.has(id)) throw new SessionBusyError("a turn is already running");
      this.#busy.add(id);
    }
    try {
      let session = id ? this.#sessions.get(id) : undefined;
      if (!session) {
        const path = id ? this.#file(id)?.path : undefined;
        if (id && !path) { emit({ type: "error", message: "The session's file was not found." }); return; }
        session = await this.#open(path);
        id = session.id;
        this.#busy.add(id);
      }
      clearTimeout(session.idle);
      session.lastTurn = Date.now();
      session.firstPrompt ??= request.message;
      if (request.model && request.model !== session.model) {
        // Like choosing a model in Pi's own /model picker, which also makes it Pi's default.
        const split = request.model.indexOf(":");
        try {
          const model = await session.conn.command({ type: "set_model", provider: request.model.slice(0, split), modelId: request.model.slice(split + 1) });
          session.model = modelValue(model.provider, model.id);
        } catch {
          emit({ type: "error", message: `Pi does not offer the model ${request.model}.` });
          this.#idle(session);
          return;
        }
      }
      emit({ type: "session", sessionId: session.id, model: session.model ?? "", cwd: this.#env.cwd, permissionMode: "default" });

      const turn: Turn = { emit, interrupted: false, startedAt: Date.now(), running: false, settled: Promise.withResolvers(), newMessage: true, sentText: false,
        numTurns: 0, usage: emptyUsage(), costUsd: 0, pending: new Map() };
      session.turn = turn;
      const onAbort = () => this.#interrupt(session, turn);
      signal.addEventListener("abort", onAbort);
      try {
        await session.conn.command({ type: "prompt", message: request.message });
        // An extension command or input handler can answer without a run; Pi has finished it by now.
        const state = await session.conn.command({ type: "get_state" });
        if (turn.running || state?.isStreaming || state?.isCompacting) await turn.settled.promise;
      } catch (error) {
        if (!turn.interrupted) turn.error ??= error instanceof Error ? error.message : "The turn failed.";
      } finally {
        signal.removeEventListener("abort", onAbort);
      }
      const status = turn.interrupted ? "interrupted" : turn.error !== undefined ? "error" : "completed";
      if (status === "error") emit({ type: "error", message: turn.error! });
      emit({ type: "result", status, sessionId: session.id, durationMs: Date.now() - turn.startedAt, numTurns: turn.numTurns, costUsd: turn.costUsd, usage: turn.usage,
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
    this.#cancelPending(turn);
    void session.conn.command({ type: "abort" }).catch(() => undefined);
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
    for (const session of this.#sessions.values()) { clearTimeout(session.idle); session.conn.close(); }
    this.#sessions.clear();
  }

  /** Starts a Pi process for a new session, or for an existing session file. */
  async #open(path: string | undefined): Promise<Session> {
    let session: Session | undefined;
    let conn: PiConnection | undefined;
    conn = await this.#env.spawn(path ? ["--session", path] : [], {
      event: (event) => { if (session) this.#event(session, event); },
      uiRequest: (request) => { if (session) void this.#dialog(session, request); else if (conn) cancelDialog(conn, request); },
      closed: () => { if (session) this.#lost(session); },
    });
    let state;
    try { state = await conn.command({ type: "get_state" }); } catch (error) { conn.close(); throw error; }
    session = { id: String(state.sessionId), conn, model: state.model ? modelValue(state.model.provider, state.model.id) : undefined, lastTurn: Date.now() };
    this.#sessions.set(session.id, session);
    return session;
  }

  // The Pi process exited: its running turn ends; the session can be resumed from its file.
  #lost(session: Session) {
    if (this.#sessions.get(session.id) !== session) return;
    this.#sessions.delete(session.id);
    clearTimeout(session.idle);
    const turn = session.turn;
    if (turn) {
      if (!turn.interrupted) turn.error ??= "Pi stopped unexpectedly.";
      this.#cancelPending(turn);
      turn.settled.resolve();
    }
  }

  #event(session: Session, event: any) {
    // Events outside a ChenWeb turn have no one to show them to.
    const turn = session.turn;
    if (!turn) return;
    switch (event.type) {
      case "agent_start":
        turn.running = true;
        return;
      case "agent_settled":
        turn.settled.resolve();
        return;
      case "turn_end":
        turn.numTurns += 1;
        return;
      case "message_start":
        if (event.message?.role === "assistant") turn.newMessage = true;
        return;
      case "message_update": {
        // Thinking deltas are never relayed; tool calls are shown when they run.
        const delta = event.assistantMessageEvent;
        if (delta?.type !== "text_delta" || typeof delta.delta !== "string" || !delta.delta) return;
        if (turn.newMessage && turn.sentText) turn.emit({ type: "text", text: "\n\n" });
        turn.newMessage = false;
        turn.sentText = true;
        turn.emit({ type: "text", text: delta.delta });
        return;
      }
      case "message_end": {
        const message = event.message;
        if (message?.role !== "assistant") return;
        const text = textOf(message.content);
        if (text) turn.finalText = text;
        const usage = message.usage ?? {};
        turn.usage.inputTokens += Number(usage.input) || 0;
        turn.usage.outputTokens += Number(usage.output) || 0;
        turn.usage.cacheReadTokens += Number(usage.cacheRead) || 0;
        turn.usage.cacheWriteTokens += Number(usage.cacheWrite) || 0;
        turn.costUsd += Number(usage.cost?.total) || 0;
        if (message.stopReason === "error") turn.error = String(message.errorMessage || "The model call failed.");
        else if (message.stopReason === "length") turn.error = "The reply was cut off at the model's output limit.";
        // A later message (after an automatic retry) that succeeds clears an earlier failure.
        else if (message.stopReason !== "aborted") turn.error = undefined;
        return;
      }
      case "tool_execution_start":
        turn.newMessage = true;
        turn.emit({ type: "tool_use", id: String(event.toolCallId), name: String(event.toolName), input: event.args ?? {}, parentToolUseId: null });
        return;
      case "tool_execution_end":
        turn.emit({ type: "tool_result", toolUseId: String(event.toolCallId), isError: event.isError === true, text: capped(textOf(event.result?.content)) });
        return;
      case "auto_retry_start":
        turn.emit({ type: "status", status: "retrying", attempt: Number(event.attempt) || 1 });
        return;
      default:
        return;
    }
  }

  // Pi has no approval prompts of its own; an extension that asks the user
  // (`ctx.ui.confirm` or `ctx.ui.select`) is answered in ChenWeb.
  async #dialog(session: Session, request: any) {
    if (FIRE_AND_FORGET.has(request.method)) return;
    const turn = session.turn;
    // Free-text dialogs (input, editor) cannot be answered from ChenWeb yet.
    if (!turn || turn.interrupted || (request.method !== "confirm" && request.method !== "select")) { cancelDialog(session.conn, request); return; }
    const requestId = `pi-${++this.#requests}`;
    const options = Array.isArray(request.options) ? request.options.filter((o: unknown): o is string => typeof o === "string") : [];
    const title = String(request.title ?? "");
    const decision = await new Promise<Decision | undefined>((resolve) => {
      turn.pending.set(requestId, resolve);
      // Pi answers a timed-out dialog itself; the card's late answer is then ignored.
      if (Number(request.timeout) > 0) setTimeout(() => { if (turn.pending.delete(requestId)) resolve(undefined); }, Number(request.timeout));
      if (request.method === "confirm") {
        turn.emit({ type: "permission_request", requestId, toolName: "confirm", input: { description: String(request.message ?? "") }, title, description: String(request.message ?? "") });
      } else {
        // Shown like Claude Code's AskUserQuestion; ChenWeb answers by question text.
        turn.emit({ type: "permission_request", requestId, toolName: "AskUserQuestion",
          input: { questions: [{ question: title || "Choose an option", header: "Pi", multiSelect: false, options: options.map((label: string) => ({ label, description: "" })) }] } });
      }
    });
    if (!decision) { cancelDialog(session.conn, request); return; }
    if (request.method === "confirm") { session.conn.send({ type: "extension_ui_response", id: request.id, confirmed: decision.behavior === "allow" }); return; }
    const answers = (decision.behavior === "allow" ? decision.updatedInput?.answers : undefined) as Record<string, unknown> | undefined;
    const value = answers?.[title || "Choose an option"];
    if (typeof value === "string" && options.includes(value)) session.conn.send({ type: "extension_ui_response", id: request.id, value });
    else cancelDialog(session.conn, request);
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
    session.conn.close();
  }
}

function cancelDialog(conn: PiConnection, request: any) {
  if (!FIRE_AND_FORGET.has(request?.method)) conn.send({ type: "extension_ui_response", id: request?.id, cancelled: true });
}
