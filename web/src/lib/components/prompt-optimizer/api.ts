// Thin typed wrapper over /api/v1/prompt-optimizer/*.
//
// Every call includes credentials:'same-origin' so the session cookie reaches
// the Echo auth middleware. The backend response envelope is either
//   { status:true, ... }
// or
//   { status:false, error_msg:"... (CWB_POH_xxx)" }.
// Callers should check `res.ok && body.status` and surface `error_msg` to
// the user.

export type Provider = 'openai' | 'openai_compatible' | 'anthropic' | 'gemini';
export type OptimizeKind = 'optimize' | 'iterate' | 'test' | 'analyze';
export type TemplateKind = OptimizeKind | 'user_optimize' | 'image_optimize';

export type Model = {
	id: string;
	user_id: string;
	name: string;
	provider: Provider;
	base_url?: string;
	model: string;
	default_params: Record<string, unknown>;
	enabled: boolean;
	has_api_key: boolean;
	created_at: string;
	updated_at: string;
};

export type CreateModelRequest = {
	name: string;
	provider: Provider;
	base_url?: string;
	model: string;
	api_key: string;
	default_params?: Record<string, unknown>;
	enabled?: boolean;
};

export type UpdateModelRequest = Omit<CreateModelRequest, 'api_key'> & {
	// empty api_key keeps the existing ciphertext
	api_key: string;
};

export type Template = {
	id: string;
	user_id?: string;
	kind: TemplateKind;
	name: string;
	description?: string;
	content: string;
	language: string;
	variables: string[];
	built_in: boolean;
	created_at: string;
	updated_at: string;
};

export type CreateTemplateRequest = {
	kind: TemplateKind;
	name: string;
	description?: string;
	content: string;
	language?: string;
	variables?: string[];
};

export type History = {
	id: string;
	user_id: string;
	kind: OptimizeKind;
	original_prompt?: string;
	optimized_prompt?: string;
	test_input?: string;
	test_output?: string;
	template_id?: string;
	model_id?: string;
	params: Record<string, unknown>;
	created_at: string;
};

export type Favorite = {
	id: string;
	user_id: string;
	ref_kind: 'history' | 'template';
	ref_id: string;
	note?: string;
	created_at: string;
};

export type Variable = {
	id: string;
	user_id: string;
	name: string;
	value: string;
	created_at: string;
	updated_at: string;
};

export type OptimizeRequest = {
	kind: OptimizeKind;
	model_id: string;
	template_id?: string;
	original_prompt?: string;
	optimized_prompt?: string;
	feedback?: string;
	test_input?: string;
	variables?: Record<string, unknown>;
	stream?: boolean;
};

export type OptimizeResponse = {
	status: true;
	id: string;
	output: string;
	kind: OptimizeKind;
	model_id: string;
	created_at: string;
};

const BASE = '/api/v1/prompt-optimizer';

// -----------------------------------------------------------------------
// Generic helpers
// -----------------------------------------------------------------------

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
	const init: RequestInit = {
		method,
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' }
	};
	if (body !== undefined) init.body = JSON.stringify(body);
	const res = await fetch(BASE + path, init);
	const text = await res.text();
	let parsed: unknown = null;
	try {
		parsed = text ? JSON.parse(text) : null;
	} catch {
		throw new Error(`invalid JSON from ${path}: ${text.slice(0, 120)}`);
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && 'error_msg' in parsed
				? String((parsed as { error_msg?: unknown }).error_msg ?? '')
				: `HTTP ${res.status}`;
		throw new Error(msg || `HTTP ${res.status}`);
	}
	if (parsed && typeof parsed === 'object' && 'status' in parsed) {
		const envelope = parsed as { status: boolean; error_msg?: string };
		if (envelope.status === false) {
			throw new Error(envelope.error_msg || 'request failed');
		}
	}
	return parsed as T;
}

// -----------------------------------------------------------------------
// Models
// -----------------------------------------------------------------------

export async function listModels(): Promise<Model[]> {
	const body = await request<{ status: true; models: Model[] }>('GET', '/models');
	return body.models ?? [];
}

export async function createModel(req: CreateModelRequest): Promise<Model> {
	const body = await request<{ status: true; model: Model }>('POST', '/models', req);
	return body.model;
}

export async function updateModel(id: string, req: UpdateModelRequest): Promise<Model> {
	const body = await request<{ status: true; model: Model }>('PUT', `/models/${id}`, req);
	return body.model;
}

export async function deleteModel(id: string): Promise<void> {
	await request<{ status: true }>('DELETE', `/models/${id}`);
}

// -----------------------------------------------------------------------
// Templates
// -----------------------------------------------------------------------

export async function listTemplates(kind?: TemplateKind): Promise<Template[]> {
	const q = kind ? `?kind=${encodeURIComponent(kind)}` : '';
	const body = await request<{ status: true; templates: Template[] }>('GET', `/templates${q}`);
	return body.templates ?? [];
}

export async function getTemplate(id: string): Promise<Template> {
	const body = await request<{ status: true; template: Template }>('GET', `/templates/${id}`);
	return body.template;
}

export async function createTemplate(req: CreateTemplateRequest): Promise<Template> {
	const body = await request<{ status: true; template: Template }>('POST', '/templates', req);
	return body.template;
}

