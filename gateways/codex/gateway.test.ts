import { describe, expect, test } from "bun:test";
import type { Connect, RpcHandlers } from "./appserver.ts";
import { CodexHarness } from "./harness.ts";
import type { ModelOption } from "./protocol.ts";
import { codexEnv, createGatewayHandler } from "./server.ts";

const SECRET = "gateway-secret";
const THREAD = "019a0000-2222-7333-8444-555555555555";
const OTHER = "019a9999-2222-7333-8444-555555555555";
const FOREIGN = "019a8888-2222-7333-8444-555555555555";

/** What a scripted turn can do: send notifications and ask the user (server requests). */
interface Ctx { threadId: string; turnId: string; text: string; notify: (method: string, params: Record<string, unknown>) => void; ask: (method: string, params: Record<string, unknown>) => Promise<any>; complete: (status?: string, error?: unknown) => void }
type Script = (ctx: Ctx) => Promise<void> | void;

/** Stands in for `codex app-server`: records requests and plays a script for each turn. */
function fakeCodex(script: Script, options: { threads?: Record<string, string> } = {}) {
  const requests: { method: string; params: any }[] = [];
  let handlers!: RpcHandlers;
  let turns = 0;
  let rpc = 0;
  const known: Record<string, string> = { [THREAD]: "/work", [FOREIGN]: "/elsewhere", ...options.threads };
  const thread = (id: string) => ({ id, cwd: known[id] ?? "/work", preview: "fix it", name: id === THREAD ? "Fix the build" : null, updatedAt: 1_700_000_000, gitInfo: { branch: "main" } });
  const connect: Connect = async (h) => {
    handlers = h;
    return {
      async request(method: string, params: any): Promise<any> {
        requests.push({ method, params });
        switch (method) {
          case "thread/start": known[THREAD] = "/work"; return { thread: thread(THREAD), model: params.model ?? "gpt-5.6-sol", cwd: "/work", sandbox: { type: "workspaceWrite" } };
          case "thread/resume": return { thread: thread(params.threadId), model: params.model ?? "gpt-5.6-sol", cwd: "/work", sandbox: { type: "workspaceWrite" } };
          case "thread/read":
            if (!(params.threadId in known)) throw new Error("no such thread");
            return { thread: { ...thread(params.threadId), turns: params.includeTurns ? [{ id: "t0", items: [
              { type: "userMessage", id: "u1", content: [{ type: "text", text: "fix it", text_elements: [] }] },
              { type: "reasoning", id: "r1", summary: ["secret reasoning"], content: [] },
              { type: "commandExecution", id: "c1", command: "make", cwd: "/work", status: "completed", exitCode: 0, aggregatedOutput: "ok" },
              { type: "agentMessage", id: "a1", text: "Done." },
            ] }] : [] } };
          case "thread/list": return { data: [thread(THREAD)], nextCursor: null };
          case "model/list": return { data: [
            { model: "gpt-5.6-terra", displayName: "GPT-5.6-Terra", description: "Balanced", hidden: false, isDefault: false },
            { model: "gpt-5.6-sol", displayName: "GPT-5.6-Sol", description: "Most capable", hidden: false, isDefault: true },
            { model: "internal", displayName: "Hidden", description: "", hidden: true, isDefault: false },
          ], nextCursor: null };
          case "turn/start": {
            const turnId = `turn-${++turns}`;
            const text = params.input[0].text;
            const notify = (m: string, p: Record<string, unknown>) => h.notification(m, { threadId: params.threadId, turnId, ...p });
            const ctx: Ctx = {
              threadId: params.threadId, turnId, text, notify,
              ask: (m, p) => h.request(m, { threadId: params.threadId, turnId, ...p }, ++rpc),
              complete: (status = "completed", error = null) => notify("turn/completed", { turn: { id: turnId, status, error, durationMs: 12, items: [] } }),
            };
            setTimeout(() => void script(ctx), 0);
            return { turn: { id: turnId, status: "inProgress", items: [] } };
          }
          default: return {};
        }
      },
      close() {},
    };
  };
  return { connect, requests, of: (method: string) => requests.filter((r) => r.method === method), exit: () => handlers.closed() };
}

function gateway(script: Script, options: { idleMs?: number; models?: () => Promise<ModelOption[]> } = {}) {
  const codex = fakeCodex(script);
  const harness = new CodexHarness({ connect: codex.connect, cwd: "/work", idleMs: options.idleMs ?? 60_000 });
  const handler = createGatewayHandler({ secret: SECRET, harness, models: options.models });
  const call = (method: string, path: string, body?: unknown, secret = SECRET) =>
    handler(new Request(`http://gateway.test${path}`, { method, headers: { authorization: `Bearer ${secret}` }, body: body === undefined ? undefined : JSON.stringify(body) }));
  const events = async (response: Response) => (await response.text()).trim().split("\n").map((line) => JSON.parse(line));
  return { codex, call, events };
}

