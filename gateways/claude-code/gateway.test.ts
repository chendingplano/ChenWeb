import { describe, expect, test } from "bun:test";
import type { CanUseTool, Options, SDKMessage, SDKUserMessage } from "@anthropic-ai/claude-agent-sdk";
import { claudeCodeEnv, createGatewayHandler, type SessionStoreApi } from "./server.ts";
import type { QueryFn } from "./session.ts";

const SECRET = "gateway-secret";
const SESSION = "11111111-2222-3333-4444-555555555555";
const OTHER = "99999999-2222-3333-4444-555555555555";

type Step = (ctx: { options: Options; text: string }) => AsyncGenerator<SDKMessage>;

/** Stands in for Claude Code: answers each pushed user message with a scripted step. */
function fakeClaude(step: Step) {
  const calls: Options[] = [];
  const received: string[] = [];
  const modes: string[] = [];
  let interrupted = false;
  const query = ((params: { prompt: AsyncIterable<SDKUserMessage>; options: Options }) => {
    calls.push(params.options);
    const sessionId = params.options.resume ?? SESSION;
    async function* run(): AsyncGenerator<SDKMessage> {
      let first = true;
      for await (const input of params.prompt) {
        const text = String(input.message.content);
        received.push(text);
        if (first) { first = false; yield { type: "system", subtype: "init", session_id: sessionId, model: "claude-opus-5-5", cwd: params.options.cwd, permissionMode: params.options.permissionMode } as unknown as SDKMessage; }
        interrupted = false;
        yield* step({ options: params.options, text });
      }
    }
    const iterator = run();
    return Object.assign(iterator, {
      interrupt: async () => { interrupted = true; return undefined; },
      setPermissionMode: async (mode: string) => { modes.push(mode); },
    });
  }) as unknown as QueryFn;
  return { query, calls, received, modes, wasInterrupted: () => interrupted };
}

const result = (status = "success", extra: Record<string, unknown> = {}) => ({ type: "result", subtype: status, is_error: status !== "success", session_id: SESSION, duration_ms: 12, num_turns: 1, total_cost_usd: 0.01, result: "done", usage: { input_tokens: 10, output_tokens: 5, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 }, ...extra }) as unknown as SDKMessage;
const text = (t: string) => ({ type: "stream_event", parent_tool_use_id: null, event: { type: "content_block_delta", index: 0, delta: { type: "text_delta", text: t } } }) as unknown as SDKMessage;

const store = (sessions: string[] = [SESSION]): SessionStoreApi => ({
  listSessions: (async () => sessions.map((id) => ({ sessionId: id, summary: "Fix the build", lastModified: 1, firstPrompt: "fix it" }))) as unknown as SessionStoreApi["listSessions"],
  getSessionInfo: (async (id: string) => (sessions.includes(id) ? { sessionId: id } : undefined)) as unknown as SessionStoreApi["getSessionInfo"],
  getSessionMessages: (async () => [
    { type: "user", uuid: "u1", session_id: SESSION, parent_tool_use_id: null, message: { role: "user", content: "fix it" } },
    { type: "assistant", uuid: "a1", session_id: SESSION, parent_tool_use_id: null, message: { content: [{ type: "thinking", thinking: "secret reasoning" }, { type: "text", text: "Done." }, { type: "tool_use", id: "t1", name: "Bash", input: { command: "make" } }] } },
    { type: "system", uuid: "s1", session_id: SESSION, parent_tool_use_id: null, message: {} },
  ]) as unknown as SessionStoreApi["getSessionMessages"],
});

