import { capped, type Block, type HistoryMessage } from "./protocol.ts";

// Codex keeps a conversation as turns of thread items (user messages, agent
// messages, commands, file changes, MCP calls, ...). These functions show
// them in protocol-v1 terms. Reasoning is never relayed.

/** A Codex thread item, as the app-server sends it; only the fields read here are typed. */
export type CodexItem = { type: string; id: string; [field: string]: any };

/** The tool call an item stands for, or undefined for messages, reasoning and other non-tool items. */
export function toolCall(item: CodexItem): { id: string; name: string; input: unknown } | undefined {
  switch (item.type) {
    case "commandExecution":
      return { id: item.id, name: "Shell", input: { command: item.command, cwd: item.cwd } };
    case "fileChange": {
      const changes = (item.changes ?? []) as { path: string; kind?: { type?: string }; diff?: string }[];
      return { id: item.id, name: "Edit", input: { path: changes.map((c) => c.path).join(", "), changes: changes.map((c) => ({ path: c.path, kind: c.kind?.type, diff: capped(c.diff ?? "") })) } };
    }
    case "mcpToolCall":
      return { id: item.id, name: `mcp__${item.server}__${item.tool}`, input: item.arguments };
    case "dynamicToolCall":
      return { id: item.id, name: item.namespace ? `${item.namespace}.${item.tool}` : String(item.tool), input: item.arguments };
    case "webSearch":
      return { id: item.id, name: "WebSearch", input: { query: item.query } };
    case "collabAgentToolCall":
      return { id: item.id, name: "Agent", input: { tool: item.tool, prompt: item.prompt ?? undefined, model: item.model ?? undefined } };
    case "imageView":
      return { id: item.id, name: "ViewImage", input: { path: item.path } };
    default:
      return undefined;
  }
}

/** The result of a finished tool item; undefined while it runs or for non-tool items. */
export function toolResult(item: CodexItem): { isError: boolean; text: string } | undefined {
  if (!toolCall(item) || item.status === "inProgress") return undefined;
  switch (item.type) {
    case "commandExecution": {
      const failed = item.status !== "completed" || (item.exitCode ?? 0) !== 0;
      const text = item.status === "declined" ? "The user declined this command." : String(item.aggregatedOutput ?? "");
      return { isError: failed, text: capped(text) };
    }
    case "fileChange":
      return { isError: item.status !== "completed", text: item.status === "declined" ? "The user declined this change." : String(item.status ?? "") };
    case "mcpToolCall":
      if (item.error) return { isError: true, text: capped(String(item.error.message ?? "")) };
      return { isError: false, text: capped(contentText(item.result?.content)) };
    case "dynamicToolCall":
      return { isError: item.success === false, text: capped(contentText(item.contentItems)) };
    case "collabAgentToolCall":
      return { isError: item.status === "failed", text: capped(Object.values(item.agentsStates ?? {}).map((s: any) => s?.message ?? s?.status ?? "").filter(Boolean).join("\n")) };
    default:
      return { isError: false, text: "" };
  }
}

function contentText(content: unknown): string {
  if (!Array.isArray(content)) return "";
  return content.map((part) => (typeof part?.text === "string" ? part.text : "")).join("");
}

/** A Codex thread's turns as ChenWeb's session history. */
export function historyMessages(turns: { items?: CodexItem[] }[]): HistoryMessage[] {
  const messages: HistoryMessage[] = [];
  const add = (uuid: string, role: "user" | "assistant", blocks: Block[]) => { if (blocks.length) messages.push({ uuid, role, parentToolUseId: null, blocks }); };
  for (const turn of turns) {
    for (const item of turn.items ?? []) {
      if (item.type === "userMessage") {
        add(item.id, "user", ((item.content ?? []) as { type: string; text?: string }[]).flatMap((part): Block[] =>
          part.type === "text" ? (part.text ? [{ type: "text", text: part.text }] : []) : part.type === "image" || part.type === "localImage" ? [{ type: "image" }] : []));
      } else if (item.type === "agentMessage" || item.type === "plan") {
        add(item.id, "assistant", item.text ? [{ type: "text", text: item.text }] : []);
      } else {
        const call = toolCall(item);
        if (!call) continue;
        add(item.id, "assistant", [{ type: "tool_use", ...call }]);
        const result = toolResult(item);
        if (result) add(`${item.id}:result`, "user", [{ type: "tool_result", toolUseId: item.id, ...result }]);
      }
    }
  }
  return messages;
}
