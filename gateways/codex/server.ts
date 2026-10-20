import { realpathSync } from "node:fs";
import { appServer } from "./appserver.ts";
import { CodexHarness, SessionBusyError } from "./harness.ts";
import { isSessionId, validateDecision, validateTurnRequest, type HarnessEvent, type ModelOption } from "./protocol.ts";

// Harness front-end protocol v1 for Codex. ChenWeb is the only client; every
// route requires the gateway secret.
export interface GatewayOptions { secret: string; harness: CodexHarness; models?: () => Promise<ModelOption[]> }

const MODELS_TTL_MS = 10 * 60_000;

const json = (status: number, value: unknown) => new Response(JSON.stringify(value), { status, headers: { "content-type": "application/json" } });
const intParam = (url: URL, name: string, fallback: number, max: number) => {
  const value = Number(url.searchParams.get(name) ?? fallback);
  return Number.isInteger(value) && value >= 0 ? Math.min(value, max) : fallback;
};

export function createGatewayHandler(options: GatewayOptions): (request: Request) => Promise<Response> {
  const harness = options.harness;
  const listModels = options.models ?? (() => harness.models());
  let models: { list: ModelOption[]; expires: number } | undefined;

  // Only threads Codex keeps for the workspace folder can be read or resumed.
  const known = async (id: string) => isSessionId(id) && (await harness.known(id));

  return async (request) => {
    if (!options.secret || request.headers.get("authorization") !== `Bearer ${options.secret}`) return json(401, { error: "unauthorized" });
    const url = new URL(request.url);
    const path = url.pathname;
    if (request.method === "GET" && path === "/health") return json(200, { status: "ok", harness: "codex", cwd: harness.cwd });

    if (request.method === "GET" && path === "/v1/models") {
      if (!models || models.expires < Date.now()) {
        try { models = { list: await listModels(), expires: Date.now() + MODELS_TTL_MS }; } catch { return json(502, { error: "Codex did not list its models" }); }
      }
      return json(200, { models: models.list });
    }

    if (request.method === "GET" && path === "/v1/sessions") {
      try {
        return json(200, { sessions: await harness.listSessions(intParam(url, "limit", 50, 200), intParam(url, "offset", 0, 100_000)) });
      } catch { return json(502, { error: "Codex did not list its sessions" }); }
    }

    const messages = path.match(/^\/v1\/sessions\/([^/]+)\/messages$/);
    if (request.method === "GET" && messages) {
      const id = decodeURIComponent(messages[1]!);
      const history = isSessionId(id) ? await harness.history(id, intParam(url, "limit", 200, 1000), intParam(url, "offset", 0, 1_000_000)) : undefined;
      if (!history) return json(404, { error: "session not found" });
      return json(200, { sessionId: id, messages: history });
    }

    const interrupt = path.match(/^\/v1\/sessions\/([^/]+)\/interrupt$/);
    if (request.method === "POST" && interrupt) {
      const interrupted = await harness.interrupt(decodeURIComponent(interrupt[1]!));
      return json(interrupted ? 202 : 404, { status: interrupted ? "interrupt_requested" : "not_running" });
    }

    const permission = path.match(/^\/v1\/sessions\/([^/]+)\/permissions\/([^/]+)$/);
    if (request.method === "POST" && permission) {
      let decision; try { decision = validateDecision(await readJSON(request, 64 * 1024)); } catch { return json(400, { error: "invalid decision" }); }
      const resolved = harness.decide(decodeURIComponent(permission[1]!), decodeURIComponent(permission[2]!), decision);
      return json(resolved ? 200 : 404, { status: resolved ? "resolved" : "not_found" });
    }

    if (request.method !== "POST" || path !== "/v1/turns") return json(404, { error: "not found" });
    let turn; try { turn = validateTurnRequest(await readJSON(request, 512 * 1024)); } catch { return json(400, { error: "invalid turn request" }); }
    if (turn.sessionId && harness.isBusy(turn.sessionId)) return json(409, { error: "a turn is already running in this session" });
    if (turn.sessionId && !harness.isLive(turn.sessionId)) {
      let found; try { found = await known(turn.sessionId); } catch { return json(502, { error: "Codex is unavailable" }); }
      if (!found) return json(404, { error: "session not found" });
    }

    const encoder = new TextEncoder();
    const abort = new AbortController();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const stream = new ReadableStream<Uint8Array>({
      start(value) { controller = value; },
      // No turn runs unattended: a disconnected client interrupts it.
      cancel() { abort.abort(); },
    });
    const emit = (event: HarnessEvent) => { try { controller.enqueue(encoder.encode(`${JSON.stringify(event)}\n`)); } catch { /* disconnected client */ } };
    harness.runTurn(turn, emit, abort.signal)
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

// Codex signs in the way it does in a terminal or IDE (~/.codex/auth.json).
// API keys would bypass that login, and ChenWeb's own secrets (present when
// the gateway is started through mise) are none of the agent's business.
export function codexEnv(source: Record<string, string | undefined>): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = {};
  for (const [name, value] of Object.entries(source)) {
    if (name === "CODEX_GATEWAY_SECRET" || /SECRET|PASSWORD|API_KEY/i.test(name)) continue;
    env[name] = value;
  }
  return env;
}

if (import.meta.main) {
  const secret = process.env.CODEX_GATEWAY_SECRET ?? "";
  if (!secret) throw new Error("CODEX_GATEWAY_SECRET is required");
  if (!process.env.CODEX_WORKSPACE) throw new Error("CODEX_WORKSPACE is required (the folder Codex works in)");
  // Codex records each thread's folder as a resolved path.
  const cwd = realpathSync(process.env.CODEX_WORKSPACE);
  const idleMinutes = Number(process.env.CODEX_SESSION_IDLE_MINUTES ?? "30");
  const harness = new CodexHarness({
    connect: appServer({ bin: process.env.CODEX_BIN || "codex", cwd, env: codexEnv(process.env) }),
    cwd, idleMs: (Number.isFinite(idleMinutes) && idleMinutes > 0 ? idleMinutes : 30) * 60_000,
  });
  const hostname = process.env.CODEX_GATEWAY_HOST ?? "127.0.0.1";
  const port = Number(process.env.CODEX_GATEWAY_PORT ?? "4319");
  Bun.serve({ hostname, port, fetch: createGatewayHandler({ secret, harness }), idleTimeout: 0 });
  process.on("SIGINT", () => { harness.close(); process.exit(0); });
  process.on("SIGTERM", () => { harness.close(); process.exit(0); });
  console.log(`Codex gateway listening on http://${hostname}:${port} (workspace ${cwd})`);
}
