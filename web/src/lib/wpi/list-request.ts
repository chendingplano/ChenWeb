export class RequestTimeoutError extends Error {}

export function hasListSource(source: { action?: string; table?: unknown } | undefined): boolean {
 if (!source) return false;
 if (source.action === 'query') return true;
 return typeof source.table === 'string' && source.table.trim().length > 0;
}

type ListSource = { action?: string; table?: unknown; fields?: { column?: string; attribute?: string }[] };
type PreviewColumn = { field: string; label: string; width?: string };

export function listQueryAvailability(source: ListSource | undefined, pageId: string, preview = false): 'unconfigured' | 'unsaved' | 'fields' | 'ready' {
 if (!hasListSource(source)) return 'unconfigured';
 if (preview && source?.action === 'load') return source.fields?.length ? 'ready' : 'fields';
 return /^[a-z0-9][a-z0-9-]{0,127}$/.test(pageId) ? 'ready' : 'unsaved';
}

export function previewColumns(columns: PreviewColumn[], source: ListSource | undefined): PreviewColumn[] {
 if (columns.length) return columns;
 return (source?.fields ?? []).filter(field => typeof field.attribute === 'string' && field.attribute.length > 0).map(field => {
  const name = field.attribute!;
  const label = name.replace(/_/g, ' ');
  return { field: name, label: label.charAt(0).toUpperCase() + label.slice(1) };
 });
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
