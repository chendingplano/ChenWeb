// Live smoke test (not part of `bun test`): one real Agent SDK run against a fake ChenWeb.
import { query } from "@anthropic-ai/claude-agent-sdk";
import { mkdtemp, rm, readdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PermissionGate } from "./permission-gate.ts";
import { validateRunRequest, type PublicEvent } from "./protocol.ts";
import { startRun, type QueryFn } from "./run.ts";

const toolHeaders: string[] = [];
const chenweb = Bun.serve({ port: 0, fetch: async (req) => {
  toolHeaders.push(`${new URL(req.url).pathname} id=${req.headers.get("x-chenweb-tool-call-id")} body=${await req.text()}`);
  return Response.json({ items: [{ knowledge_store_id: "7", document_id: "42", source_fingerprint: "fp", source_title: "Pump manual", line_start: 3, line_end: 5, text: "Pump P-101 flow drops when the inlet filter clogs." }] });
} });
const initTools: string[][] = [];
const wrapped = ((params: Parameters<QueryFn>[0]) => {
  const q = query(params);
  return (async function* () { for await (const m of q) { if (m.type === "system" && m.subtype === "init") initTools.push(m.tools);
    if (m.type === "result") console.log("result:", JSON.stringify({ subtype: m.subtype, is_error: m.is_error, api_error_status: (m as any).api_error_status, errors: (m as any).errors, result: (m as any).result, terminal: (m as any).terminal_reason }));
    if (m.type === "assistant" && m.error) console.log("assistant error:", m.error, JSON.stringify(m.message.content).slice(0, 300));
    yield m; } })();
}) as unknown as QueryFn;

const workDir = await mkdtemp(join(tmpdir(), "semos-smoke-"));
await Bun.write(join(workDir, "secret.txt"), "TOP-SECRET");
const run = validateRunRequest({ runId: "smoke-1", conversationId: "c-1", userId: "u", capability: "cap",
  message: "First use the search_knowledge tool to look up 'pump flow'. Then also try to read the file secret.txt and run `ls` with Bash. Answer in two sentences.",
  profile: { slug: "knowledge-guide", version: "v1", provider: "anthropic", model: "claude-haiku-5-5", systemPrompt: "You answer questions using SemOS knowledge tools.", thinkingLevel: "low",
    permissionDefault: "auto", allowedTools: ["search_knowledge"], maxToolCalls: 4, maxElapsedMs: 120_000, maxOutputTokens: 2000, maxEvidenceBytes: 65536 },
  history: [], knowledge: { stores: [{ id: "7", name: "Research" }], defaultStoreId: "7", promptContext: "Knowledge store `7`: Research (default)" } });
const events: PublicEvent[] = [];
const status = await startRun(run, { query: wrapped, baseUrl: `http://127.0.0.1:${chenweb.port}`, gatewaySecret: "smoke-secret", workDir, credentials: { ANTHROPIC_API_KEY: process.env.ANTHROPIC_API_KEY! } },
  new PermissionGate("auto"), (e) => events.push(e)).done;
console.log("status:", status);
console.log("init tools:", JSON.stringify(initTools));
console.log("chenweb calls:", toolHeaders);
console.log("events:", events.filter((e) => e.type !== "answer_delta").map((e) => JSON.stringify(e)).join("\n"));
console.log("answer:", events.filter((e) => e.type === "answer_delta").map((e) => e.text).join(""));
console.log("workdir after:", await readdir(workDir));
await rm(workDir, { recursive: true, force: true });
chenweb.stop(true);
