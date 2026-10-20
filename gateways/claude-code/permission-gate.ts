// Holds ask-mode tool calls until ChenWeb relays the user's decision. Same
// behavior as the Pi gateway's gate.
export interface PermissionRequest { requestId: string; tool: string; input: unknown }

export class PermissionGate {
  readonly #mode: "ask" | "auto";
  readonly #notify?: (request: PermissionRequest) => void;
  readonly #requests = new Map<string, { request: PermissionRequest; resolve: (allowed: boolean) => void }>();
  constructor(mode: "ask" | "auto", notify?: (request: PermissionRequest) => void) { this.#mode = mode; this.#notify = notify; }
  authorize(requestId: string, tool: string, input: unknown): Promise<boolean> {
    if (this.#mode === "auto") return Promise.resolve(true);
    if (this.#requests.has(requestId)) return Promise.resolve(false);
    const request = { requestId, tool, input };
    return new Promise((resolve) => { this.#requests.set(requestId, { request, resolve }); this.#notify?.(request); });
  }
  resolve(requestId: string, allowed: boolean): boolean { const p = this.#requests.get(requestId); if (!p) return false; this.#requests.delete(requestId); p.resolve(allowed); return true; }
  cancelAll(): void { for (const { resolve } of this.#requests.values()) resolve(false); this.#requests.clear(); }
}
