import { afterAll, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Connect, RpcHandlers } from "./acp.ts";
import { DeepSeekHarness, modelChoices } from "./harness.ts";
import type { ModelOption } from "./protocol.ts";
import { createGatewayHandler, dshEnv } from "./server.ts";

const SECRET = "gateway-secret";
const NEW = "11111111-2222-4333-8444-555555555555";
const OLD = "22222222-2222-4333-8444-555555555555";
const FOREIGN = "33333333-2222-4333-8444-555555555555";
const CHILD = "44444444-2222-4333-8444-555555555555";
const ANCIENT = "55555555-2222-4333-8444-555555555555";
const EMPTY = "77777777-2222-4333-8444-555555555555";

const FLASH = JSON.stringify(["deepseek-official", "deepseek-v4-flash"]);
const PRO = JSON.stringify(["deepseek-official", "deepseek-v4-pro"]);
const configOptions = (current = FLASH) => [
  { id: "model", name: "Model", category: "model", type: "select", currentValue: current, options: [{ group: "deepseek-official", name: "DeepSeek", options: [
    { value: FLASH, name: "deepseek-v4-flash" },
    { value: PRO, name: "DeepSeek-V4-Pro", description: "Stronger agentic coding" },
  ] }] },
  { id: "reasoning_effort", name: "Reasoning effort", type: "select", currentValue: "high", options: [{ value: "high", name: "High" }] },
];

// dsh session logs, as dsh writes them: zstd-compressed JSONL under <root>/<project>/<id>/.
const root = mkdtempSync(join(tmpdir(), "dsh-gateway-test-"));
afterAll(() => rmSync(root, { recursive: true, force: true }));
function writeLog(id: string, header: Record<string, unknown>, events: Record<string, unknown>[], version = 4) {
  const dir = join(root, "--work--", id);
  mkdirSync(dir, { recursive: true });
  const lines = [{ type: "session", version, id, createdAt: 1, cwd: "/work", isSeeded: false, delegationDepth: 0, ...header }, ...events].map((e) => JSON.stringify(e)).join("\n") + "\n";
  // Two frames, as dsh appends batches.
  const cut = lines.indexOf("\n") + 1;
  writeFileSync(join(dir, `session.v${version}.jsonl.zstd`), Buffer.concat([Bun.zstdCompressSync(lines.slice(0, cut)), Bun.zstdCompressSync(lines.slice(cut))]));
}
writeLog(OLD, {}, [
  { type: "user/message", seq: 1, data: { role: "user", source: { kind: "user" }, content: [{ type: "text", text: "fix the build" }] } },
  { type: "user/message", seq: 2, data: { role: "user", source: { kind: "agent-instructions" }, content: [{ type: "text", text: "<system-reminder>AGENTS.md</system-reminder>" }] } },
  { type: "session/title", seq: 3, data: { title: "Fix the build" } },
  { type: "assistant/message", seq: 4, data: { message: { role: "assistant", content: [{ type: "reasoning", text: "secret reasoning" }, { type: "text", text: "Looking." }, { type: "tool-call", id: "call_1", name: "bash", arguments: "{\"command\":\"make\"}" }] } } },
  { type: "tool/result", seq: 5, data: { message: { role: "tool", toolCallId: "call_1", content: [{ type: "text", text: "ok" }], isError: false } } },
  { type: "assistant/message", seq: 6, data: { message: { role: "assistant", content: [{ type: "text", text: "Done." }] } } },
]);
writeLog(FOREIGN, { cwd: "/elsewhere" }, []);
writeLog(CHILD, { delegationDepth: 1 }, []);
writeLog(ANCIENT, {}, [], 3);
// What dsh keeps for a session that never received a message (such as the one opened to list models).
writeLog(EMPTY, {}, [{ type: "permission/preset", seq: 0, data: { preset: "workspace-write" } }]);

