import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { capped, type Block, type HistoryMessage } from "./protocol.ts";

// Pi keeps each session as one JSONL file, <agent dir>/sessions/--<cwd>--/
// <timestamp>_<session id>.jsonl: a header line, then entries that form a
// tree through id/parentId. The active branch runs from the last entry (the
// leaf) back to the root, as Pi's SessionManager reads it. RPC mode cannot
// list sessions it is not running, so the gateway reads these files itself.

/** The session-file format version this gateway reads. */
export const SESSION_VERSION = 3;

export interface SessionHeader { type: "session"; id: string; cwd?: string; version?: number; parentSession?: string }

/** A file in a format version this gateway does not read; its header is still known. */
export class UnsupportedSessionError extends Error {
  constructor(readonly header: SessionHeader) { super(`Pi session format v${String(header.version ?? 1)} is not supported`); }
}

export interface Entry { type: string; id: string; parentId: string | null; timestamp?: string; [field: string]: any }
export interface SessionFile { path: string; header: SessionHeader; entries: Entry[]; modified: number }

/** Pi's session folder for a working directory (as `getDefaultSessionDir` computes it). */
export const sessionDir = (agentDir: string, cwd: string) => join(agentDir, "sessions", `--${cwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--`);

/** The session files in a folder, most recently modified first. */
export function sessionFiles(dir: string): { path: string; id: string; modified: number }[] {
  let names: string[];
  try { names = readdirSync(dir); } catch { return []; }
  const files: { path: string; id: string; modified: number }[] = [];
  for (const name of names) {
    const match = name.match(/_([0-9a-f-]{36})\.jsonl$/i);
    if (!match) continue;
    try { files.push({ path: join(dir, name), id: match[1]!, modified: statSync(join(dir, name)).mtimeMs }); } catch { /* removed meanwhile */ }
  }
  return files.sort((a, b) => b.modified - a.modified);
}

export const findSession = (dir: string, id: string) => sessionFiles(dir).find((file) => file.id === id)?.path;

/** Reads a session file. Undefined when it cannot be read; throws UnsupportedSessionError for another format version. */
export function readSession(path: string): SessionFile | undefined {
  let text: string;
  let modified: number;
  try { text = readFileSync(path, "utf8"); modified = statSync(path).mtimeMs; } catch { return undefined; }
  const lines: any[] = [];
  for (const line of text.split("\n")) {
    if (!line.trim()) continue;
    try { lines.push(JSON.parse(line)); } catch { /* an incomplete final line */ }
  }
  const header = lines.shift() as SessionHeader | undefined;
  if (header?.type !== "session" || typeof header.id !== "string") return undefined;
  if (header.version !== SESSION_VERSION) throw new UnsupportedSessionError(header);
  return { path, header, entries: lines.filter((e) => typeof e?.type === "string" && typeof e.id === "string"), modified };
}

/** The entries of the active branch, root first. */
export function activeBranch(entries: Entry[]): Entry[] {
  const byId = new Map(entries.map((e) => [e.id, e]));
  const branch: Entry[] = [];
  const seen = new Set<string>();
  for (let entry = entries.at(-1); entry && !seen.has(entry.id); entry = entry.parentId ? byId.get(entry.parentId) : undefined) {
    seen.add(entry.id);
    branch.push(entry);
  }
  return branch.reverse();
}

const isUser = (entry: Entry) => entry.type === "message" && entry.message?.role === "user";

/** Whether anyone has written in the session. */
export const conversed = (session: SessionFile) => session.entries.some(isUser);

export function textOf(content: unknown): string {
  if (typeof content === "string") return content;
  return Array.isArray(content) ? content.map((part) => (part?.type === "text" && typeof part.text === "string" ? part.text : "")).join("") : "";
}

export function summary(session: SessionFile): { title: string; firstPrompt: string; lastModified: number } {
  // A name set with /name, --name or an extension; otherwise Pi shows the first message.
  let name = "";
  for (const entry of session.entries) if (entry.type === "session_info") name = typeof entry.name === "string" ? entry.name : "";
  const first = activeBranch(session.entries).find(isUser) ?? session.entries.find(isUser);
  const firstPrompt = first ? textOf(first.message.content) : "";
  return { title: name || firstPrompt, firstPrompt, lastModified: session.modified };
}

/**
 * The active branch as ChenWeb's session history: what the user typed, the
 * assistant's text and tool calls, tool results, and the user's own `!`
 * commands. Thinking, extension-injected messages and summaries are left out.
 */
export function historyMessages(session: SessionFile): HistoryMessage[] {
  const messages: HistoryMessage[] = [];
  const add = (uuid: string, role: "user" | "assistant", blocks: Block[]) => { if (blocks.length) messages.push({ uuid, role, parentToolUseId: null, blocks }); };
  for (const entry of activeBranch(session.entries)) {
    if (entry.type !== "message") continue;
    const message = entry.message ?? {};
    switch (message.role) {
      case "user":
        add(entry.id, "user", typeof message.content === "string"
          ? (message.content ? [{ type: "text", text: message.content }] : [])
          : ((message.content ?? []) as { type: string; text?: string }[]).flatMap((part): Block[] =>
            part.type === "text" ? (part.text ? [{ type: "text", text: part.text }] : []) : part.type === "image" ? [{ type: "image" }] : []));
        break;
      case "assistant":
        add(entry.id, "assistant", ((message.content ?? []) as { type: string; text?: string; id?: string; name?: string; arguments?: unknown }[]).flatMap((part): Block[] => {
          if (part.type === "text") return part.text ? [{ type: "text", text: part.text }] : [];
          if (part.type === "toolCall") return [{ type: "tool_use", id: String(part.id), name: String(part.name), input: part.arguments ?? {} }];
          return [];
        }));
        break;
      case "toolResult":
        add(entry.id, "user", [{ type: "tool_result", toolUseId: String(message.toolCallId), isError: message.isError === true, text: capped(textOf(message.content)) }]);
        break;
      case "bashExecution":
        // A command the user ran with `!`, shown as Pi's bash tool.
        add(entry.id, "user", [
          { type: "tool_use", id: entry.id, name: "bash", input: { command: String(message.command ?? "") } },
          { type: "tool_result", toolUseId: entry.id, isError: typeof message.exitCode === "number" && message.exitCode !== 0, text: capped(String(message.output ?? "")) },
        ]);
        break;
    }
  }
  return messages;
}
