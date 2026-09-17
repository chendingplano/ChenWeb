export type DrawingMetadata = { name: string; description?: string; prompt: string; keywords?: string; notes?: string; model?: string };
export type PendingProductDrawing = DrawingMetadata & { token: string; image_url: string; expires_at: string };
export type ProductDrawing = DrawingMetadata & { id: number; filename: string; model_name: string; image_url: string; created_at: string; updated_at: string };
export type ProductDrawingPage = { drawings: ProductDrawing[]; total: number; page: number; page_size: number };
export type KeptProductDrawing = { status: boolean; filename: string; path: string; id?: number };

async function jsonRequest<T>(url: string, init: RequestInit, fetchFn: typeof fetch): Promise<T> {
	const res = await fetchFn(url, { ...init, credentials: 'same-origin' });
	let body: T & { error?: string };
	try { body = (await res.json()) as T & { error?: string }; } catch { throw new Error(`request failed: ${res.status}`); }
	if (!res.ok) throw new Error(body.error || `request failed: ${res.status}`);
	return body;
}
export function generateProductDrawing(metadata: DrawingMetadata | typeof fetch = { name: 'Ventilator exploded view', prompt: '' }, fetchFn: typeof fetch = fetch) { if (typeof metadata === 'function') { fetchFn = metadata; metadata = { name: 'Ventilator exploded view', prompt: '' }; } return jsonRequest<PendingProductDrawing>('/api/v1/product-drawings/generate', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(metadata)}, fetchFn); }
export function composeDrawingPrompt(productName: string, components: string[], fetchFn: typeof fetch = fetch) { return jsonRequest<{prompt:string}>('/api/v1/product-drawings/compose-prompt', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({product_name:productName,components})}, fetchFn); }
export function listProductDrawings(page=1, pageSize=12, fetchFn: typeof fetch = fetch) { return jsonRequest<ProductDrawingPage>(`/api/v1/product-drawings?page=${page}&page_size=${pageSize}`, {method:'GET'}, fetchFn); }
export function updateProductDrawing(id:number, metadata:DrawingMetadata, fetchFn:typeof fetch=fetch) { return jsonRequest<{status:boolean}>(`/api/v1/product-drawings/${id}`, {method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(metadata)}, fetchFn); }
export function deleteProductDrawing(id:number, fetchFn:typeof fetch=fetch) { return jsonRequest<{status:boolean}>(`/api/v1/product-drawings/${id}`, {method:'DELETE'}, fetchFn); }
export function productDrawingContentUrl(id:number) { return `/api/v1/product-drawings/${id}/content`; }
export function pendingProductDrawingContentUrl(token:string) { return `/api/v1/product-drawings/pending/${encodeURIComponent(token)}/content`; }
export function keepProductDrawing(token:string, fetchFn:typeof fetch=fetch) { return jsonRequest<KeptProductDrawing>(`/api/v1/product-drawings/pending/${encodeURIComponent(token)}/keep`, {method:'POST'}, fetchFn); }
export async function ignoreProductDrawing(token:string, fetchFn:typeof fetch=fetch) { await jsonRequest<{status:boolean}>(`/api/v1/product-drawings/pending/${encodeURIComponent(token)}`, {method:'DELETE'}, fetchFn); }
