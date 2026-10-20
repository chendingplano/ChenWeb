// The connection to DeepSeek Harness: `dsh --profile acp`, dsh's Agent Client
// Protocol server, over stdio (JSON-RPC 2.0, one message per line). One dsh
// process serves every session.

export type RequestId = string | number;

/** What dsh sends without being asked: session updates and its own requests (permission prompts). */
export interface RpcHandlers {
  notification(method: string, params: any): void;
  request(method: string, params: any, id: RequestId): Promise<unknown>;
  /** The dsh process exited or its stream ended. */
  closed(): void;
}

export interface RpcConnection {
  request<T = any>(method: string, params: unknown): Promise<T>;
  notify(method: string, params: unknown): void;
  close(): void;
}

/** Starts and initializes a dsh ACP server. Replaced by a scripted stand-in in tests. */
export type Connect = (handlers: RpcHandlers) => Promise<RpcConnection>;

export class RpcError extends Error {
  constructor(readonly code: number, message: string) { super(message); }
}

export const CLIENT_INFO = { name: "chenweb_harness_frontend", title: "ChenWeb", version: "0.1.0" };

export function acpServer(options: { bin: string; cwd: string; env: Record<string, string | undefined> }): Connect {
  return async (handlers) => {
    // dsh writes only protocol frames to stdout; its diagnostics go to stderr.
    const proc = Bun.spawn([options.bin, "--profile", "acp"], { cwd: options.cwd, env: options.env, stdin: "pipe", stdout: "pipe", stderr: "inherit" });
    const waiting = new Map<number, { resolve: (value: any) => void; reject: (error: Error) => void }>();
    let nextId = 0;
    let closed = false;
    const write = (message: Record<string, unknown>) => {
      if (closed) return;
      try { proc.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", ...message })}\n`); proc.stdin.flush(); } catch { /* reported by the exit handler */ }
    };
    const shutDown = () => {
      if (closed) return;
      closed = true;
      for (const pending of waiting.values()) pending.reject(new Error("DeepSeek Harness exited"));
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
      } else if (message.id !== undefined && message.id !== null) {
        const id = message.id;
        handlers.request(message.method, message.params, id).then(
          (result) => write({ id, result }),
          (error) => write({ id, error: { code: error instanceof RpcError ? error.code : -32603, message: error instanceof Error ? error.message : "request failed" } }),
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
        if (closed) return Promise.reject(new Error("DeepSeek Harness exited"));
        const id = ++nextId;
        return new Promise((resolve, reject) => { waiting.set(id, { resolve, reject }); write({ id, method, params }); });
      },
      notify(method, params) { write({ method, params }); },
      // Closing stdin is dsh's orderly shutdown: it drains and persists its sessions.
      close() { try { proc.stdin.end(); } catch { proc.kill(); } shutDown(); },
    };
    // The gateway offers dsh no client filesystem or terminal: dsh uses its own tools.
    await connection.request("initialize", { protocolVersion: 1, clientCapabilities: {}, clientInfo: CLIENT_INFO });
    return connection;
  };
}
