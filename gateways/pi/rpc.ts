// The connection to Pi: `pi --mode rpc`, Pi's headless protocol over stdio
// (one JSON object per line). Commands go in on stdin; responses, agent events
// and extension UI requests come out on stdout. One Pi process serves one
// session.

/** What Pi sends without being asked: agent events and its extensions' UI requests. */
export interface PiHandlers {
  event(event: any): void;
  uiRequest(request: any): void;
  /** The Pi process exited or its stream ended. */
  closed(): void;
}

export interface PiConnection {
  /** Sends a command and resolves with its response's `data`; rejects with Pi's error. */
  command<T = any>(command: Record<string, unknown>): Promise<T>;
  /** Writes a message that has no response, such as an `extension_ui_response`. */
  send(message: Record<string, unknown>): void;
  close(): void;
}

/** Starts Pi with extra arguments (`--session <file>`, `--no-session`). Replaced by a scripted stand-in in tests. */
export type Spawn = (args: string[], handlers: PiHandlers) => Promise<PiConnection>;

export class CommandError extends Error {}

export function piProcess(options: { bin: string; cwd: string; env: Record<string, string | undefined>; args?: string[] }): Spawn {
  return async (args, handlers) => {
    const proc = Bun.spawn([options.bin, "--mode", "rpc", ...(options.args ?? []), ...args], { cwd: options.cwd, env: options.env, stdin: "pipe", stdout: "pipe", stderr: "inherit" });
    const waiting = new Map<string, { resolve: (value: any) => void; reject: (error: Error) => void }>();
    let nextId = 0;
    let closed = false;
    const write = (message: Record<string, unknown>) => {
      if (closed) return;
      try { proc.stdin.write(`${JSON.stringify(message)}\n`); proc.stdin.flush(); } catch { /* reported by the exit handler */ }
    };
    const shutDown = () => {
      if (closed) return;
      closed = true;
      for (const pending of waiting.values()) pending.reject(new Error("Pi exited"));
      waiting.clear();
      handlers.closed();
    };
    const dispatch = (message: any) => {
      if (message?.type === "response") {
        const pending = typeof message.id === "string" ? waiting.get(message.id) : undefined;
        if (!pending) return;
        waiting.delete(message.id);
        if (message.success) pending.resolve(message.data);
        else pending.reject(new CommandError(String(message.error ?? `${String(message.command)} failed`)));
      } else if (message?.type === "extension_ui_request") {
        handlers.uiRequest(message);
      } else if (message && typeof message.type === "string") {
        handlers.event(message);
      }
    };
    void (async () => {
      const decoder = new TextDecoder();
      let buffer = "";
      try {
        for await (const chunk of proc.stdout) {
          buffer += decoder.decode(chunk, { stream: true });
          // Records end at LF only: U+2028 and U+2029 are valid inside JSON strings.
          let newline: number;
          while ((newline = buffer.indexOf("\n")) >= 0) {
            let line = buffer.slice(0, newline);
            buffer = buffer.slice(newline + 1);
            if (line.endsWith("\r")) line = line.slice(0, -1);
            if (!line.trim()) continue;
            try { dispatch(JSON.parse(line)); } catch { /* not a protocol message */ }
          }
        }
      } catch { /* reported below */ }
      shutDown();
    })();
    void proc.exited.then(shutDown);

    return {
      command(command) {
        if (closed) return Promise.reject(new Error("Pi exited"));
        const id = `gw-${++nextId}`;
        return new Promise((resolve, reject) => { waiting.set(id, { resolve, reject }); write({ ...command, id }); });
      },
      send: write,
      // Closing stdin ends RPC mode; Pi has already written each entry to the session file.
      close() {
        try { proc.stdin.end(); } catch { /* already gone */ }
        const kill = setTimeout(() => proc.kill(), 5_000);
        void proc.exited.then(() => clearTimeout(kill));
        shutDown();
      },
    };
  };
}
