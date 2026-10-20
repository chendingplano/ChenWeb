import { describe, expect, test } from "bun:test";
import type { CanUseTool, SDKMessage, SDKUserMessage } from "@anthropic-ai/claude-agent-sdk";
import { PermissionGate } from "./permission-gate.ts";
import { validateRunRequest, type PublicEvent, type RunRequest } from "./protocol.ts";
import { promptText, runOptions, startRun, type QueryFn, type RunEnvironment } from "./run.ts";
import { createGatewayHandler } from "./server.ts";
import { createRunTools, mcpToolName } from "./tools.ts";

const SECRET = "gateway-secret-gateway-secret-000";

function runRequest(overrides: Partial<RunRequest["profile"]> = {}, extra: Partial<RunRequest> = {}): RunRequest {
  return validateRunRequest({
    runId: "run-1", conversationId: "conversation-1", userId: "user-1", message: "Why is flow low?", capability: "cap-token",
    profile: { slug: "knowledge-guide", version: "v2", provider: "anthropic", model: "claude-sonnet-5-5", systemPrompt: "Use evidence.", thinkingLevel: "medium",
      permissionDefault: "auto", allowedTools: ["search_knowledge", "get_saved_tool_result"], maxToolCalls: 4, maxElapsedMs: 60_000, maxOutputTokens: 1000, maxEvidenceBytes: 65_536, ...overrides },
    history: [], knowledge: { stores: [{ id: "7", name: "Research" }], defaultStoreId: "7", promptContext: "Store `7`: Research (default)" },
    ...extra,
  });
}

const sdk = { name: "semos", source: "sdk" };
const context = (toolUseID: string, mcpServer: { name: string; source: string } | undefined = sdk) =>
  ({ signal: new AbortController().signal, toolUseID, mcpServer }) as Parameters<CanUseTool>[2];

function toolHarness(mode: "ask" | "auto" = "auto", fetchImpl?: (request: Request) => Promise<Response>) {
  const events: PublicEvent[] = [];
  const requests: Request[] = [];
  const gate = new PermissionGate(mode, (p) => events.push({ type: "permission_request", requestId: p.requestId, tool: p.tool }));
  const tools = createRunTools({
    baseUrl: "http://chenweb.test", gatewaySecret: SECRET, capability: "cap-token", runId: "run-1",
    allowedTools: ["search_knowledge", "get_saved_tool_result"], knowledgeStoreIds: ["7"], defaultStoreId: "7",
    maxEvidenceBytes: 65_536, maxToolCalls: 2, gate, emit: (e) => events.push(e),
    fetch: fetchImpl ?? (async (request) => {
      requests.push(request);
      return Response.json({ items: [{ knowledge_store_id: "7", document_id: "42", source_fingerprint: "fp", source_title: "Pump manual", secret_field: "x" }] });
    }),
  });
  return { tools, gate, events, requests };
}

describe("protocol", () => {
  test("rejects providers other than Anthropic", () => {
    expect(() => runRequest({ provider: "openai" })).toThrow();
  });

  test("replays history as a transcript before the question", () => {
    const run = runRequest({}, { history: [
      { role: "user", content: "What pumps exist?" },
      { role: "assistant", content: "Two pumps.", toolCalls: [{ id: "call-1", name: "search_knowledge", arguments: { query: "pumps" }, result: "{\"items\":[]}", isError: false }] },
    ] });
    const text = promptText(run);
    expect(text).toContain("<earlier_turns>");
    expect(text).toContain("[Tool call call-1: search_knowledge {\"query\":\"pumps\"}]");
    expect(text.endsWith("<current_question>\nWhy is flow low?\n</current_question>")).toBe(true);
    expect(promptText(runRequest())).toBe("Why is flow low?");
  });
});

