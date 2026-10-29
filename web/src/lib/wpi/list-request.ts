export class RequestTimeoutError extends Error {}

export function hasListSource(source: { action?: string; table?: unknown } | undefined): boolean {
 if (!source) return false;
 if (source.action === 'query') return true;
 return typeof source.table === 'string' && source.table.trim().length > 0;
}

export function listQueryAvailability(source: { action?: string; table?: unknown } | undefined, pageId: string): 'unconfigured' | 'unsaved' | 'ready' {
 if (!hasListSource(source)) return 'unconfigured';
 return /^[a-z0-9][a-z0-9-]{0,127}$/.test(pageId) ? 'ready' : 'unsaved';
}

export async function requestWithTimeout<T>(request: (signal: AbortSignal) => Promise<T>, timeoutMs = 20_000, parent?: AbortSignal): Promise<T> {
 const controller = new AbortController();
 const cancel = () => controller.abort();
 if (parent?.aborted) controller.abort();
 else parent?.addEventListener('abort', cancel, { once: true });
 let timer: ReturnType<typeof setTimeout> | undefined;
 try {
  return await Promise.race([
   request(controller.signal),
   new Promise<never>((_, reject) => { timer = setTimeout(() => { reject(new RequestTimeoutError()); controller.abort(); }, timeoutMs); })
  ]);
 } finally {
  clearTimeout(timer);
  parent?.removeEventListener('abort', cancel);
 }
}