const answer: Script = ({ text, notify, complete }) => {
  notify("item/agentMessage/delta", { itemId: "m1", delta: `echo: ${text}` });
  notify("item/started", { item: { type: "commandExecution", id: "c1", command: "cat a.go", cwd: "/work", status: "inProgress" } });
  notify("item/completed", { item: { type: "commandExecution", id: "c1", command: "cat a.go", cwd: "/work", status: "completed", exitCode: 0, aggregatedOutput: "package main" } });
  notify("item/agentMessage/delta", { itemId: "m2", delta: "All done." });
  notify("item/completed", { item: { type: "agentMessage", id: "m2", text: "All done." } });
  notify("thread/tokenUsage/updated", { tokenUsage: { last: { inputTokens: 10, outputTokens: 5, cachedInputTokens: 2, cacheWriteInputTokens: 0 } } });
  complete();
};

describe("Codex environment", () => {
  test("uses the Codex login and keeps ChenWeb secrets out", () => {
    const env = codexEnv({ PATH: "/bin", HOME: "/Users/me", OPENAI_API_KEY: "k", CODEX_API_KEY: "c", CODEX_GATEWAY_SECRET: "s", PG_PASSWORD: "p", CLAUDE_CODE_GATEWAY_SECRET: "x" });
    expect(env).toEqual({ PATH: "/bin", HOME: "/Users/me" });
  });
});

