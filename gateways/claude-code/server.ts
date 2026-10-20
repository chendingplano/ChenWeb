import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { query } from "@anthropic-ai/claude-agent-sdk";
import { PermissionGate, type PermissionRequest } from "./permission-gate.ts";
import { validateRunRequest, validateSummaryRequest, type PublicEvent, type RunRequest, type SummaryRequest } from "./protocol.ts";
import { startRun, summarize, type ActiveRun, type RunEnvironment } from "./run.ts";

// The same routes and bearer-secret scheme as the Pi gateway, so ChenWeb's
// adapter for this harness differs only in its URL and secret.
const MAX_REQUEST_BYTES = 512 * 1024;
const json = (status: number, value: unknown) => new Response(JSON.stringify(value), { status, headers: { "content-type": "application/json" } });

/** Creates and removes a run's empty working directory. */
export interface WorkDirs { create(): Promise<string>; remove(dir: string): Promise<void> }
export interface GatewayOptions {
  secret: string;
  environment: Omit<RunEnvironment, "workDir">;
  workDirs?: WorkDirs;
  start?: typeof startRun;
  summarizer?: (request: SummaryRequest, env: RunEnvironment) => Promise<string>;
}

const tempWorkDirs: WorkDirs = {
  create: () => mkdtemp(join(tmpdir(), "semos-claude-run-")),
  remove: (dir) => rm(dir, { recursive: true, force: true }),
};

export function createGatewayHandler(options: GatewayOptions): (request: Request) => Promise<Response> {
  const runs = new Map<string, { conversationId: string; run: ActiveRun; gate: PermissionGate }>();
  const workDirs = options.workDirs ?? tempWorkDirs;
  const start = options.start ?? startRun;
  const summarizer = options.summarizer ?? summarize;
  return async (request) => {
    if (!options.secret || request.headers.get("authorization") !== `Bearer ${options.secret}`) return json(401, { error: "unauthorized" });
    const url = new URL(request.url);
    if (request.method === "GET" && url.pathname === "/health") return json(200, { status: "ok" });
    if (request.method === "POST" && url.pathname === "/v1/summaries") {
      let summaryRequest: SummaryRequest; try { summaryRequest = validateSummaryRequest(await readJSON(request, MAX_REQUEST_BYTES)); } catch { return json(400, { error: "invalid summary request" }); }
      const workDir = await workDirs.create();
      try {
        const summary = (await summarizer(summaryRequest, { ...options.environment, workDir })).trim();
        return summary ? json(200, { summary }) : json(502, { error: "empty summary" });
      } catch { return json(502, { error: "summary failed" }); } finally { await workDirs.remove(workDir); }
    }
    const cancel = url.pathname.match(/^\/v1\/runs\/([^/]+)\/cancel$/);
    if (request.method === "POST" && cancel) {
      const active = runs.get(decodeURIComponent(cancel[1]!));
      active?.run.cancel();
      return json(active ? 202 : 404, { status: active ? "cancel_requested" : "not_found" });
    }
    const permission = url.pathname.match(/^\/v1\/runs\/([^/]+)\/permissions\/([^/]+)$/);
    if (request.method === "POST" && permission) {
      let body: unknown; try { body = await readJSON(request, 1024); } catch { return json(400, { error: "invalid request" }); }
      const allowed = (body as { allowed?: unknown })?.allowed;
      if (typeof allowed !== "boolean") return json(400, { error: "invalid request" });
      const resolved = runs.get(decodeURIComponent(permission[1]!))?.gate.resolve(decodeURIComponent(permission[2]!), allowed) ?? false;
      return json(resolved ? 200 : 404, { status: resolved ? "resolved" : "not_found" });
    }
    if (request.method !== "POST" || url.pathname !== "/v1/runs") return json(404, { error: "not found" });
    let run: RunRequest; try { run = validateRunRequest(await readJSON(request, MAX_REQUEST_BYTES)); } catch { return json(400, { error: "invalid run request" }); }
    if (runs.has(run.runId) || [...runs.values()].some((r) => r.conversationId === run.conversationId)) return json(409, { error: "run unavailable" });

    const encoder = new TextEncoder();
    let controller!: ReadableStreamDefaultController<Uint8Array>;
    let active: ActiveRun | undefined;
    const stream = new ReadableStream<Uint8Array>({ start(value) { controller = value; }, cancel() { active?.cancel(); } });
    const emit = (event: PublicEvent) => { try { controller.enqueue(encoder.encode(`${JSON.stringify(event)}\n`)); } catch { /* disconnected client */ } };
    const gate = new PermissionGate(run.profile.permissionDefault, (p: PermissionRequest) => emit({ type: "permission_request", requestId: p.requestId, tool: p.tool }));
    const workDir = await workDirs.create();
    try { active = start(run, { ...options.environment, workDir }, gate, emit); }
    catch { await workDirs.remove(workDir); return json(409, { error: "run unavailable" }); }
    runs.set(run.runId, { conversationId: run.conversationId, run: active, gate });
    void active.done.finally(async () => {
      runs.delete(run.runId);
      await workDirs.remove(workDir);
      try { controller.close(); } catch { /* disconnected client */ }
    });
    return new Response(stream, { status: 200, headers: { "content-type": "application/x-ndjson; charset=utf-8", "cache-control": "no-store" } });
  };
}

async function readJSON(request: Request, maximum: number): Promise<unknown> {
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > maximum) throw new Error("request too large");
  const reader = request.body?.getReader();
  if (!reader) throw new Error("empty request");
  const chunks: Uint8Array[] = [];
  let size = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > maximum) { await reader.cancel(); throw new Error("request too large"); }
    chunks.push(value);
  }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
}

// Model credentials the Claude Code process may receive. Everything else in
// this process's environment, including the gateway secret, stays here.
const CREDENTIAL_VARS = ["ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN"];

if (import.meta.main) {
  const secret = process.env.CLAUDE_CODE_GATEWAY_SECRET ?? "";
  if (!secret) throw new Error("CLAUDE_CODE_GATEWAY_SECRET is required");
  const credentials = Object.fromEntries(CREDENTIAL_VARS.flatMap((name) => (process.env[name] ? [[name, process.env[name]!]] : [])));
  if (Object.keys(credentials).length === 0) throw new Error("a model credential (for example ANTHROPIC_API_KEY) is required");
  const handler = createGatewayHandler({ secret, environment: { query, baseUrl: process.env.CHENWEB_INTERNAL_URL ?? "http://127.0.0.1:1323", gatewaySecret: secret, credentials } });
  const hostname = process.env.CLAUDE_CODE_GATEWAY_HOST ?? "127.0.0.1";
  const port = Number(process.env.CLAUDE_CODE_GATEWAY_PORT ?? "4318");
  Bun.serve({ hostname, port, fetch: handler, idleTimeout: 0 });
  console.log(`Claude Code gateway listening on http://${hostname}:${port}`);
}
