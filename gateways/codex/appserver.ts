// The connection to Codex: `codex app-server`, the JSON-RPC interface the
// Codex IDE extension uses, over stdio (one JSON message per line, without a
// "jsonrpc" field). One app-server process serves every session.

export type RequestId = string | number;

/** What Codex sends without being asked: notifications and its own requests (approvals, questions). */
export interface RpcHandlers {
  notification(method: string, params: any): void;
  request(method: string, params: any, id: RequestId): Promise<unknown>;
  /** The app-server exited or its stream ended. */
  closed(): void;
}

export interface RpcConnection {
  request<T = any>(method: string, params: unknown): Promise<T>;
  close(): void;
}

/** Starts and initializes an app-server. Replaced by a scripted stand-in in tests. */
export type Connect = (handlers: RpcHandlers) => Promise<RpcConnection>;

export class RpcError extends Error {
  constructor(readonly code: number, message: string) { super(message); }
}

export const CLIENT_INFO = { name: "chenweb_harness_frontend", title: "ChenWeb", version: "0.1.0" };

export function appServer(options: { bin: string; cwd: string; env: Record<string, string | undefined> }): Connect {
  return async (handlers) => {
    const proc = Bun.spawn([options.bin, "app-server"], { cwd: options.cwd, env: options.env, stdin: "pipe", stdout: "pipe", stderr: "inherit" });
    const waiting = new Map<number, { resolve: (value: any) => void; reject: (error: Error) => void }>();
    let nextId = 0;
    let closed = false;
    const write = (message: unknown) => {
      if (closed) return;
      try { proc.stdin.write(`${JSON.stringify(message)}\n`); proc.stdin.flush(); } catch { /* reported by the exit handler */ }
    };
    const shutDown = () => {
      if (closed) return;
      closed = true;
      for (const pending of waiting.values()) pending.reject(new Error("Codex app-server exited"));
      waiting.clear();
      handlers.closed();
    };
    const dispatch = (message: { id?: RequestId; method?: string; params?: unknown; result?: unknown; error?: { code?: number; message?: string } }) => {
      if (message.method === undefined) {
        const pending = typeof message.id === "number" ? waiting.get(message.id) : undefined;
        if (!pending) return;
        waiting.delete(message.id as number);
        if (message.error) pending.reject(new RpcError(Number(message.error.code) || -32000, String(message.error.message ?? "request failed")));
        else pending.resolve(message.result);
      } else if (message.id !== undefined) {
        const id = message.id;
        handlers.request(message.method, message.params, id).then(
          (result) => write({ id, result }),
          (error) => write({ id, error: { code: error instanceof RpcError ? error.code : -32000, message: error instanceof Error ? error.message : "request failed" } }),
        );
      } else {
        handlers.notification(message.method, message.params);
      }
    };
    void (async () => {
      const decoder = new TextDecoder();
      let buffer = "";
      try {
        for await (const chunk of proc.stdout) {
          buffer += decoder.decode(chunk, { stream: true });
          let newline: number;
          while ((newline = buffer.indexOf("\n")) >= 0) {
            const line = buffer.slice(0, newline).trim();
            buffer = buffer.slice(newline + 1);
            if (!line) continue;
            try { dispatch(JSON.parse(line)); } catch { /* not a protocol message */ }
          }
        }
      } catch { /* reported below */ }
      shutDown();
    })();
    void proc.exited.then(shutDown);

    const connection: RpcConnection = {
      request(method, params) {
        if (closed) return Promise.reject(new Error("Codex app-server exited"));
        const id = ++nextId;
        return new Promise((resolve, reject) => { waiting.set(id, { resolve, reject }); write({ id, method, params }); });
      },
      close() { proc.kill(); shutDown(); },
    };
    // experimentalApi enables Codex's request_user_input questions.
    await connection.request("initialize", { clientInfo: CLIENT_INFO, capabilities: { experimentalApi: true, requestAttestation: false } });
    write({ method: "initialized" });
    return connection;
  };
}
