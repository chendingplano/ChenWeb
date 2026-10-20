// Live check (not part of `bun test`): turns through the gateway on the real
// DeepSeek Harness with dsh's own login. The third turn writes a file outside
// the workspace, which dsh asks about; the check approves exactly that write
// and denies anything else.
//   DEEPSEEK_HARNESS_WORKSPACE=/path/to/folder DEEPSEEK_HARNESS_BIN=/path/to/dsh bun run live-check.ts
import { existsSync, readFileSync, realpathSync, rmSync } from "node:fs";
import { homedir } from "node:os";
import { acpServer } from "./acp.ts";
import { DeepSeekHarness } from "./harness.ts";
import { createGatewayHandler, dshEnv, sessionsRoot } from "./server.ts";

const cwd = realpathSync(process.env.DEEPSEEK_HARNESS_WORKSPACE ?? process.cwd());
const harness = new DeepSeekHarness({ connect: acpServer({ bin: process.env.DEEPSEEK_HARNESS_BIN || "dsh", cwd, env: dshEnv(process.env) }), cwd, sessionsRoot: sessionsRoot(process.env), idleMs: 60_000 });
const handler = createGatewayHandler({ secret: "live-check", harness });
const call = (method: string, path: string, body?: unknown) =>
  handler(new Request(`http://gateway.local${path}`, { method, headers: { authorization: "Bearer live-check" }, body: body === undefined ? undefined : JSON.stringify(body) }));
// Outside the workspace, so dsh's workspace-write sandbox asks first.
const target = `${homedir()}/chenweb-dsh-gateway-live-check.txt`;
async function turn(body: { sessionId?: string; message: string; model?: string }) {
  const response = await call("POST", "/v1/turns", body);
  const lines: any[] = [];
  const reader = response.body!.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = "";
  let sessionId = body.sessionId;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += value;
    let newline: number;
    while ((newline = buffer.indexOf("\n")) >= 0) {
      const event = JSON.parse(buffer.slice(0, newline));
      buffer = buffer.slice(newline + 1);
      lines.push(event);
      if (event.type === "session") sessionId = event.sessionId;
      if (event.type !== "text") console.log("  ", JSON.stringify(event).slice(0, 240));
      if (event.type === "permission_request") {
        // Approve only the expected probe write, as a user clicking Allow would; deny anything else.
        const expected = event.toolName === "write" && event.input?.file_path === target;
        console.log("   decision:", expected ? "allow" : "deny");
        void call("POST", `/v1/sessions/${sessionId}/permissions/${event.requestId}`, { behavior: expected ? "allow" : "deny" });
      }
    }
  }
  console.log("   answer:", JSON.stringify(lines.filter((e) => e.type === "text").map((e) => e.text).join("")));
  return lines;
}
const models = (await (await call("GET", "/v1/models")).json()).models.map((m: { value: string }) => m.value);
console.log("models:", models.join(", "));
console.log("turn 1");
// Like the chat page: the model comes from dsh's own list.
const first = await turn({ model: models[0], message: "Remember the codeword PELICAN-42. Also tell me the first line of ChenWeb/go.mod. Keep the reply to one line." });
const sessionId = first.find((e) => e.type === "session")?.sessionId;
console.log("turn 2 (same session)");
await turn({ sessionId, message: "What was the codeword I gave you? Reply with the codeword only." });
rmSync(target, { force: true });
console.log("turn 3 (write outside the workspace, approved)");
await turn({ sessionId, message: `Use the write tool to create ${target} containing the single word ok. If the sandbox refuses, retry once with sandbox_permissions so that I am asked to approve.` });
console.log("file written:", existsSync(target) && readFileSync(target, "utf8").trim() === "ok");
rmSync(target, { force: true });
const history = await (await call("GET", `/v1/sessions/${sessionId}/messages`)).json();
console.log("history messages from dsh's log:", history.messages?.length, history.messages?.slice(0, 2).map((m: any) => m.blocks[0]?.text?.slice(0, 60)));
const listed = await (await call("GET", "/v1/sessions?limit=5")).json();
console.log("session listed:", JSON.stringify(listed.sessions.find((s: { sessionId: string }) => s.sessionId === sessionId)));
harness.close();
await Bun.sleep(1500);
process.exit(0);