interface Ctx { sessionId: string; text: string; update: (update: Record<string, unknown>) => void; ask: (params: Record<string, unknown>) => Promise<any>; cancelled: Promise<void> }
type Script = (ctx: Ctx) => Promise<string> | string;

/** Stands in for `dsh --profile acp`: records requests and plays a script for each prompt. */
function fakeDsh(script: Script) {
  const requests: { method: string; params: any }[] = [];
  const notes: { method: string; params: any }[] = [];
  let handlers!: RpcHandlers;
  let rpc = 0;
  let sessions = 0;
  const cancels = new Map<string, () => void>();
  // Like the real connection, requests in flight fail when dsh exits.
  const exited = Promise.withResolvers<never>();
  exited.promise.catch(() => undefined);
  const connect: Connect = async (h) => {
    handlers = h;
    return {
      async request(method: string, params: any): Promise<any> {
        requests.push({ method, params });
        switch (method) {
          case "session/new": return { sessionId: ++sessions === 1 ? "99999999-2222-4333-8444-555555555555" : NEW, configOptions: configOptions() };
          case "session/resume": return { configOptions: configOptions() };
          case "session/set_config_option": return { configOptions: configOptions(params.value) };
          case "session/list": return { sessions: [OLD, EMPTY, FOREIGN, ANCIENT].map((sessionId) => ({ sessionId, cwd: "/work" })) };
          case "session/prompt": {
            const sessionId = params.sessionId;
            const cancelled = new Promise<void>((resolve) => cancels.set(sessionId, resolve));
            const ctx: Ctx = {
              sessionId, text: params.prompt[0].text, cancelled,
              update: (update) => h.notification("session/update", { sessionId, update }),
              ask: (p) => h.request("session/request_permission", { sessionId, ...p }, ++rpc),
            };
            return { stopReason: await Promise.race([script(ctx), exited.promise]) };
          }
          default: return {};
        }
      },
      notify(method: string, params: any) { notes.push({ method, params }); if (method === "session/cancel") cancels.get(params.sessionId)?.(); },
      close() {},
    };
  };
  return { connect, requests, notes, of: (method: string) => requests.filter((r) => r.method === method), exit: () => { exited.reject(new Error("DeepSeek Harness exited")); handlers.closed(); } };
}

function gateway(script: Script, options: { idleMs?: number; models?: () => Promise<ModelOption[]> } = {}) {
  const dsh = fakeDsh(script);
  const harness = new DeepSeekHarness({ connect: dsh.connect, cwd: "/work", sessionsRoot: root, idleMs: options.idleMs ?? 60_000 });
  const handler = createGatewayHandler({ secret: SECRET, harness, models: options.models });
  const call = (method: string, path: string, body?: unknown, secret = SECRET) =>
    handler(new Request(`http://gateway.test${path}`, { method, headers: { authorization: `Bearer ${secret}` }, body: body === undefined ? undefined : JSON.stringify(body) }));
  const events = async (response: Response) => (await response.text()).trim().split("\n").map((line) => JSON.parse(line));
  return { dsh, call, events };
}

const answer: Script = ({ text, update }) => {
  update({ sessionUpdate: "agent_message_chunk", messageId: "m1", content: { type: "text", text: `echo: ${text}` } });
  update({ sessionUpdate: "agent_thought_chunk", messageId: "t1", content: { type: "text", text: "secret thought" } });
  update({ sessionUpdate: "agent_message_chunk", messageId: "m2", content: { type: "text", text: "Reading." } });
  update({ sessionUpdate: "tool_call", toolCallId: "call_1", title: "read", kind: "other", status: "in_progress", rawInput: { file_path: "/work/a.go" } });
  update({ sessionUpdate: "tool_call_update", toolCallId: "call_1", status: "completed", content: [{ type: "content", content: { type: "text", text: "package main" } }] });
  update({ sessionUpdate: "usage_update", used: 100, size: 1000 });
  update({ sessionUpdate: "agent_message_chunk", messageId: "m3", content: { type: "text", text: "Done." } });
  return "end_turn";
};

