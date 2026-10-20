import { afterAll, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, utimesSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { PiHarness } from "./harness.ts";
import type { ModelOption } from "./protocol.ts";
import type { PiHandlers, Spawn } from "./rpc.ts";
import { createGatewayHandler, piEnv } from "./server.ts";
import { sessionDir } from "./sessionfile.ts";

const SECRET = "gateway-secret";
const NEW = "11111111-2222-4333-8444-555555555555";
const OLD = "22222222-2222-4333-8444-555555555555";
const FOREIGN = "33333333-2222-4333-8444-555555555555";
const ANCIENT = "55555555-2222-4333-8444-555555555555";
const NAMED = "66666666-2222-4333-8444-555555555555";

// Pi session files, as Pi writes them: <agent dir>/sessions/--<cwd>--/<timestamp>_<id>.jsonl.
const agent = mkdtempSync(join(tmpdir(), "pi-gateway-test-"));
afterAll(() => rmSync(agent, { recursive: true, force: true }));
const dir = sessionDir(agent, "/work");
mkdirSync(dir, { recursive: true });
function writeSession(id: string, header: Record<string, unknown>, entries: Record<string, unknown>[], modified: number) {
  const path = join(dir, `2026-10-20T10-00-00-000Z_${id}.jsonl`);
  writeFileSync(path, [{ type: "session", version: 3, id, timestamp: "2026-10-20T10:00:00.000Z", cwd: "/work", ...header }, ...entries].map((e) => JSON.stringify(e)).join("\n") + "\n");
  utimesSync(path, modified, modified);
  return path;
}
const msg = (id: string, parentId: string | null, message: Record<string, unknown>) => ({ type: "message", id, parentId, timestamp: "2026-10-20T10:00:01.000Z", message });
const OLD_PATH = writeSession(OLD, {}, [
  { type: "model_change", id: "e0", parentId: null, provider: "deepseek", modelId: "deepseek-v4-flash" },
  msg("e1", "e0", { role: "user", content: [{ type: "text", text: "fix the build" }] }),
  msg("e2", "e1", { role: "assistant", content: [{ type: "thinking", thinking: "secret thinking" }, { type: "text", text: "Looking." }, { type: "toolCall", id: "call_1", name: "bash", arguments: { command: "make" } }], stopReason: "toolUse" }),
  msg("e3", "e2", { role: "toolResult", toolCallId: "call_1", toolName: "bash", content: [{ type: "text", text: "ok" }], isError: false }),
  msg("e4", "e3", { role: "assistant", content: [{ type: "text", text: "Abandoned branch." }], stopReason: "stop" }),
  // Branched with /tree from e3: e4 is no longer on the active branch.
  msg("e5", "e3", { role: "assistant", content: [{ type: "text", text: "Done." }], stopReason: "stop" }),
  { type: "custom_message", id: "e6", parentId: "e5", customType: "ext", content: "injected", display: false },
  msg("e7", "e6", { role: "bashExecution", command: "ls", output: "a.go", exitCode: 0, cancelled: false, truncated: false }),
], 2_000);
writeSession(FOREIGN, { cwd: "/elsewhere" }, [msg("f1", null, { role: "user", content: "hi" })], 3_000);
writeSession(ANCIENT, { version: 2 }, [msg("a1", null, { role: "user", content: "hi" })], 1_000);
writeSession(NAMED, {}, [msg("n1", null, { role: "user", content: "first words" }), { type: "session_info", id: "n2", parentId: "n1", name: "Refactor auth" }], 1_500);
// What Pi keeps of a session that has no message.
writeSession("77777777-2222-4333-8444-555555555555", {}, [{ type: "model_change", id: "m0", parentId: null, provider: "deepseek", modelId: "x" }], 4_000);

interface Ctx { message: string; args: string[]; emit: (event: Record<string, unknown>) => void; ask: (request: Record<string, unknown>) => Promise<any>; aborted: Promise<void> }
/** Plays one prompt. Returning "command" answers like an extension command (no agent run). */
type Script = (ctx: Ctx) => Promise<string | void> | string | void;

const FLASH = { provider: "deepseek", id: "deepseek-v4-flash", name: "DeepSeek V4 Flash" };
const PRO = { provider: "deepseek", id: "deepseek-v4-pro", name: "DeepSeek V4 Pro" };
const SLASHED = { provider: "openrouter", id: "anthropic/claude", name: "Routed" };

/** Stands in for `pi --mode rpc`: records spawns and commands, and plays a script for each prompt. */
function fakePi(script: Script) {
  const spawns: string[][] = [];
  const commands: Record<string, any>[] = [];
  const sent: Record<string, any>[] = [];
  const procs: { handlers: PiHandlers; closed: boolean }[] = [];
  let ui = 0;
  const spawn: Spawn = async (args, handlers) => {
    spawns.push(args);
    const proc = { handlers, closed: false };
    procs.push(proc);
    const id = args[0] === "--session" ? (args[1]!.match(/_([0-9a-f-]{36})\.jsonl$/)![1]!) : NEW;
    let model = FLASH;
    let streaming = false;
    const answers = new Map<string, (value: any) => void>();
    let abort = () => {};
    return {
      async command(command: Record<string, any>): Promise<any> {
        commands.push(command);
        switch (command.type) {
          case "get_state": return { sessionId: id, model, isStreaming: streaming, isCompacting: false };
          case "get_available_models": return { models: [PRO, FLASH, SLASHED] };
          case "set_model": {
            const found = [FLASH, PRO].find((m) => m.provider === command.provider && m.id === command.modelId);
            if (!found) throw new Error(`Model not found: ${String(command.provider)}/${String(command.modelId)}`);
            model = found;
            return found;
          }
          case "abort": abort(); return undefined;
          case "prompt": {
            const aborted = new Promise<void>((resolve) => { abort = resolve; });
            const ctx: Ctx = {
              message: String(command.message), args, aborted,
              emit: (event) => handlers.event(event),
              ask: (request) => new Promise((resolve) => { const uiId = `ui-${++ui}`; answers.set(uiId, resolve); handlers.uiRequest({ type: "extension_ui_request", id: uiId, ...request }); }),
            };
            // An extension command is finished before Pi acknowledges the prompt; a run settles later.
            const outcome = script(ctx);
            if (outcome === "command") return undefined;
            streaming = true;
            void Promise.resolve(outcome).then(() => { streaming = false; handlers.event({ type: "agent_settled" }); });
            return undefined;
          }
          default: return undefined;
        }
      },
      send(message: Record<string, any>) { sent.push(message); answers.get(message.id)?.(message); },
      close() { proc.closed = true; },
    };
  };
  return { spawn, spawns, commands, sent, procs, of: (type: string) => commands.filter((c) => c.type === type), exit: () => procs.at(-1)!.handlers.closed() };
}

function gateway(script: Script, options: { idleMs?: number; models?: () => Promise<ModelOption[]> } = {}) {
  const pi = fakePi(script);
  const harness = new PiHarness({ spawn: pi.spawn, cwd: "/work", sessionDir: dir, idleMs: options.idleMs ?? 60_000 });
  const handler = createGatewayHandler({ secret: SECRET, harness, models: options.models });
  const call = (method: string, path: string, body?: unknown, secret = SECRET) =>
    handler(new Request(`http://gateway.test${path}`, { method, headers: { authorization: `Bearer ${secret}` }, body: body === undefined ? undefined : JSON.stringify(body) }));
  const events = async (response: Response) => (await response.text()).trim().split("\n").map((line) => JSON.parse(line));
  return { pi, harness, call, events };
}

const assistant = (text: string, usage = { input: 10, output: 5, cacheRead: 2, cacheWrite: 1, cost: { total: 0.01 } }, stopReason = "stop") =>
  ({ role: "assistant", content: [{ type: "text", text }], usage, stopReason });

const answer: Script = ({ message, emit }) => {
  emit({ type: "agent_start" });
  emit({ type: "message_start", message: { role: "assistant" } });
  emit({ type: "message_update", assistantMessageEvent: { type: "thinking_delta", delta: "secret thought" } });
  emit({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "echo: " } });
  emit({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: message } });
  emit({ type: "message_end", message: { ...assistant(`echo: ${message}`), stopReason: "toolUse" } });
  emit({ type: "tool_execution_start", toolCallId: "call_1", toolName: "read", args: { path: "/work/a.go" } });
  emit({ type: "tool_execution_end", toolCallId: "call_1", toolName: "read", result: { content: [{ type: "text", text: "package main" }] }, isError: false });
  emit({ type: "turn_end" });
  emit({ type: "message_start", message: { role: "assistant" } });
  emit({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "Done." } });
  emit({ type: "message_end", message: assistant("Done.") });
  emit({ type: "turn_end" });
};

