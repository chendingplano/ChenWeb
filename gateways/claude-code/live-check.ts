// Live check (not part of `bun test`): two turns through the gateway on the real
// Claude Code harness with the machine's Claude Code login.
//   CLAUDE_CODE_WORKSPACE=/path/to/folder bun run live-check.ts
import { query } from "@anthropic-ai/claude-agent-sdk";
import { claudeCodeEnv, createGatewayHandler } from "./server.ts";

const cwd = process.env.CLAUDE_CODE_WORKSPACE ?? process.cwd();
const handler = createGatewayHandler({ secret: "live-check", environment: { query, cwd, env: claudeCodeEnv(process.env, false), idleMs: 60_000 } });
const call = (method: string, path: string, body?: unknown) =>
  handler(new Request(`http://gateway.local${path}`, { method, headers: { authorization: "Bearer live-check" }, body: body === undefined ? undefined : JSON.stringify(body) }));
async function turn(body: unknown) {
  const lines = (await (await call("POST", "/v1/turns", body)).text()).trim().split("\n").map((l) => JSON.parse(l));
  const answer = lines.filter((e) => e.type === "text").map((e) => e.text).join("");
  for (const e of lines) if (e.type !== "text") console.log("  ", JSON.stringify(e).slice(0, 220));
  console.log("   answer:", JSON.stringify(answer));
  return lines;
}
console.log("turn 1");
const first = await turn({ message: "Remember the codeword PELICAN-42. Also tell me the first line of ChenWeb/go.mod. Keep the reply to one line." });
const sessionId = first.find((e) => e.type === "session")?.sessionId;
console.log("turn 2 (same session)");
await turn({ sessionId, message: "What was the codeword I gave you? Reply with the codeword only." });
const history = await (await call("GET", `/v1/sessions/${sessionId}/messages`)).json();
console.log("history messages from Claude Code:", history.messages.length);
const listed = await (await call("GET", "/v1/sessions?limit=3")).json();
console.log("session listed:", listed.sessions.some((s: { sessionId: string }) => s.sessionId === sessionId));
process.exit(0);