describe("deepseek harness gateway", () => {
  test("requires the secret", async () => {
    const { call } = gateway(answer);
    expect((await call("GET", "/health", undefined, "wrong")).status).toBe(401);
    expect(await (await call("GET", "/health")).json()).toEqual({ status: "ok", harness: "deepseek-harness", cwd: "/work" });
  });

  test("a turn creates a session and relays text and tools, never thoughts", async () => {
    const { call, events, dsh } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hi" }));
    expect(lines[0]).toEqual({ type: "session", sessionId: "99999999-2222-4333-8444-555555555555", model: "deepseek-official:deepseek-v4-flash", cwd: "/work", permissionMode: "default" });
    expect(lines.filter((e) => e.type === "text").map((e) => e.text).join("")).toBe("echo: hi\n\nReading.Done.");
    expect(lines).toContainEqual({ type: "tool_use", id: "call_1", name: "read", input: { file_path: "/work/a.go" }, parentToolUseId: null });
    expect(lines).toContainEqual({ type: "tool_result", toolUseId: "call_1", isError: false, text: "package main" });
    expect(JSON.stringify(lines)).not.toContain("secret thought");
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "completed", text: "Done.", costUsd: 0 });
    expect(dsh.of("session/new")[0]!.params).toEqual({ cwd: "/work", mcpServers: [] });
    expect(dsh.of("session/prompt")[0]!.params.prompt).toEqual([{ type: "text", text: "hi" }]);
  });

  test("a turn on an earlier session resumes it and sends only the new message", async () => {
    const { call, events, dsh } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { sessionId: OLD, message: "and now?" }));
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "completed", sessionId: OLD });
    expect(dsh.of("session/resume")[0]!.params).toEqual({ sessionId: OLD, cwd: "/work", mcpServers: [] });
    expect(dsh.of("session/new")).toHaveLength(0);
    // Live now: the next turn needs no resume.
    await events(await call("POST", "/v1/turns", { sessionId: OLD, message: "again" }));
    expect(dsh.of("session/resume")).toHaveLength(1);
  });

  test("sessions from another folder, subagents and unknown ids are refused", async () => {
    const { call } = gateway(answer);
    for (const id of [FOREIGN, CHILD, "66666666-2222-4333-8444-555555555555"]) {
      expect((await call("POST", "/v1/turns", { sessionId: id, message: "x" })).status).toBe(404);
      expect((await call("GET", `/v1/sessions/${id}/messages`)).status).toBe(404);
    }
  });

  test("a chosen model is set before the prompt", async () => {
    const { call, events, dsh } = gateway(answer);
    const lines = await events(await call("POST", "/v1/turns", { message: "hi", model: "deepseek-official:deepseek-v4-pro" }));
    expect(dsh.of("session/set_config_option")[0]!.params).toMatchObject({ configId: "model", value: PRO });
    expect(lines[0]).toMatchObject({ type: "session", model: "deepseek-official:deepseek-v4-pro" });
    const unknown = await events(await call("POST", "/v1/turns", { message: "hi", model: "nope:model" }));
    expect(unknown).toEqual([{ type: "error", message: "DeepSeek Harness does not offer the model nope:model." }]);
  });

  test("plan and acceptEdits modes are refused without contacting dsh", async () => {
    const { call, events, dsh } = gateway(answer);
    for (const permissionMode of ["plan", "acceptEdits"]) {
      const lines = await events(await call("POST", "/v1/turns", { message: "hi", permissionMode }));
      expect(lines).toHaveLength(1);
      expect(lines[0].type).toBe("error");
    }
    expect(dsh.requests).toHaveLength(0);
  });

  test("an approval shows the matching tool call; allow picks allow_once, deny reject_once", async () => {
    const results: any[] = [];
    const ask: Script = async ({ update, ask }) => {
      update({ sessionUpdate: "tool_call", toolCallId: "call_w", title: "write", kind: "other", status: "in_progress", rawInput: { file_path: "/home/x.txt", content: "hi", justification: "outside the workspace" } });
      const options = [{ optionId: "allow-always", name: "Always", kind: "allow_always" }, { optionId: "allow-once", name: "Allow once", kind: "allow_once" }, { optionId: "reject-once", name: "Reject", kind: "reject_once" }];
      results.push(await ask({ toolCall: { toolCallId: "call_w" }, options }));
      results.push(await ask({ toolCall: { toolCallId: "call_w" }, options }));
      return "end_turn";
    };
    const { call, dsh } = gateway(ask);
    const response = await call("POST", "/v1/turns", { message: "write it" });
    const reader = response.body!.pipeThrough(new TextDecoderStream()).getReader();
    let buffer = "";
    let sessionId = "";
    let answered = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += value;
      for (let nl = buffer.indexOf("\n"); nl >= 0; nl = buffer.indexOf("\n")) {
        const event = JSON.parse(buffer.slice(0, nl));
        buffer = buffer.slice(nl + 1);
        if (event.type === "session") sessionId = event.sessionId;
        if (event.type === "permission_request") {
          expect(event).toMatchObject({ toolName: "write", input: { file_path: "/home/x.txt" }, reason: "outside the workspace" });
          expect(event.requestId).toMatch(/^dsh-\d+$/);
          const behavior = answered++ === 0 ? "allow" : "deny";
          expect((await call("POST", `/v1/sessions/${sessionId}/permissions/${event.requestId}`, { behavior })).status).toBe(200);
        }
      }
    }
    expect(results).toEqual([{ outcome: { outcome: "selected", optionId: "allow-once" } }, { outcome: { outcome: "selected", optionId: "reject-once" } }]);
    expect(dsh.of("session/prompt")).toHaveLength(1);
  });

  test("interrupting cancels the prompt and its open approvals", async () => {
    let asked: Promise<any> | undefined;
    const slow: Script = async ({ update, ask, cancelled }) => {
      update({ sessionUpdate: "tool_call", toolCallId: "call_b", title: "bash", status: "in_progress", rawInput: { command: "rm -rf build" } });
      asked = ask({ toolCall: { toolCallId: "call_b" }, options: [{ optionId: "allow-once", kind: "allow_once" }] });
      await cancelled;
      return "cancelled";
    };
    const { call, events, dsh } = gateway(slow);
    const response = call("POST", "/v1/turns", { message: "clean" });
    await Bun.sleep(20);
    const sessionId = "99999999-2222-4333-8444-555555555555";
    expect((await call("POST", `/v1/sessions/${sessionId}/interrupt`)).status).toBe(202);
    const lines = await events(await response);
    expect(lines.at(-1)).toMatchObject({ type: "result", status: "interrupted" });
    expect(await asked).toEqual({ outcome: { outcome: "cancelled" } });
    expect(dsh.notes).toContainEqual({ method: "session/cancel", params: { sessionId } });
    expect((await call("POST", `/v1/sessions/${sessionId}/interrupt`)).status).toBe(404);
  });

  test("models come from a session's model option, current first", async () => {
    const { call, dsh } = gateway(answer);
    const body = await (await call("GET", "/v1/models")).json();
    expect(body.models).toEqual([
      { value: "deepseek-official:deepseek-v4-flash", resolvedModel: "deepseek-v4-flash", displayName: "deepseek-v4-flash", description: "DeepSeek" },
      { value: "deepseek-official:deepseek-v4-pro", resolvedModel: "deepseek-v4-pro", displayName: "DeepSeek-V4-Pro", description: "Stronger agentic coding" },
    ]);
    expect(dsh.of("session/close")).toHaveLength(1);
    await call("GET", "/v1/models");
    expect(dsh.of("session/new")).toHaveLength(1);
  });

  test("history comes from dsh's log: user text, assistant text and tools, no reasoning or injected context", async () => {
    const { call } = gateway(answer);
    const body = await (await call("GET", `/v1/sessions/${OLD}/messages`)).json();
    expect(body.messages).toEqual([
      { uuid: "1", role: "user", parentToolUseId: null, blocks: [{ type: "text", text: "fix the build" }] },
      { uuid: "4", role: "assistant", parentToolUseId: null, blocks: [{ type: "text", text: "Looking." }, { type: "tool_use", id: "call_1", name: "bash", input: { command: "make" } }] },
      { uuid: "5:result", role: "user", parentToolUseId: null, blocks: [{ type: "tool_result", toolUseId: "call_1", isError: false, text: "ok" }] },
      { uuid: "6", role: "assistant", parentToolUseId: null, blocks: [{ type: "text", text: "Done." }] },
    ]);
    const old = await call("GET", `/v1/sessions/${ANCIENT}/messages`);
    expect(old.status).toBe(502);
    expect((await old.json()).error).toContain("v3");
  });

  test("listing keeps workspace sessions with a conversation, with titles from the log", async () => {
    const { call, events } = gateway(answer);
    const { sessions } = await (await call("GET", "/v1/sessions")).json();
    expect(sessions.map((s: any) => s.sessionId)).toEqual([OLD, ANCIENT]);
    expect(sessions[0]).toMatchObject({ title: "Fix the build", firstPrompt: "fix the build", live: false });
    expect(sessions[0].lastModified).toBeGreaterThan(0);
    expect((await (await call("GET", "/v1/sessions?limit=1&offset=1")).json()).sessions.map((s: any) => s.sessionId)).toEqual([ANCIENT]);
    // dsh does not list sessions open on its connection; the gateway adds them first.
    await events(await call("POST", "/v1/turns", { sessionId: OLD, message: "more" }));
    const after = (await (await call("GET", "/v1/sessions")).json()).sessions;
    expect(after.map((s: any) => [s.sessionId, s.live])).toEqual([[OLD, true], [ANCIENT, false]]);
  });

  test("dsh exiting ends the running turn with an error", async () => {
    let exit = () => {};
    const hang: Script = () => new Promise(() => { exit(); });
    const g = gateway(hang);
    exit = () => setTimeout(() => g.dsh.exit(), 5);
    const lines = await g.events(await g.call("POST", "/v1/turns", { message: "hi" }));
    expect(lines.some((e) => e.type === "error" && e.message.includes("stopped unexpectedly"))).toBe(true);
  });

  test("idle sessions are closed in dsh", async () => {
    const { call, events, dsh } = gateway(answer, { idleMs: 10 });
    await events(await call("POST", "/v1/turns", { message: "hi" }));
    await Bun.sleep(40);
    expect(dsh.of("session/close").map((r) => r.params.sessionId)).toEqual(["99999999-2222-4333-8444-555555555555"]);
  });
});

describe("dsh environment", () => {
  test("drops keys, secrets and the gateway's own settings", () => {
    expect(dshEnv({ PATH: "/bin", DEEPSEEK_API_KEY: "k", DB_PASSWORD: "p", DEEPSEEK_HARNESS_GATEWAY_SECRET: "s", DEEPSEEK_HARNESS_WORKSPACE: "/w", DSH_HOME: "/h" }))
      .toEqual({ PATH: "/bin", DSH_HOME: "/h" });
  });
});

describe("model choices", () => {
  test("skips values that are not provider/model pairs", () => {
    const { choices } = modelChoices([{ id: "model", currentValue: "x", options: [{ value: "plain", name: "Plain" }, { value: FLASH, name: "Flash" }] }]);
    expect(choices.map((c) => c.value)).toEqual(["deepseek-official:deepseek-v4-flash"]);
  });
});
