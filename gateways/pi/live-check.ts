// Live check (not part of `bun test`): turns through the gateway on the real
// Pi with Pi's own login. Pi asks before nothing by itself, so the check loads
// one probe extension (`-e`, for this check only) that asks, as Pi's
// permission-gate example does, before a write to the probe file. The third
// turn makes that write; the check approves exactly it and denies anything else.
//   PI_HARNESS_WORKSPACE=/path/to/folder [PI_HARNESS_BIN=pi] bun run live-check.ts
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { join } from "node:path";
import { PiHarness } from "./harness.ts";
import { piProcess } from "./rpc.ts";
import { agentDir, createGatewayHandler, passList, piEnv } from "./server.ts";
import { sessionDir } from "./sessionfile.ts";

const cwd = realpathSync(process.env.PI_HARNESS_WORKSPACE ?? process.cwd());
const target = `${homedir()}/chenweb-pi-gateway-live-check.txt`;
const probeDir = mkdtempSync(join(tmpdir(), "pi-gateway-live-check-"));
const probe = join(probeDir, "probe-gate.ts");
writeFileSync(probe, `export default function (pi) {
  pi.on("tool_call", async (event, ctx) => {
    if (event.toolName === "write" && String(event.input?.path ?? "").endsWith("chenweb-pi-gateway-live-check.txt")) {
      const ok = await ctx.ui.confirm("Write the live-check file?", String(event.input.path));
      if (!ok) return { block: true, reason: "Denied in ChenWeb" };
    }
  });
}
`);
const harness = new PiHarness({
  spawn: piProcess({ bin: process.env.PI_HARNESS_BIN || "pi", cwd, env: piEnv(process.env, passList(process.env.PI_HARNESS_PASS_ENV)), args: ["-e", probe] }),
  cwd, sessionDir: sessionDir(agentDir(process.env), cwd), idleMs: 60_000,
});
const handler = createGatewayHandler({ secret: "live-check", harness });
const call = (method: string, path: string, body?: unknown) =>
  handler(new Request(`http://gateway.local${path}`, { method, headers: { authorization: "Bearer live-check" }, body: body === undefined ? undefined : JSON.stringify(body) }));
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
        const expected = event.toolName === "confirm" && event.description === target;
        console.log("   decision:", expected ? "allow" : "deny");
        void call("POST", `/v1/sessions/${sessionId}/permissions/${event.requestId}`, { behavior: expected ? "allow" : "deny" });
      }
    }
  }
  console.log("   answer:", JSON.stringify(lines.filter((e) => e.type === "text").map((e) => e.text).join("")));
  return lines;
}
const models = (await (await call("GET", "/v1/models")).json()).models.map((m: { value: string }) => m.value);
console.log(`models (${models.length}):`, models.join(", "));
console.log("turn 1");
// Like the chat page: the model comes from Pi's own list (its default is first).
const first = await turn({ model: models[0], message: "Remember the codeword PELICAN-42. Also tell me the first line of ChenWeb/go.mod. Keep the reply to one line." });
const sessionId = first.find((e) => e.type === "session")?.sessionId;
console.log("turn 2 (same session)");
await turn({ sessionId, message: "What was the codeword I gave you? Reply with the codeword only." });
rmSync(target, { force: true });
console.log("turn 3 (write asked about by the probe extension, approved)");
await turn({ sessionId, message: `Use the write tool to create ${target} containing the single word ok.` });
console.log("file written:", existsSync(target) && readFileSync(target, "utf8").trim() === "ok");
rmSync(target, { force: true });
const history = await (await call("GET", `/v1/sessions/${sessionId}/messages`)).json();
console.log("history messages from Pi's session file:", history.messages?.length, history.messages?.slice(0, 2).map((m: any) => m.blocks[0]?.text?.slice(0, 60)));
const listed = await (await call("GET", "/v1/sessions?limit=5")).json();
console.log("session listed:", JSON.stringify(listed.sessions.find((s: { sessionId: string }) => s.sessionId === sessionId)));
harness.close();
console.log("turn 4 (resumed in a new Pi process)");
await turn({ sessionId, message: "Repeat the codeword once more, codeword only." });
harness.close();
rmSync(probeDir, { recursive: true, force: true });
await Bun.sleep(1000);
process.exit(0);
