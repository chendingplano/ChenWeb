import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { capped, type Block, type HistoryMessage } from "./protocol.ts";

// dsh's ACP server neither replays history nor reports titles, so the gateway
// reads dsh's own session log: <sessions root>/<project dir>/<session id>/
// session.v<N>.jsonl, compressed as concatenated zstd frames (.zstd) by
// default. One JSON event per line; the first line is the header.

/** The session-log format version this gateway reads. */
export const LOG_VERSION = 4;

export interface LogHeader { id: string; cwd?: string; delegationDepth?: number; version: number }

/** A log in a format version this gateway does not read; its header is still known. */
export class UnsupportedLogError extends Error {
  constructor(readonly header: LogHeader) { super(`DeepSeek Harness session format v${String(header.version)} is not supported`); }
}

export interface SessionLog {
  header: LogHeader;
  events: { type: string; seq?: number; data?: any }[];
  modified: number;
}

const LOG_FILE = /^session(?:\.v(\d+))?\.jsonl(\.zstd)?$/;

/** The newest-version log file of a session, if dsh has written one. */
export function findLog(root: string, id: string): string | undefined {
  let projects: string[];
  try { projects = readdirSync(root); } catch { return undefined; }
  for (const project of projects) {
    let files: string[];
    try { files = readdirSync(join(root, project, id)); } catch { continue; }
    let best: { version: number; file: string } | undefined;
    for (const file of files) {
      const match = file.match(LOG_FILE);
      const version = match ? Number(match[1] ?? 0) : -1;
      if (match && (!best || version > best.version)) best = { version, file };
    }
    if (best) return join(root, project, id, best.file);
  }
  return undefined;
}

/** Reads a session's log. Undefined when there is none; throws UnsupportedLogError for another format version and other errors for an unreadable file. */
export function readLog(root: string, id: string): SessionLog | undefined {
  const path = findLog(root, id);
  if (!path) return undefined;
  let bytes: Uint8Array;
  try { bytes = readFileSync(path); } catch { return undefined; }
  // dsh appends whole frames; a torn final frame (after a crash) fails here
  // and is repaired by dsh the next time it resumes the session.
  const text = new TextDecoder().decode(path.endsWith(".zstd") ? Bun.zstdDecompressSync(bytes) : bytes);
  const lines = text.split("\n");
  const events: SessionLog["events"] = [];
  for (const line of lines) {
    if (!line.trim()) continue;
    try { events.push(JSON.parse(line)); } catch { /* an incomplete final line */ }
  }
  const header = events.shift() as unknown as LogHeader | undefined;
  if (!header || (header as { type?: string }).type !== "session") return undefined;
  if (header.version !== LOG_VERSION) throw new UnsupportedLogError(header);
  return { header, events, modified: statSync(path).mtimeMs };
}

/** Whether the log is a root session (not a subagent's) recorded for the workspace folder. */
export const inWorkspace = (header: LogHeader, cwd: string) => header.cwd === cwd && (header.delegationDepth ?? 0) === 0;

/** Messages the user typed, not the context dsh adds (instructions, runtime context, skill catalogs). */
const fromUser = (event: SessionLog["events"][number]) => event.type === "user/message" && event.data?.source?.kind === "user";

/** Whether anyone has written in the session. */
export const conversed = (log: SessionLog) => log.events.some(fromUser);

const textOf = (content: unknown) => (Array.isArray(content) ? content.map((part) => (part?.type === "text" && typeof part.text === "string" ? part.text : "")).join("") : "");

export function summary(log: SessionLog): { title: string; firstPrompt: string; lastModified: number } {
  let title = "";
  for (const event of log.events) if (event.type === "session/title" && typeof event.data?.title === "string") title = event.data.title;
  const first = log.events.find(fromUser);
  const firstPrompt = first ? textOf(first.data.content) : "";
  return { title: title || firstPrompt, firstPrompt, lastModified: log.modified };
}

/** A session's conversation as ChenWeb's session history. Reasoning is never shown. */
export function historyMessages(log: SessionLog): HistoryMessage[] {
  const messages: HistoryMessage[] = [];
  const add = (uuid: string, role: "user" | "assistant", blocks: Block[]) => { if (blocks.length) messages.push({ uuid, role, parentToolUseId: null, blocks }); };
  for (const event of log.events) {
    const seq = String(event.seq ?? messages.length);
    if (fromUser(event)) {
      add(seq, "user", ((event.data.content ?? []) as { type: string; text?: string }[]).flatMap((part): Block[] =>
        part.type === "text" ? (part.text ? [{ type: "text", text: part.text }] : []) : part.type === "image" ? [{ type: "image" }] : []));
    } else if (event.type === "assistant/message") {
      add(seq, "assistant", ((event.data?.message?.content ?? []) as { type: string; text?: string; id?: string; name?: string; arguments?: string }[]).flatMap((part): Block[] => {
        if (part.type === "text") return part.text ? [{ type: "text", text: part.text }] : [];
        if (part.type === "tool-call") return [{ type: "tool_use", id: String(part.id), name: String(part.name), input: parseArguments(part.arguments) }];
        return [];
      }));
    } else if (event.type === "tool/result") {
      const message = event.data?.message ?? {};
      add(`${seq}:result`, "user", [{ type: "tool_result", toolUseId: String(message.toolCallId), isError: message.isError === true, text: capped(textOf(message.content)) }]);
    }
  }
  return messages;
}

function parseArguments(value: unknown): unknown {
  if (typeof value !== "string") return value ?? {};
  try { return JSON.parse(value); } catch { return { arguments: value }; }
}
