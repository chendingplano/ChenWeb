import { getSessionInfo, getSessionMessages, listSessions, query } from "@anthropic-ai/claude-agent-sdk";
import type { SDKUserMessage } from "@anthropic-ai/claude-agent-sdk";
import { toHistoryMessage, isSessionId, validateDecision, validateTurnRequest, type HarnessEvent, type ModelOption } from "./protocol.ts";
import { LiveSession, SessionBusyError, claudeCodeOptions, type SessionEnvironment } from "./session.ts";

// Harness front-end protocol v1 for Claude Code. ChenWeb is the only client;
// every route requires the gateway secret.
export interface SessionStoreApi {
  listSessions: typeof listSessions;
  getSessionInfo: typeof getSessionInfo;
  getSessionMessages: typeof getSessionMessages;
}
export interface GatewayOptions { secret: string; environment: SessionEnvironment; sessions?: SessionStoreApi; models?: () => Promise<ModelOption[]> }

/**
 * The models Claude Code offers this account, as its /model picker lists them.
 * It starts Claude Code, asks, and stops it again; no message is sent.
 */
export async function claudeCodeModels(env: SessionEnvironment): Promise<ModelOption[]> {
  const abort = new AbortController();
  async function* idle(): AsyncGenerator<SDKUserMessage> { await new Promise((resolve) => abort.signal.addEventListener("abort", resolve)); }
  const probe = env.query({ prompt: idle(), options: { ...claudeCodeOptions(env), abortController: abort } });
  try {
    const models = await probe.supportedModels();
    return models.map((m) => ({ value: m.value, resolvedModel: m.resolvedModel, displayName: m.displayName, description: m.description }));
  } finally {
    abort.abort();
  }
}

const MODELS_TTL_MS = 10 * 60_000;

const json = (status: number, value: unknown) => new Response(JSON.stringify(value), { status, headers: { "content-type": "application/json" } });
const intParam = (url: URL, name: string, fallback: number, max: number) => {
  const value = Number(url.searchParams.get(name) ?? fallback);
  return Number.isInteger(value) && value >= 0 ? Math.min(value, max) : fallback;
};

export function createGatewayHandler(options: GatewayOptions): (request: Request) => Promise<Response> {
  const env = options.environment;
  const store = options.sessions ?? { listSessions, getSessionInfo, getSessionMessages };
  const live = new Map<string, LiveSession>();
  const listModels = options.models ?? (() => claudeCodeModels(env));
  let models: { list: ModelOption[]; expires: number } | undefined;

  // Only sessions Claude Code lists for the workspace folder can be read or resumed.
  const known = async (id: string) => isSessionId(id) && (live.has(id) || (await store.getSessionInfo(id, { dir: env.cwd })) !== undefined);

  const openSession = (resume: string | undefined, permissionMode: "default" | "acceptEdits" | "plan", model: string | undefined) => {
    const session: LiveSession = new LiveSession(env, {
      resume, permissionMode, model,
      onId: (id) => live.set(id, session),
      onClose: () => { for (const [id, s] of live) if (s === session) live.delete(id); },
    });
    if (resume) live.set(resume, session);
    return session;
  };

  return async (request) => {
    if (!options.secret || request.headers.get("authorization") !== `Bearer ${options.secret}`) return json(401, { error: "unauthorized" });
    const url = new URL(request.url);
    const path = url.pathname;
    if (request.method === "GET" && path === "/health") return json(200, { status: "ok", harness: "claude-code", cwd: env.cwd });

    if (request.method === "GET" && path === "/v1/models") {
      if (!models || models.expires < Date.now()) {
        try { models = { list: await listModels(), expires: Date.now() + MODELS_TTL_MS }; } catch { return json(502, { error: "Claude Code did not list its models" }); }
      }
      return json(200, { models: models.list });
    }

    if (request.method === "GET" && path === "/v1/sessions") {
      const sessions = await store.listSessions({ dir: env.cwd, limit: intParam(url, "limit", 50, 200), offset: intParam(url, "offset", 0, 100_000) });
      return json(200, { sessions: sessions.map((s) => ({ sessionId: s.sessionId, title: s.customTitle || s.summary, firstPrompt: s.firstPrompt, lastModified: s.lastModified, gitBranch: s.gitBranch, live: live.has(s.sessionId) })) });
    }

    const messages = path.match(/^\/v1\/sessions\/([^/]+)\/messages$/);
    if (request.method === "GET" && messages) {
      const id = decodeURIComponent(messages[1]!);
      if (!(await known(id))) return json(404, { error: "session not found" });
      const entries = await store.getSessionMessages(id, { dir: env.cwd, limit: intParam(url, "limit", 200, 1000), offset: intParam(url, "offset", 0, 1_000_000) });
      return json(200, { sessionId: id, messages: entries.flatMap((entry) => toHistoryMessage(entry) ?? []) });
    }

    const interrupt = path.match(/^\/v1\/sessions\/([^/]+)\/interrupt$/);
    if (request.method === "POST" && interrupt) {
      const interrupted = await live.get(decodeURIComponent(interrupt[1]!))?.interrupt();
      return json(interrupted ? 202 : 404, { status: interrupted ? "interrupt_requested" : "not_running" });
    }

    const permission = path.match(/^\/v1\/sessions\/([^/]+)\/permissions\/([^/]+)$/);
    if (request.method === "POST" && permission) {
      let decision; try { decision = validateDecision(await readJSON(request, 64 * 1024)); } catch { return json(400, { error: "invalid decision" }); }
      const resolved = live.get(decodeURIComponent(permission[1]!))?.decide(decodeURIComponent(permission[2]!), decision) ?? false;
      return json(resolved ? 200 : 404, { status: resolved ? "resolved" : "not_found" });
    }

    if (request.method !== "POST" || path !== "/v1/turns") return json(404, { error: "not found" });
    let turn; try { turn = validateTurnRequest(await readJSON(request, 512 * 1024)); } catch { return json(400, { error: "invalid turn request" }); }
    let session = turn.sessionId ? live.get(turn.sessionId) : undefined;
    if (turn.sessionId && !session) {
      if (!(await known(turn.sessionId))) return json(404, { error: "session not found" });
      session = openSession(turn.sessionId, turn.permissionMode ?? "default", turn.model);
    }
    session ??= openSession(undefined, turn.permissionMode ?? "default", turn.model);
    if (session.busy) return json(409, { error: "a turn is already running in this session" });

    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const activeSession = session;
    const stream = new ReadableStream<Uint8Array>({
      start(value) { controller = value; },
      // No turn runs unattended: a disconnected client interrupts it.
      cancel() { void activeSession.interrupt(); },
    });
    const emit = (event: HarnessEvent) => { try { controller.enqueue(encoder.encode(`${JSON.stringify(event)}\n`)); } catch { /* disconnected client */ } };
    activeSession.runTurn(turn.message, turn.permissionMode, turn.model, emit)
      .catch((error) => emit({ type: "error", message: error instanceof SessionBusyError ? "A turn is already running." : "The turn could not start." }))
      .finally(() => { try { controller.close(); } catch { /* already closed */ } });
    return new Response(stream, { status: 200, headers: { "content-type": "application/x-ndjson; charset=utf-8", "cache-control": "no-store" } });
  };
}