describe("turns", () => {
  test("starts a Codex thread in the workspace and streams its events", async () => {
    const { codex, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hello" }));
    expect(lines).toEqual([
      { type: "session", sessionId: THREAD, model: "gpt-5.6-sol", cwd: "/work", permissionMode: "default" },
      { type: "text", text: "echo: hello" },
      { type: "tool_use", id: "c1", name: "Shell", input: { command: "cat a.go", cwd: "/work" }, parentToolUseId: null },
      { type: "tool_result", toolUseId: "c1", isError: false, text: "package main" },
      { type: "text", text: "All done." },
      { type: "result", status: "completed", sessionId: THREAD, durationMs: 12, numTurns: 1, costUsd: 0, usage: { inputTokens: 10, outputTokens: 5, cacheReadTokens: 2, cacheWriteTokens: 0 }, text: "All done." },
    ]);
    // Codex's own instructions, sandbox and approval rules: nothing is overridden.
    expect(codex.of("thread/start")[0]!.params).toEqual({ cwd: "/work" });
  });

  test("separate agent messages are kept apart", async () => {
    const { call, events } = gateway(({ notify, complete }) => {
      notify("item/agentMessage/delta", { itemId: "m1", delta: "First." });
      notify("item/agentMessage/delta", { itemId: "m2", delta: "Second." });
      complete();
    });
    const texts = (await events(await call("POST", "/v1/turns", { message: "hi" }))).filter((e) => e.type === "text").map((e) => e.text);
    expect(texts).toEqual(["First.", "\n\n", "Second."]);
  });

  test("a second turn goes to the same thread with only the new message, switching model and sandbox", async () => {
    const { codex, call, events } = gateway(answer);
    await events(await call("POST", "/v1/turns", { message: "first" }));
    const lines = await events(await call("POST", "/v1/turns", { sessionId: THREAD, message: "second", model: "gpt-5.6-terra", permissionMode: "plan" }));
    expect(lines.at(-1).status).toBe("completed");
    expect(codex.of("thread/start")).toHaveLength(1);
    const starts = codex.of("turn/start").map((r) => r.params);
    expect(starts.map((p) => p.input[0].text)).toEqual(["first", "second"]);
    expect(starts[0].model).toBeUndefined();
    expect(starts[1].model).toBe("gpt-5.6-terra");
    expect(starts[1].sandboxPolicy).toEqual({ type: "readOnly", networkAccess: false });
  });

  test("a new thread starts on the chosen model", async () => {
    const { codex, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hi", model: "gpt-5.6-luna" }));
    expect(lines[0].model).toBe("gpt-5.6-luna");
    expect(codex.of("thread/start")[0]!.params.model).toBe("gpt-5.6-luna");
    expect((await call("POST", "/v1/turns", { message: "x", model: "bad model!" })).status).toBe(400);
  });

  test("resumes a workspace thread and refuses others", async () => {
    const { codex, call, events } = gateway(answer);
    expect((await call("POST", "/v1/turns", { sessionId: OTHER, message: "hi" })).status).toBe(404);
    expect((await call("POST", "/v1/turns", { sessionId: FOREIGN, message: "hi" })).status).toBe(404);
    await events(await call("POST", "/v1/turns", { sessionId: THREAD, message: "hi" }));
    expect(codex.of("thread/resume")[0]!.params).toEqual({ threadId: THREAD });
  });

  test("refuses a second turn while one is running, and bypassPermissions", async () => {
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    const { call, events } = gateway(async ({ complete }) => { await held; complete(); });
    const first = await call("POST", "/v1/turns", { message: "slow" });
    await Bun.sleep(5);
    expect((await call("POST", "/v1/turns", { sessionId: THREAD, message: "again" })).status).toBe(409);
    expect((await call("POST", "/v1/turns", { message: "x", permissionMode: "bypassPermissions" })).status).toBe(400);
    release();
    expect((await events(first)).at(-1).status).toBe("completed");
  });

  test("a command approval waits for the user's decision", async () => {
    const decisions: unknown[] = [];
    const { call, events } = gateway(async ({ ask, complete }) => {
      decisions.push(await ask("item/commandExecution/requestApproval", { itemId: "c1", command: "rm -rf build", cwd: "/work", reason: "outside the sandbox" }));
      decisions.push(await ask("item/commandExecution/requestApproval", { itemId: "c2", command: "curl x", cwd: "/work" }));
      complete();
    });
    const response = await call("POST", "/v1/turns", { message: "clean" });
    await Bun.sleep(5);
    expect((await call("POST", `/v1/sessions/${THREAD}/permissions/codex-1`, { behavior: "allow" })).status).toBe(200);
    await Bun.sleep(5);
    expect((await call("POST", `/v1/sessions/${THREAD}/permissions/codex-2`, { behavior: "deny" })).status).toBe(200);
    const lines = await events(response);
    expect(lines[1]).toEqual({ type: "permission_request", requestId: "codex-1", toolName: "Shell", input: { command: "rm -rf build", cwd: "/work" }, reason: "outside the sandbox" });
    expect(decisions).toEqual([{ decision: "accept" }, { decision: "decline" }]);
  });

  test("a file change approval shows the change", async () => {
    let decided: unknown;
    const { call, events } = gateway(async ({ notify, ask, complete }) => {
      notify("item/started", { item: { type: "fileChange", id: "f1", status: "inProgress", changes: [{ path: "/tmp/x.txt", kind: { type: "add" }, diff: "+hi" }] } });
      decided = await ask("item/fileChange/requestApproval", { itemId: "f1", reason: null });
      complete();
    });
    const response = await call("POST", "/v1/turns", { message: "write" });
    await Bun.sleep(5);
    await call("POST", `/v1/sessions/${THREAD}/permissions/codex-1`, { behavior: "allow" });
    const lines = await events(response);
    expect(lines[2]).toEqual({ type: "permission_request", requestId: "codex-1", toolName: "Edit", input: { path: "/tmp/x.txt", changes: [{ path: "/tmp/x.txt", kind: "add", diff: "+hi" }] } });
    expect(decided).toEqual({ decision: "accept" });
  });

  test("Codex's questions are asked like AskUserQuestion and answered by question ID", async () => {
    let answered: unknown;
    const { call, events } = gateway(async ({ ask, complete }) => {
      answered = await ask("item/tool/requestUserInput", { itemId: "q", autoResolutionMs: null, questions: [{ id: "db", header: "Database", question: "Which DB?", isOther: false, isSecret: false, options: [{ label: "Postgres", description: "" }, { label: "MySQL", description: "" }] }] });
      complete();
    });
    const response = await call("POST", "/v1/turns", { message: "set up the db" });
    await Bun.sleep(5);
    await call("POST", `/v1/sessions/${THREAD}/permissions/codex-1`, { behavior: "allow", updatedInput: { answers: { "Which DB?": "Postgres" } } });
    const lines = await events(response);
    expect(lines[1].toolName).toBe("AskUserQuestion");
    expect(lines[1].input.questions[0]).toEqual({ question: "Which DB?", header: "Database", multiSelect: false, options: [{ label: "Postgres", description: "" }, { label: "MySQL", description: "" }] });
    expect(answered).toEqual({ answers: { db: { answers: ["Postgres"] } } });
  });

  test("interrupt stops the turn in Codex and marks it interrupted", async () => {
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    const { codex, call, events } = gateway(async ({ notify, complete }) => { notify("item/agentMessage/delta", { itemId: "m", delta: "working" }); await held; complete("interrupted"); });
    const response = await call("POST", "/v1/turns", { message: "long task" });
    await Bun.sleep(5);
    expect((await call("POST", `/v1/sessions/${THREAD}/interrupt`)).status).toBe(202);
    expect(codex.of("turn/interrupt")[0]!.params).toEqual({ threadId: THREAD, turnId: "turn-1" });
    release();
    expect((await events(response)).at(-1).status).toBe("interrupted");
  });

  test("a failed turn reports Codex's error", async () => {
    const { call, events } = gateway(({ notify, complete }) => {
      notify("error", { willRetry: true, error: { message: "overloaded" } });
      complete("failed", { message: "Usage limit reached" });
    });
    const lines = await events(await call("POST", "/v1/turns", { message: "hi" }));
    expect(lines.slice(1).map((e) => e.type)).toEqual(["status", "error", "result"]);
    expect(lines[2].message).toBe("Usage limit reached");
    expect(lines[3].status).toBe("error");
  });

  test("an app-server exit ends the running turn", async () => {
    const { codex, call, events } = gateway(() => { setTimeout(() => codex.exit(), 5); });
    const lines = await events(await call("POST", "/v1/turns", { message: "hi" }));
    expect(lines.at(-1)).toEqual({ type: "error", message: "Codex stopped unexpectedly." });
  });

  test("an idle thread is unloaded and resumed on the next turn", async () => {
    const { codex, call, events } = gateway(answer, { idleMs: 5 });
    await events(await call("POST", "/v1/turns", { message: "first" }));
    await Bun.sleep(20);
    expect(codex.of("thread/unsubscribe")[0]!.params).toEqual({ threadId: THREAD });
    await events(await call("POST", "/v1/turns", { sessionId: THREAD, message: "later" }));
    expect(codex.of("thread/resume")).toHaveLength(1);
  });
});

describe("models", () => {
  test("lists Codex's own models, default first, and caches them", async () => {
    const { codex, call } = gateway(answer);
    expect(await (await call("GET", "/v1/models")).json()).toEqual({ models: [
      { value: "gpt-5.6-sol", resolvedModel: "gpt-5.6-sol", displayName: "GPT-5.6-Sol", description: "Most capable" },
      { value: "gpt-5.6-terra", resolvedModel: "gpt-5.6-terra", displayName: "GPT-5.6-Terra", description: "Balanced" },
    ] });
    await call("GET", "/v1/models");
    expect(codex.of("model/list")).toHaveLength(1);
  });

  test("reports a failed model listing", async () => {
    const { call } = gateway(answer, { models: async () => { throw new Error("no login"); } });
    expect((await call("GET", "/v1/models")).status).toBe(502);
  });
});

describe("sessions and history", () => {
  test("requires the gateway secret", async () => {
    const { call } = gateway(answer);
    expect((await call("GET", "/v1/sessions", undefined, "wrong")).status).toBe(401);
  });

  test("lists workspace threads and returns history without reasoning", async () => {
    const { codex, call } = gateway(answer);
    expect(await (await call("GET", "/v1/sessions")).json()).toEqual({ sessions: [{ sessionId: THREAD, title: "Fix the build", firstPrompt: "fix it", lastModified: 1_700_000_000_000, gitBranch: "main", live: false }] });
    expect(codex.of("thread/list")[0]!.params).toMatchObject({ cwd: "/work", sourceKinds: ["cli", "vscode", "appServer"] });
    const history = await (await call("GET", `/v1/sessions/${THREAD}/messages`)).json();
    expect(history.messages).toEqual([
      { uuid: "u1", role: "user", parentToolUseId: null, blocks: [{ type: "text", text: "fix it" }] },
      { uuid: "c1", role: "assistant", parentToolUseId: null, blocks: [{ type: "tool_use", id: "c1", name: "Shell", input: { command: "make", cwd: "/work" } }] },
      { uuid: "c1:result", role: "user", parentToolUseId: null, blocks: [{ type: "tool_result", toolUseId: "c1", isError: false, text: "ok" }] },
      { uuid: "a1", role: "assistant", parentToolUseId: null, blocks: [{ type: "text", text: "Done." }] },
    ]);
    expect((await call("GET", `/v1/sessions/${OTHER}/messages`)).status).toBe(404);
    expect((await call("GET", `/v1/sessions/${FOREIGN}/messages`)).status).toBe(404);
  });
});