function gateway(step: Step, options: { sessions?: string[]; idleMs?: number } = {}) {
  const claude = fakeClaude(step);
  const handler = createGatewayHandler({ secret: SECRET, sessions: store(options.sessions), environment: { query: claude.query, cwd: "/work", env: { PATH: "/bin" }, idleMs: options.idleMs ?? 60_000 } });
  const call = (method: string, path: string, body?: unknown, secret = SECRET) =>
    handler(new Request(`http://gateway.test${path}`, { method, headers: { authorization: `Bearer ${secret}` }, body: body === undefined ? undefined : JSON.stringify(body) }));
  const events = async (response: Response) => (await response.text()).trim().split("\n").map((line) => JSON.parse(line));
  return { claude, call, events };
}

async function* answer({ text: message }: { text: string }) {
  yield text(`echo: ${message}`);
  yield { type: "assistant", parent_tool_use_id: null, message: { content: [{ type: "text", text: "ignored (already streamed)" }, { type: "tool_use", id: "toolu_1", name: "Read", input: { file_path: "a.go" } }] } } as unknown as SDKMessage;
  yield { type: "user", parent_tool_use_id: null, message: { role: "user", content: [{ type: "tool_result", tool_use_id: "toolu_1", content: "package main", is_error: false }] } } as unknown as SDKMessage;
  yield result();
}

describe("Claude Code environment", () => {
  test("uses the Claude Code login and keeps ChenWeb secrets out", () => {
    const env = claudeCodeEnv({ PATH: "/bin", HOME: "/Users/me", ANTHROPIC_API_KEY: "k", ANTHROPIC_AUTH_TOKEN: "t", CLAUDE_CODE_GATEWAY_SECRET: "s", PG_PASSWORD: "p", PI_RUN_CAPABILITY_SECRET: "c", OPENAI_API_KEY: "o" }, false);
    expect(env).toEqual({ PATH: "/bin", HOME: "/Users/me", CLAUDE_AGENT_SDK_CLIENT_APP: "chenweb-harness-frontend/0.1.0" });
    expect(claudeCodeEnv({ ANTHROPIC_API_KEY: "k" }, true).ANTHROPIC_API_KEY).toBe("k");
  });
});