async function readJSON(request: Request, maximum: number): Promise<unknown> {
  const text = await request.text();
  if (new TextEncoder().encode(text).byteLength > maximum) throw new Error("request too large");
  return JSON.parse(text);
}

const SENSITIVE = /SECRET|PASSWORD|API_KEY|TOKEN|ENCRYPTION|CREDENTIAL|DATABASE_URL|_DSN$|_KEY$/i;

// Claude Code signs in the way it does in a terminal or IDE. Model API keys
// would take precedence over that login, and ChenWeb's own secrets (present
// when the gateway is started through mise) are none of the agent's business.
export function claudeCodeEnv(source: Record<string, string | undefined>, useApiKey: boolean): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = {};
  for (const [name, value] of Object.entries(source)) {
    if (name === "CLAUDE_CODE_GATEWAY_SECRET" || name === "CLAUDE_CODE_GATEWAY_USE_API_KEY") continue;
    const modelKey = name === "ANTHROPIC_API_KEY" || name === "ANTHROPIC_AUTH_TOKEN";
    if (modelKey ? !useApiKey : SENSITIVE.test(name)) continue;
    env[name] = value;
  }
  env.CLAUDE_AGENT_SDK_CLIENT_APP = "chenweb-harness-frontend/0.1.0";
  return env;
}

if (import.meta.main) {
  const secret = process.env.CLAUDE_CODE_GATEWAY_SECRET ?? "";
  const cwd = process.env.CLAUDE_CODE_WORKSPACE ?? "";
  if (!secret) throw new Error("CLAUDE_CODE_GATEWAY_SECRET is required");
  if (!cwd) throw new Error("CLAUDE_CODE_WORKSPACE is required (the folder Claude Code works in)");
  const idleMinutes = Number(process.env.CLAUDE_CODE_SESSION_IDLE_MINUTES ?? "30");
  const handler = createGatewayHandler({ secret, environment: {
    query, cwd, env: claudeCodeEnv(process.env, process.env.CLAUDE_CODE_GATEWAY_USE_API_KEY === "1"),
    idleMs: (Number.isFinite(idleMinutes) && idleMinutes > 0 ? idleMinutes : 30) * 60_000,
  } });
  const hostname = process.env.CLAUDE_CODE_GATEWAY_HOST ?? "127.0.0.1";
  const port = Number(process.env.CLAUDE_CODE_GATEWAY_PORT ?? "4318");
  Bun.serve({ hostname, port, fetch: handler, idleTimeout: 0 });
  console.log(`Claude Code gateway listening on http://${hostname}:${port} (workspace ${cwd})`);
}