describe("tool boundary", () => {
  test("run options switch off built-in tools, settings and inherited secrets", () => {
    const { tools } = toolHarness();
    const env: RunEnvironment = { query: (() => { throw new Error("unused"); }) as unknown as QueryFn, baseUrl: "http://chenweb.test", gatewaySecret: SECRET, workDir: "/tmp/run-dir", credentials: { ANTHROPIC_API_KEY: "model-key" } };
    const options = runOptions(runRequest(), env, tools, new AbortController());
    expect(options.tools).toEqual([]);
    expect(options.allowedTools).toEqual([]);
    expect(Object.keys(options.mcpServers ?? {})).toEqual(["semos"]);
    expect(options.strictMcpConfig).toBe(true);
    expect(options.settingSources).toEqual([]);
    expect(options.persistSession).toBe(false);
    expect(options.env?.HOME).toBe("/tmp/run-dir");
    expect(options.env?.ANTHROPIC_API_KEY).toBe("model-key");
    expect(Object.values(options.env ?? {})).not.toContain(SECRET);
    expect(options.maxTurns).toBe(5);
  });

  test("denies built-in, foreign-server and unlisted tools", async () => {
    const { tools } = toolHarness();
    expect((await tools.canUseTool("Bash", { command: "ls" }, context("t1", undefined)))?.behavior).toBe("deny");
    expect((await tools.canUseTool(mcpToolName("search_knowledge"), { query: "x" }, context("t2", { name: "semos", source: "project" })))?.behavior).toBe("deny");
    expect((await tools.canUseTool(mcpToolName("read_source_passages"), { document_id: "1" }, context("t3")))?.behavior).toBe("deny");
  });

  test("a call that did not pass the gate is not executed", async () => {
    const { tools, requests } = toolHarness();
    const result = await tools.handlers.get("search_knowledge")!({ query: "flow" });
    expect(result.isError).toBe(true);
    expect(requests).toHaveLength(0);
  });

  test("allowed call reaches ChenWeb under the SDK tool-use ID and reports safe sources", async () => {
    const { tools, events, requests } = toolHarness();
    expect((await tools.canUseTool(mcpToolName("search_knowledge"), { query: "flow" }, context("toolu_1")))?.behavior).toBe("allow");
    const result = await tools.handlers.get("search_knowledge")!({ query: "flow" });
    expect(result.isError).toBeUndefined();
    const request = requests[0]!;
    expect(request.url).toBe("http://chenweb.test/api/internal/agent-tools/search_knowledge");
    expect(request.headers.get("authorization")).toBe(`Bearer ${SECRET}`);
    expect(request.headers.get("x-chenweb-tool-call-id")).toBe("toolu_1");
    expect(request.headers.get("x-chenweb-run-capability")).toBe("cap-token");
    expect(await request.json()).toEqual({ query: "flow", knowledge_store_id: "7" });
    expect(events).toEqual([
      { type: "activity", status: "started", tool: "search_knowledge", toolCallId: "toolu_1" },
      { type: "sources", toolCallId: "toolu_1", toolName: "search_knowledge", sources: [{ knowledge_store_id: "7", document_id: "42", source_title: "Pump manual", source_fingerprint: "fp" }] },
      { type: "activity", status: "completed", tool: "search_knowledge", toolCallId: "toolu_1", error: false },
    ]);
  });

  test("ask mode waits for the decision and records a denial as a failed call", async () => {
    const { tools, gate, events } = toolHarness("ask");
    const pending = tools.canUseTool(mcpToolName("search_knowledge"), { query: "flow" }, context("toolu_2"));
    await Bun.sleep(0);
    expect(events.at(-1)).toEqual({ type: "permission_request", requestId: "toolu_2", tool: "search_knowledge" });
    expect(gate.resolve("toolu_2", false)).toBe(true);
    expect((await pending)?.behavior).toBe("deny");
    expect(events.at(-1)).toEqual({ type: "activity", status: "completed", tool: "search_knowledge", toolCallId: "toolu_2", error: true });
  });

  test("enforces the tool call limit", async () => {
    const { tools } = toolHarness();
    for (const id of ["a", "b"]) expect((await tools.canUseTool(mcpToolName("search_knowledge"), { query: id }, context(id)))?.behavior).toBe("allow");
    expect((await tools.canUseTool(mcpToolName("search_knowledge"), { query: "c" }, context("c")))?.behavior).toBe("deny");
  });
});

// A fake query() that replays scripted SDK messages after reading the prompt.
function fakeQuery(script: SDKMessage[], options: { hang?: boolean } = {}): { query: QueryFn; prompts: string[] } {
  const prompts: string[] = [];
  const query = ((params: { prompt: AsyncIterable<SDKUserMessage> | string; options?: { abortController?: AbortController } }) => (async function* () {
    if (typeof params.prompt !== "string") {
      const first = await params.prompt[Symbol.asyncIterator]().next();
      prompts.push(String((first.value as SDKUserMessage).message.content));
    }
    for (const message of script) yield message;
    if (options.hang) {
      const signal = params.options?.abortController?.signal;
      await new Promise((_, reject) => {
        if (signal?.aborted) reject(new Error("aborted"));
        signal?.addEventListener("abort", () => reject(new Error("aborted")));
      });
    }
  })()) as unknown as QueryFn;
  return { query, prompts };
}

