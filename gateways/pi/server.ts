import { realpathSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { PiHarness, SessionBusyError } from "./harness.ts";
import { isSessionId, validateDecision, validateTurnRequest, type HarnessEvent, type ModelOption } from "./protocol.ts";
import { piProcess } from "./rpc.ts";
import { sessionDir, UnsupportedSessionError } from "./sessionfile.ts";

// Harness front-end protocol v1 for Pi. ChenWeb is the only client; every
// route requires the gateway secret.
export interface GatewayOptions { secret: string; harness: PiHarness; models?: () => Promise<ModelOption[]> }

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

  return async (request) => {
    if (!options.secret || request.headers.get("authorization") !== `Bearer ${options.secret}`) return json(401, { error: "unauthorized" });
    const url = new URL(request.url);
    const path = url.pathname;
    if (request.method === "GET" && path === "/health") return json(200, { status: "ok", harness: "pi", cwd: harness.cwd });

    if (request.method === "GET" && path === "/v1/models") {
      if (!models || models.expires < Date.now()) {
        try { models = { list: await listModels(), expires: Date.now() + MODELS_TTL_MS }; } catch { return json(502, { error: "Pi did not list its models" }); }
      }
      return json(200, { models: models.list });
    }

    if (request.method === "GET" && path === "/v1/sessions") {
      try {
        return json(200, { sessions: harness.listSessions(intParam(url, "limit", 50, 200), intParam(url, "offset", 0, 100_000)) });
      } catch { return json(502, { error: "Pi's sessions could not be listed" }); }
    }

    const messages = path.match(/^\/v1\/sessions\/([^/]+)\/messages$/);
    if (request.method === "GET" && messages) {
      const id = decodeURIComponent(messages[1]!);
      let history;
      try {
        history = isSessionId(id) ? harness.history(id, intParam(url, "limit", 200, 1000), intParam(url, "offset", 0, 1_000_000)) : undefined;
      } catch (error) {
        return json(502, { error: error instanceof UnsupportedSessionError ? error.message : "Pi's session file could not be read" });
      }
      if (!history) return json(404, { error: "session not found" });
      return json(200, { sessionId: id, messages: history });
    }

    const interrupt = path.match(/^\/v1\/sessions\/([^/]+)\/interrupt$/);
    if (request.method === "POST" && interrupt) {
      const interrupted = harness.interrupt(decodeURIComponent(interrupt[1]!));
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
    // Only sessions Pi recorded for the workspace folder can be resumed.
    let known = false;
    try { known = !turn.sessionId || harness.known(turn.sessionId); } catch (error) {
      return json(502, { error: error instanceof UnsupportedSessionError ? error.message : "Pi's session file could not be read" });
    }
    if (!known) return json(404, { error: "session not found" });

    const encoder = new TextEncoder();
    const abort = new AbortController();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    const stream = new ReadableStream<Uint8Array>({
      start(value) { controller = value; },
      // No turn runs unattended: a disconnected client aborts it.
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

const SENSITIVE = /SECRET|PASSWORD|API_KEY|TOKEN|ENCRYPTION|CREDENTIAL|DATABASE_URL|_DSN$|_KEY$/i;

// Pi signs in with its own stored login (~/.pi/agent/auth.json). Keys in the
// environment would override it, ChenWeb's own secrets (present when the
// gateway is started through mise) are none of the agent's business, and the
// gateway's settings are not Pi's. `pass` names variables Pi's own
// configuration reads (such as a key that models.json refers to by name).
export function piEnv(source: Record<string, string | undefined>, pass: string[] = []): Record<string, string | undefined> {
  const env: Record<string, string | undefined> = {};
  for (const [name, value] of Object.entries(source)) {
    if (name.startsWith("PI_HARNESS_") || (SENSITIVE.test(name) && !pass.includes(name))) continue;
    env[name] = value;
  }
  return env;
}

/** Pi's configuration folder (`PI_CODING_AGENT_DIR`, by default `~/.pi/agent`). */
export const agentDir = (env: Record<string, string | undefined>) => env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent");

export const passList = (value: string | undefined) => (value ?? "").split(",").map((name) => name.trim()).filter(Boolean);

if (import.meta.main) {
  const secret = process.env.PI_HARNESS_GATEWAY_SECRET ?? "";
  if (!secret) throw new Error("PI_HARNESS_GATEWAY_SECRET is required");
  if (!process.env.PI_HARNESS_WORKSPACE) throw new Error("PI_HARNESS_WORKSPACE is required (the folder Pi works in)");
  // Pi records each session's folder as a resolved path.
  const cwd = realpathSync(process.env.PI_HARNESS_WORKSPACE);
  const idleMinutes = Number(process.env.PI_HARNESS_SESSION_IDLE_MINUTES ?? "30");
  const harness = new PiHarness({
    spawn: piProcess({ bin: process.env.PI_HARNESS_BIN || "pi", cwd, env: piEnv(process.env, passList(process.env.PI_HARNESS_PASS_ENV)) }),
    cwd, sessionDir: sessionDir(agentDir(process.env), cwd), idleMs: (Number.isFinite(idleMinutes) && idleMinutes > 0 ? idleMinutes : 30) * 60_000,
  });
  const hostname = process.env.PI_HARNESS_GATEWAY_HOST ?? "127.0.0.1";
  const port = Number(process.env.PI_HARNESS_GATEWAY_PORT ?? "4321");
  Bun.serve({ hostname, port, fetch: createGatewayHandler({ secret, harness }), idleTimeout: 0 });
  process.on("SIGINT", () => { harness.close(); process.exit(0); });
  process.on("SIGTERM", () => { harness.close(); process.exit(0); });
  console.log(`Pi gateway listening on http://${hostname}:${port} (workspace ${cwd})`);
}