export async function updateTemplate(id: string, req: CreateTemplateRequest): Promise<Template> {
	const body = await request<{ status: true; template: Template }>('PUT', `/templates/${id}`, req);
	return body.template;
}

export async function deleteTemplate(id: string): Promise<void> {
	await request<{ status: true }>('DELETE', `/templates/${id}`);
}

// -----------------------------------------------------------------------
// History
// -----------------------------------------------------------------------

export async function listHistory(opts?: {
	kind?: OptimizeKind;
	page?: number;
	page_size?: number;
}): Promise<{ history: History[]; total: number; page: number; page_size: number }> {
	const params = new URLSearchParams();
	if (opts?.kind) params.set('kind', opts.kind);
	if (opts?.page !== undefined) params.set('page', String(opts.page));
	if (opts?.page_size !== undefined) params.set('page_size', String(opts.page_size));
	const q = params.toString() ? `?${params.toString()}` : '';
	const body = await request<{
		status: true;
		history: History[];
		total: number;
		page: number;
		page_size: number;
	}>('GET', `/history${q}`);
	return {
		history: body.history ?? [],
		total: body.total ?? 0,
		page: body.page ?? 0,
		page_size: body.page_size ?? 25
	};
}

export async function deleteHistory(id: string): Promise<void> {
	await request<{ status: true }>('DELETE', `/history/${id}`);
}

// -----------------------------------------------------------------------
// Favorites
// -----------------------------------------------------------------------

export async function listFavorites(): Promise<Favorite[]> {
	const body = await request<{ status: true; favorites: Favorite[] }>('GET', '/favorites');
	return body.favorites ?? [];
}

export async function createFavorite(req: {
	ref_kind: 'history' | 'template';
	ref_id: string;
	note?: string;
}): Promise<string> {
	const body = await request<{ status: true; id: string }>('POST', '/favorites', req);
	return body.id;
}

export async function deleteFavorite(id: string): Promise<void> {
	await request<{ status: true }>('DELETE', `/favorites/${id}`);
}

// -----------------------------------------------------------------------
// Variables
// -----------------------------------------------------------------------

export async function listVariables(): Promise<Variable[]> {
	const body = await request<{ status: true; variables: Variable[] }>('GET', '/variables');
	return body.variables ?? [];
}

export async function upsertVariable(req: { name: string; value: string }): Promise<string> {
	const body = await request<{ status: true; id: string }>('POST', '/variables', req);
	return body.id;
}

export async function deleteVariable(id: string): Promise<void> {
	await request<{ status: true }>('DELETE', `/variables/${id}`);
}

// -----------------------------------------------------------------------
// Optimize — non-streaming
// -----------------------------------------------------------------------

export async function optimize(req: OptimizeRequest): Promise<OptimizeResponse> {
	return request<OptimizeResponse>('POST', '/optimize', { ...req, stream: false });
}

// -----------------------------------------------------------------------
// Optimize — streaming (SSE)
//
// The backend emits lines like:
//   data: {"delta":"...chunk..."}
//   data: {"delta":"...chunk..."}
//   data: {"done":true,"id":"...","output":"...full text..."}
//   data: [DONE]
// optimizeStream resolves once [DONE] is seen, passing the assembled output
// and (if available) the history row id so the caller can favorite the run.
// -----------------------------------------------------------------------

export type StreamCallbacks = {
	onDelta?: (delta: string, full: string) => void;
	onError?: (err: string) => void;
	signal?: AbortSignal;
};

export async function optimizeStream(
	req: OptimizeRequest,
	cb: StreamCallbacks = {}
): Promise<{ id: string; output: string }> {
	const res = await fetch(BASE + '/optimize', {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
		body: JSON.stringify({ ...req, stream: true }),
		signal: cb.signal
	});
	if (!res.ok || !res.body) {
		const text = await res.text().catch(() => '');
		let msg = `HTTP ${res.status}`;
		try {
			const p = JSON.parse(text);
			if (p?.error_msg) msg = p.error_msg;
		} catch {
			if (text) msg = text.slice(0, 200);
		}
		throw new Error(msg);
	}

	const reader = res.body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';
	let full = '';
	let id = '';

	while (true) {
		const { value, done } = await reader.read();
		if (done) break;
		buffer += decoder.decode(value, { stream: true });

		let idx: number;
		while ((idx = buffer.indexOf('\n\n')) !== -1) {
			const frame = buffer.slice(0, idx);
			buffer = buffer.slice(idx + 2);

			const dataLines = frame
				.split('\n')
				.filter((l) => l.startsWith('data:'))
				.map((l) => l.slice(5).trimStart());
			if (dataLines.length === 0) continue;
			const payload = dataLines.join('\n');

			if (payload === '[DONE]') {
				return { id, output: full };
			}

			try {
				const evt = JSON.parse(payload) as {
					delta?: string;
					done?: boolean;
					id?: string;
					output?: string;
					error?: string;
				};
				if (evt.error) {
					cb.onError?.(evt.error);
					continue;
				}
				if (typeof evt.delta === 'string') {
					full += evt.delta;
					cb.onDelta?.(evt.delta, full);
				}
				if (evt.done) {
					id = evt.id ?? '';
					if (typeof evt.output === 'string') full = evt.output;
				}
			} catch {
				// ignore malformed frame; keep streaming
			}
		}
	}
	return { id, output: full };
}
