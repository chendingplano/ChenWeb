// Live check (not part of `bun test`): turns through the gateway on the real
// Codex harness with the machine's Codex login. The third turn writes a file
// outside the workspace, which Codex asks about; the check approves it.
//   CODEX_WORKSPACE=/path/to/folder bun run live-check.ts
import { existsSync, realpathSync, rmSync } from "node:fs";
import { homedir } from "node:os";
import { appServer } from "./appserver.ts";
import { CodexHarness } from "./harness.ts";
import { codexEnv, createGatewayHandler } from "./server.ts";

const cwd = realpathSync(process.env.CODEX_WORKSPACE ?? process.cwd());
const harness = new CodexHarness({ connect: appServer({ bin: process.env.CODEX_BIN || "codex", cwd, env: codexEnv(process.env) }), cwd, idleMs: 60_000 });
const handler = createGatewayHandler({ secret: "live-check", harness });
const call = (method: string, path: string, body?: unknown) =>
  handler(new Request(`http://gateway.local${path}`, { method, headers: { authorization: "Bearer live-check" }, body: body === undefined ? undefined : JSON.stringify(body) }));
async function turn(body: { sessionId?: string; message: string; model?: string }) {
  const response = await call("POST", "/v1/turns", body);
  const lines: any[] = [];
  const reader = response.body!.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += value;
    let newline: number;
    while ((newline = buffer.indexOf("\n")) >= 0) {
      const event = JSON.parse(buffer.slice(0, newline));
      buffer = buffer.slice(newline + 1);
      lines.push(event);
      if (event.type !== "text") console.log("  ", JSON.stringify(event).slice(0, 220));
      // Approve what Codex asks, as a user clicking Allow would.
      if (event.type === "permission_request") void call("POST", `/v1/sessions/${body.sessionId}/permissions/${event.requestId}`, { behavior: "allow" });
    }
  }
  console.log("   answer:", JSON.stringify(lines.filter((e) => e.type === "text").map((e) => e.text).join("")));
  return lines;
}
const models = (await (await call("GET", "/v1/models")).json()).models.map((m: { value: string }) => m.value);
console.log("models:", models.join(", "));
console.log("turn 1");
// Like the chat page: the model comes from Codex's own list.
const first = await turn({ model: models[0], message: "Remember the codeword PELICAN-42. Also tell me the first line of ChenWeb/go.mod. Keep the reply to one line." });
const sessionId = first.find((e) => e.type === "session")?.sessionId;
console.log("turn 2 (same session)");
await turn({ sessionId, message: "What was the codeword I gave you? Reply with the codeword only." });
// Outside the workspace and outside the sandbox's writable temp folders.
const target = `${homedir()}/chenweb-codex-gateway-live-check.txt`;
rmSync(target, { force: true });
console.log("turn 3 (write outside the workspace, approved)");
await turn({ sessionId, message: `Create the file ${target} containing the single word ok. Use a shell command or a patch, whichever you prefer.` });
console.log("file written:", existsSync(target));
const history = await (await call("GET", `/v1/sessions/${sessionId}/messages`)).json();
console.log("history messages from Codex:", history.messages.length);
const listed = await (await call("GET", "/v1/sessions?limit=5")).json();
console.log("session listed:", listed.sessions.some((s: { sessionId: string }) => s.sessionId === sessionId));
harness.close();
process.exit(0);