const streamText = (text: string) => ({ type: "stream_event", parent_tool_use_id: null, event: { type: "content_block_delta", index: 0, delta: { type: "text_delta", text } } }) as unknown as SDKMessage;
const result = (subtype: string, isError = false) => ({ type: "result", subtype, is_error: isError, usage: { input_tokens: 120, output_tokens: 30, cache_read_input_tokens: 5, cache_creation_input_tokens: 0 } }) as unknown as SDKMessage;

function environment(query: QueryFn): RunEnvironment {
  return { query, baseUrl: "http://chenweb.test", gatewaySecret: SECRET, workDir: "/tmp/run-dir", credentials: { ANTHROPIC_API_KEY: "model-key" } };
}

describe("run events", () => {
  test("streams the answer, reports usage, and completes", async () => {
    const { query, prompts } = fakeQuery([
      { type: "stream_event", parent_tool_use_id: "toolu_sub", event: { type: "content_block_delta", index: 0, delta: { type: "text_delta", text: "subagent" } } } as unknown as SDKMessage,
      streamText("Flow is "), streamText("low."), result("success"),
    ]);
    const events: PublicEvent[] = [];
    expect(await startRun(runRequest(), environment(query), new PermissionGate("auto"), (e) => events.push(e)).done).toBe("completed");
    expect(prompts).toEqual(["Why is flow low?"]);
    expect(events).toEqual([
      { type: "answer_delta", text: "Flow is " }, { type: "answer_delta", text: "low." },
      { type: "usage", inputTokens: 120, outputTokens: 30, cacheReadTokens: 5, cacheWriteTokens: 0 },
      { type: "completion", status: "completed" },
    ]);
  });

  test("maps a turn limit to limit_reached", async () => {
    const events: PublicEvent[] = [];
    expect(await startRun(runRequest(), environment(fakeQuery([result("error_max_turns", true)]).query), new PermissionGate("auto"), (e) => events.push(e)).done).toBe("limit_reached");
    expect(events.at(-1)).toEqual({ type: "completion", status: "limit_reached" });
  });

  test("stops when the output budget is used", async () => {
    const { query } = fakeQuery([
      { type: "stream_event", parent_tool_use_id: null, event: { type: "message_delta", delta: {}, usage: { output_tokens: 1000 } } } as unknown as SDKMessage,
    ], { hang: true });
    const events: PublicEvent[] = [];
    expect(await startRun(runRequest(), environment(query), new PermissionGate("auto"), (e) => events.push(e)).done).toBe("limit_reached");
  });

  test("cancel aborts the query", async () => {
    const run = startRun(runRequest(), environment(fakeQuery([streamText("Partial")], { hang: true }).query), new PermissionGate("auto"), () => {});
    await Bun.sleep(5);
    run.cancel();
    expect(await run.done).toBe("cancelled");
  });
});

describe("gateway routes", () => {
  const workDirs = { create: async () => "/tmp/run-dir", remove: async () => {} };
  const post = (path: string, body: unknown, secret = SECRET) => new Request(`http://gateway.test${path}`, { method: "POST", headers: { authorization: `Bearer ${secret}`, "content-type": "application/json" }, body: JSON.stringify(body) });

  test("requires the gateway secret", async () => {
    const handler = createGatewayHandler({ secret: SECRET, environment: environment(fakeQuery([]).query), workDirs });
    expect((await handler(post("/v1/runs", {}, "wrong"))).status).toBe(401);
  });

  test("streams NDJSON events for a run", async () => {
    const handler = createGatewayHandler({ secret: SECRET, environment: environment(fakeQuery([streamText("Hi"), result("success")]).query), workDirs });
    const response = await handler(post("/v1/runs", runRequest()));
    expect(response.status).toBe(200);
    const lines = (await response.text()).trim().split("\n").map((line) => JSON.parse(line));
    expect(lines[0]).toEqual({ type: "answer_delta", text: "Hi" });
    expect(lines.at(-1)).toEqual({ type: "completion", status: "completed" });
  });

  test("rejects an invalid run and unknown cancel", async () => {
    const handler = createGatewayHandler({ secret: SECRET, environment: environment(fakeQuery([]).query), workDirs });
    expect((await handler(post("/v1/runs", { runId: "x" }))).status).toBe(400);
    expect((await handler(post("/v1/runs/missing/cancel", {}))).status).toBe(404);
  });

  test("summaries use the injected summarizer", async () => {
    const handler = createGatewayHandler({ secret: SECRET, environment: environment(fakeQuery([]).query), workDirs, summarizer: async () => " Folded. " });
    const response = await handler(post("/v1/summaries", { provider: "anthropic", model: "claude-sonnet-5-5", systemPrompt: "Summarize.", messages: [{ role: "user", content: "Q" }], maxTokens: 500 }));
    expect(await response.json()).toEqual({ summary: "Folded." });
  });
});