describe("pi gateway", () => {
  test("requires the secret", async () => {
    const { call } = gateway(answer);
    expect((await call("GET", "/health", undefined, "wrong")).status).toBe(401);
    expect(await (await call("GET", "/health")).json()).toMatchObject({ harness: "pi", cwd: "/work" });
  });

  test("lists Pi's models from a process that saves no session, the default first", async () => {
    const { pi, call } = gateway(answer);
    const body = await (await call("GET", "/v1/models")).json();
    expect(body.models).toEqual([
      { value: "deepseek:deepseek-v4-flash", resolvedModel: "deepseek-v4-flash", displayName: "DeepSeek V4 Flash", description: "deepseek" },
      { value: "deepseek:deepseek-v4-pro", resolvedModel: "deepseek-v4-pro", displayName: "DeepSeek V4 Pro", description: "deepseek" },
    ]);
    expect(pi.spawns).toEqual([["--no-session"]]);
    expect(pi.procs[0]!.closed).toBe(true);
    await call("GET", "/v1/models");
    expect(pi.spawns.length).toBe(1);
  });

  test("streams a new session's turn: text, tool use, usage and cost, never thinking", async () => {
    const { pi, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hello" }));
    expect(pi.spawns).toEqual([[]]);
    expect(lines[0]).toEqual({ type: "session", sessionId: NEW, model: "deepseek:deepseek-v4-flash", cwd: "/work", permissionMode: "default" });
    expect(lines.filter((e) => e.type === "text").map((e) => e.text).join("")).toBe("echo: hello\n\nDone.");
    expect(JSON.stringify(lines)).not.toContain("secret");
    expect(lines).toContainEqual({ type: "tool_use", id: "call_1", name: "read", input: { path: "/work/a.go" }, parentToolUseId: null });
    expect(lines).toContainEqual({ type: "tool_result", toolUseId: "call_1", isError: false, text: "package main" });
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "completed", sessionId: NEW, numTurns: 2, text: "Done.",
      usage: { inputTokens: 20, outputTokens: 10, cacheReadTokens: 4, cacheWriteTokens: 2 } });
    expect(lines.at(-1).costUsd).toBeCloseTo(0.02);
    expect(pi.of("prompt")).toEqual([{ type: "prompt", message: "hello" }]);
  });

  test("a live session keeps its process and sends only the new message", async () => {
    const { pi, call, events } = gateway(answer);
    await events(await call("POST", "/v1/turns", { message: "one" }));
    await events(await call("POST", "/v1/turns", { sessionId: NEW, message: "two" }));
    expect(pi.spawns.length).toBe(1);
    expect(pi.of("prompt").map((c) => c.message)).toEqual(["one", "two"]);
  });

  test("resumes a workspace session from its file in a new Pi process", async () => {
    const { pi, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { sessionId: OLD, message: "continue" }));
    expect(pi.spawns).toEqual([["--session", OLD_PATH]]);
    expect(lines[0]).toMatchObject({ type: "session", sessionId: OLD });
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "completed" });
  });

  test("refuses sessions outside the workspace, unknown ones and unsupported files", async () => {
    const { pi, call } = gateway(answer);
    expect((await call("POST", "/v1/turns", { sessionId: FOREIGN, message: "x" })).status).toBe(404);
    expect((await call("POST", "/v1/turns", { sessionId: "99999999-2222-4333-8444-555555555555", message: "x" })).status).toBe(404);
    expect((await call("GET", `/v1/sessions/${FOREIGN}/messages`)).status).toBe(404);
    expect((await call("GET", "/v1/sessions/not-a-session/messages")).status).toBe(404);
    const ancient = await call("GET", `/v1/sessions/${ANCIENT}/messages`);
    expect(ancient.status).toBe(502);
    expect((await ancient.json()).error).toBe("Pi session format v2 is not supported");
    expect(pi.spawns).toEqual([]);
  });

  test("reads history from Pi's file: the active branch, no thinking or injected messages", async () => {
    const { call } = gateway(answer);
    const body = await (await call("GET", `/v1/sessions/${OLD}/messages`)).json();
    expect(body.messages).toEqual([
      { uuid: "e1", role: "user", parentToolUseId: null, blocks: [{ type: "text", text: "fix the build" }] },
      { uuid: "e2", role: "assistant", parentToolUseId: null, blocks: [{ type: "text", text: "Looking." }, { type: "tool_use", id: "call_1", name: "bash", input: { command: "make" } }] },
      { uuid: "e3", role: "user", parentToolUseId: null, blocks: [{ type: "tool_result", toolUseId: "call_1", isError: false, text: "ok" }] },
      { uuid: "e5", role: "assistant", parentToolUseId: null, blocks: [{ type: "text", text: "Done." }] },
      { uuid: "e7", role: "user", parentToolUseId: null, blocks: [{ type: "tool_use", id: "e7", name: "bash", input: { command: "ls" } }, { type: "tool_result", toolUseId: "e7", isError: false, text: "a.go" }] },
    ]);
    const page = await (await call("GET", `/v1/sessions/${OLD}/messages?limit=1&offset=1`)).json();
    expect(page.messages.map((m: { uuid: string }) => m.uuid)).toEqual(["e2"]);
  });

  test("lists workspace sessions that have a conversation, open ones first", async () => {
    const { call, events } = gateway(answer);
    await events(await call("POST", "/v1/turns", { message: "brand new" }));
    const body = await (await call("GET", "/v1/sessions")).json();
    // ANCIENT is in a format the gateway does not read: listed without a title.
    expect(body.sessions.map((s: { sessionId: string }) => s.sessionId)).toEqual([NEW, OLD, NAMED, ANCIENT]);
    expect(body.sessions[0]).toMatchObject({ title: "brand new", live: true });
    expect(body.sessions[1]).toEqual({ sessionId: OLD, title: "fix the build", firstPrompt: "fix the build", lastModified: 2_000_000, live: false });
    expect(body.sessions[2]).toMatchObject({ title: "Refactor auth", firstPrompt: "first words" });
    const page = await (await call("GET", "/v1/sessions?limit=1&offset=1")).json();
    expect(page.sessions.map((s: { sessionId: string }) => s.sessionId)).toEqual([OLD]);
  });

  test("an extension's confirm dialog becomes an approval", async () => {
    let confirmed: unknown;
    const { pi, call } = gateway(async ({ emit, ask }) => {
      emit({ type: "agent_start" });
      confirmed = (await ask({ method: "confirm", title: "Allow rm -rf build?", message: "The command deletes files." })).confirmed;
      emit({ type: "message_end", message: assistant("ok") });
    });
    const response = await call("POST", "/v1/turns", { message: "clean" });
    const reader = response.body!.pipeThrough(new TextDecoderStream()).getReader();
    let text = "";
    while (!text.includes("permission_request")) text += (await reader.read()).value;
    const request = JSON.parse(text.trim().split("\n").find((l) => l.includes("permission_request"))!);
    expect(request).toEqual({ type: "permission_request", requestId: "pi-1", toolName: "confirm", input: { description: "The command deletes files." }, title: "Allow rm -rf build?", description: "The command deletes files." });
    expect((await call("POST", `/v1/sessions/${NEW}/permissions/pi-1`, { behavior: "allow" })).status).toBe(200);
    while (!(await reader.read()).done);
    expect(confirmed).toBe(true);
    expect(pi.sent).toEqual([{ type: "extension_ui_response", id: "ui-1", confirmed: true }]);
  });

  test("a select dialog becomes a question; free-text dialogs are cancelled", async () => {
    const { pi, call, events } = gateway(async ({ emit, ask }) => {
      emit({ type: "agent_start" });
      await ask({ method: "input", title: "Name?" });
      await ask({ method: "select", title: "Which branch?", options: ["main", "dev"] });
      ask({ method: "notify", message: "fyi" });
    });
    const response = call("POST", "/v1/turns", { message: "pick" });
    await Bun.sleep(10);
    expect((await call("POST", `/v1/sessions/${NEW}/permissions/pi-1`, { behavior: "allow", updatedInput: { answers: { "Which branch?": "dev" } } })).status).toBe(200);
    const lines = await events(await response);
    expect(lines).toContainEqual({ type: "permission_request", requestId: "pi-1", toolName: "AskUserQuestion",
      input: { questions: [{ question: "Which branch?", header: "Pi", multiSelect: false, options: [{ label: "main", description: "" }, { label: "dev", description: "" }] }] } });
    expect(pi.sent).toEqual([{ type: "extension_ui_response", id: "ui-1", cancelled: true }, { type: "extension_ui_response", id: "ui-2", value: "dev" }]);
  });

  test("interrupting aborts Pi's run and cancels open dialogs", async () => {
    const { pi, call, events } = gateway(async ({ emit, ask, aborted }) => {
      emit({ type: "agent_start" });
      void ask({ method: "confirm", title: "Allow?" });
      await aborted;
      emit({ type: "message_end", message: { ...assistant(""), stopReason: "aborted" } });
    });
    const response = call("POST", "/v1/turns", { message: "long job" });
    await Bun.sleep(10);
    expect((await call("POST", `/v1/sessions/${NEW}/interrupt`)).status).toBe(202);
    const lines = await events(await response);
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "interrupted" });
    expect(pi.of("abort").length).toBe(1);
    expect(pi.sent).toEqual([{ type: "extension_ui_response", id: "ui-1", cancelled: true }]);
    expect((await call("POST", `/v1/sessions/${NEW}/interrupt`)).status).toBe(404);
  });

  test("a model from Pi's list is set on the session; an unknown one is an error", async () => {
    const { pi, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hi", model: "deepseek:deepseek-v4-pro" }));
    expect(pi.of("set_model")).toEqual([{ type: "set_model", provider: "deepseek", modelId: "deepseek-v4-pro" }]);
    expect(lines[0]).toMatchObject({ type: "session", model: "deepseek:deepseek-v4-pro" });
    await events(await call("POST", "/v1/turns", { sessionId: NEW, message: "again", model: "deepseek:deepseek-v4-pro" }));
    expect(pi.of("set_model").length).toBe(1);
    const bad = await events(await call("POST", "/v1/turns", { sessionId: NEW, message: "x", model: "deepseek:nope" }));
    expect(bad).toEqual([{ type: "error", message: "Pi does not offer the model deepseek:nope." }]);
  });

  test("refuses permission modes Pi does not have, before starting Pi", async () => {
    const { pi, call, events } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "plan it", permissionMode: "plan" }));
    expect(lines).toEqual([{ type: "error", message: 'Pi has no "plan" mode; only the default mode runs.' }]);
    expect(pi.spawns).toEqual([]);
  });

  test("an extension command answered without a run ends the turn", async () => {
    const { call, events } = gateway(() => "command");
    const lines = await events(await call("POST", "/v1/turns", { message: "/name Work" }));
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "completed", numTurns: 0 });
  });

  test("a failed model call ends the turn with Pi's error", async () => {
    const { call, events } = gateway(({ emit }) => {
      emit({ type: "agent_start" });
      emit({ type: "message_end", message: { ...assistant(""), stopReason: "error", errorMessage: "No API key for deepseek" } });
    });
    const lines = await events(await call("POST", "/v1/turns", { message: "hi" }));
    expect(lines.slice(-2)).toMatchObject([{ type: "error", message: "No API key for deepseek" }, { type: "result", status: "error" }]);
  });

  test("Pi exiting mid-turn ends it with an error; the next turn starts a new process", async () => {
    let gw!: ReturnType<typeof gateway>;
    gw = gateway(({ emit }) => { emit({ type: "agent_start" }); gw.pi.exit(); return new Promise(() => {}); });
    const lines = await gw.events(await gw.call("POST", "/v1/turns", { message: "hi" }));
    expect(lines.slice(-2)).toMatchObject([{ type: "error", message: "Pi stopped unexpectedly." }, { type: "result", status: "error" }]);
    // Its file was never written (no reply arrived), so it cannot be resumed.
    expect((await gw.call("POST", "/v1/turns", { sessionId: NEW, message: "again" })).status).toBe(404);
  });

  test("an idle session's process is stopped and resumed from its file", async () => {
    const { pi, call, events } = gateway(answer, { idleMs: 5 });
    await events(await call("POST", "/v1/turns", { sessionId: OLD, message: "one" }));
    await Bun.sleep(20);
    expect(pi.procs[0]!.closed).toBe(true);
    await events(await call("POST", "/v1/turns", { sessionId: OLD, message: "two" }));
    expect(pi.spawns).toEqual([["--session", OLD_PATH], ["--session", OLD_PATH]]);
  });

  test("Pi gets neither the gateway's settings nor ChenWeb's secrets", () => {
    const env = piEnv({ PATH: "/bin", PI_HARNESS_GATEWAY_SECRET: "s", PI_HARNESS_WORKSPACE: "/w", ANTHROPIC_API_KEY: "k", DASHSCOPE_API_KEY: "d",
      OPENMETADATA_ADMIN_TOKEN: "t", TEST_DATABASE_URL: "postgres://u:p@h/db", PROMPT_OPTIMIZER_ENCRYPTION_KEY: "e", PI_CODING_AGENT_DIR: "/a" }, ["DASHSCOPE_API_KEY"]);
    expect(env).toEqual({ PATH: "/bin", DASHSCOPE_API_KEY: "d", PI_CODING_AGENT_DIR: "/a" });
  });
});