describe("turns", () => {
  test("runs the full Claude Code harness and streams its events", async () => {
    const { claude, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hello" }));
    expect(lines).toEqual([
      { type: "session", sessionId: SESSION, model: "claude-opus-5-5", cwd: "/work", permissionMode: "default" },
      { type: "text", text: "echo: hello" },
      { type: "tool_use", id: "toolu_1", name: "Read", input: { file_path: "a.go" }, parentToolUseId: null },
      { type: "tool_result", toolUseId: "toolu_1", isError: false, text: "package main" },
      { type: "result", status: "completed", sessionId: SESSION, durationMs: 12, numTurns: 1, costUsd: 0.01, usage: { inputTokens: 10, outputTokens: 5, cacheReadTokens: 0, cacheWriteTokens: 0 }, text: "done" },
    ]);
    const options = claude.calls[0]!;
    expect(options.systemPrompt).toEqual({ type: "preset", preset: "claude_code" });
    expect(options.tools).toEqual({ type: "preset", preset: "claude_code" });
    expect(options.settingSources).toEqual(["user", "project", "local"]);
    expect(options.cwd).toBe("/work");
    expect(options.persistSession).toBeUndefined();
  });

  test("a second turn goes to the same live session with only the new message", async () => {
    const { claude, call, events } = gateway(answer);
    await events(await call("POST", "/v1/turns", { message: "first" }));
    const lines = await events(await call("POST", "/v1/turns", { sessionId: SESSION, message: "second", permissionMode: "acceptEdits" }));
    expect(lines.at(-1).status).toBe("completed");
    expect(claude.calls).toHaveLength(1);
    expect(claude.received).toEqual(["first", "second"]);
    expect(claude.modes).toEqual(["acceptEdits"]);
  });

  test("resumes a listed session and refuses others", async () => {
    const { claude, call, events } = gateway(answer, { sessions: [SESSION] });
    expect((await call("POST", "/v1/turns", { sessionId: OTHER, message: "hi" })).status).toBe(404);
    await events(await call("POST", "/v1/turns", { sessionId: SESSION, message: "hi" }));
    expect(claude.calls[0]!.resume).toBe(SESSION);
  });

  test("refuses a second turn while one is running, and bypassPermissions", async () => {
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    const { call, events } = gateway(async function* () { await held; yield result(); });
    const first = call("POST", "/v1/turns", { message: "slow" });
    const firstResponse = await first;
    await Bun.sleep(5);
    expect((await call("POST", "/v1/turns", { sessionId: SESSION, message: "again" })).status).toBe(409);
    expect((await call("POST", "/v1/turns", { message: "x", permissionMode: "bypassPermissions" })).status).toBe(400);
    release();
    expect((await events(firstResponse)).at(-1).status).toBe("completed");
  });

  test("permission requests wait for the user's decision, including answers", async () => {
    let decided: unknown;
    const { call, events } = gateway(async function* ({ options }) {
      decided = await (options.canUseTool as CanUseTool)("AskUserQuestion", { questions: [{ question: "Which DB?" }] },
        { signal: new AbortController().signal, toolUseID: "toolu_q", title: "Claude has a question" } as Parameters<CanUseTool>[2]);
      yield result();
    });
    const response = await call("POST", "/v1/turns", { message: "set up the db" });
    await Bun.sleep(5);
    expect((await call("POST", `/v1/sessions/${SESSION}/permissions/toolu_q`, { behavior: "allow", updatedInput: { answers: { "Which DB?": "Postgres" } } })).status).toBe(200);
    const lines = await events(response);
    expect(lines[1]).toEqual({ type: "permission_request", requestId: "toolu_q", toolName: "AskUserQuestion", input: { questions: [{ question: "Which DB?" }] }, title: "Claude has a question" });
    expect(decided).toEqual({ behavior: "allow", updatedInput: { answers: { "Which DB?": "Postgres" } } });
  });

  test("interrupt marks the turn interrupted", async () => {
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    const { claude, call, events } = gateway(async function* () { yield text("working"); await held; yield result("error_during_execution"); });
    const response = await call("POST", "/v1/turns", { message: "long task" });
    await Bun.sleep(5);
    expect((await call("POST", `/v1/sessions/${SESSION}/interrupt`)).status).toBe(202);
    expect(claude.wasInterrupted()).toBe(true);
    release();
    expect((await events(response)).at(-1).status).toBe("interrupted");
  });

  test("an idle session closes and is resumed on the next turn", async () => {
    const { claude, call, events } = gateway(answer, { idleMs: 5 });
    await events(await call("POST", "/v1/turns", { message: "first" }));
    await Bun.sleep(20);
    await events(await call("POST", "/v1/turns", { sessionId: SESSION, message: "later" }));
    expect(claude.calls).toHaveLength(2);
    expect(claude.calls[1]!.resume).toBe(SESSION);
  });
});

describe("sessions and history", () => {
  test("requires the gateway secret", async () => {
    const { call } = gateway(answer);
    expect((await call("GET", "/v1/sessions", undefined, "wrong")).status).toBe(401);
  });

  test("lists workspace sessions and returns history without thinking", async () => {
    const { call } = gateway(answer);
    expect(await (await call("GET", "/v1/sessions")).json()).toEqual({ sessions: [{ sessionId: SESSION, title: "Fix the build", firstPrompt: "fix it", lastModified: 1, live: false }] });
    const history = await (await call("GET", `/v1/sessions/${SESSION}/messages`)).json();
    expect(history.messages).toEqual([
      { uuid: "u1", role: "user", parentToolUseId: null, blocks: [{ type: "text", text: "fix it" }] },
      { uuid: "a1", role: "assistant", parentToolUseId: null, blocks: [{ type: "text", text: "Done." }, { type: "tool_use", id: "t1", name: "Bash", input: { command: "make" } }] },
    ]);
    expect((await call("GET", `/v1/sessions/${OTHER}/messages`)).status).toBe(404);
  });
});
