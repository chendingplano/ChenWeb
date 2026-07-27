// Client for the CDM HTTP API (server/api/cdmhandler), registered at
// /api/v1/cdm/* behind the same auth middleware as the rest of /api/v1.
//
// Follows the fetch/error-handling shape already used by the home3 API
// clients (e.g. llm-accounts-client.ts): relative paths, same-origin
// credentials, and a JSON error body carrying error_msg. This module adds
// typed errors on top of that, because the CDM API's 409s are three
// different situations a caller must react to differently — stale version,
// frozen document, block slug conflict — and a single generic Error would
// force every caller to string-match error_msg to tell them apart.

import type { Document } from './types.js';

export class CdmApiError extends Error {
	readonly status: number;
	constructor(message: string, status: number) {
		super(message);
		this.name = 'CdmApiError';
		this.status = status;
	}
}

/** 400: model.Validate rejected the document. One entry per violation. */
export class CdmValidationError extends CdmApiError {
	readonly violations: string[];
	constructor(message: string, violations: string[]) {
		super(message, 400);
		this.name = 'CdmValidationError';
		this.violations = violations;
	}
}

/**
 * 409: the save's content_version no longer matches what is stored (DR6).
 * currentVersion is what the server actually has, so a caller can reload
 * and show the author what changed rather than just failing.
 */
export class CdmStaleVersionError extends CdmApiError {
	readonly currentVersion: number;
	constructor(message: string, currentVersion: number) {
		super(message, 409);
		this.name = 'CdmStaleVersionError';
		this.currentVersion = currentVersion;
	}
}

/** 409: the document is published and therefore read-only (D8). */
export class CdmFrozenError extends CdmApiError {
	constructor(message: string) {
		super(message, 409);
		this.name = 'CdmFrozenError';
	}
}

/** 409: a block id collides with one already in the document. */
export class CdmBlockConflictError extends CdmApiError {
	constructor(message: string) {
		super(message, 409);
		this.name = 'CdmBlockConflictError';
	}
}

// Mirrors the `conflict` discriminator values in
// server/api/cdmhandler/handler.go (conflictStaleVersion / conflictFrozen /
// conflictBlockSlug).
type ErrorBody = {
	status: false;
	error_msg: string;
	violations?: string[];
	conflict?: 'stale_version' | 'frozen' | 'block_slug';
	content_version?: number;
	edit_version?: number;
};

function isErrorBody(v: unknown): v is ErrorBody {
	return typeof v === 'object' && v !== null && 'error_msg' in v;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}

	if (!res.ok) {
		const body = isErrorBody(parsed) ? parsed : null;
		const message = body?.error_msg ?? `HTTP ${res.status}`;
		switch (body?.conflict) {
			case 'stale_version':
				throw new CdmStaleVersionError(message, body.edit_version ?? body.content_version ?? 0);
			case 'frozen':
				throw new CdmFrozenError(message);
			case 'block_slug':
				throw new CdmBlockConflictError(message);
		}
		if (res.status === 400 && body?.violations) {
			throw new CdmValidationError(message, body.violations);
		}
		throw new CdmApiError(message, res.status);
	}
	return parsed as T;
}

const jsonHeaders = { 'Content-Type': 'application/json' };

export interface CreateDocumentOptions {
	tenantId: string;
	ksStoreId?: number;
}

/**
 * Where a not-yet-created document (DocumentEditor's key-less "New Document"
 * case) will be created, plus the display name its create-confirmation
 * dialog names so the author knows which knowledge store they are committing
 * to before the first save actually calls createDocument.
 */
export interface CreateTarget extends CreateDocumentOptions {
	ksName: string;
}

/**
 * Creates a document. doc.document_key is ignored — keys are
 * server-allocated from the title (design D5) — and the returned document
 * carries the real one.
 */
export function createDocument(doc: Document, opts: CreateDocumentOptions): Promise<Document> {
	const params = new URLSearchParams({ tenant_id: opts.tenantId });
	if (opts.ksStoreId !== undefined) {
		params.set('ks_store_id', String(opts.ksStoreId));
	}
	return request<Document>(`/api/v1/cdm/documents?${params}`, {
		method: 'POST',
		headers: jsonHeaders,
		body: JSON.stringify(doc)
	});
}

export function getDocument(key: string): Promise<Document> {
	return request<Document>(`/api/v1/cdm/documents/${encodeURIComponent(key)}`);
}

/**
 * Saves doc. doc.content_version is the caller's expectation for optimistic
 * concurrency (DR6): it must be the version this document was loaded or last
 * saved at, not the version the caller wants to move to. On success the
 * returned document carries the new content_version.
 */
export function saveDocument(doc: Document): Promise<Document> {
	return request<Document>(`/api/v1/cdm/documents/${encodeURIComponent(doc.document_key)}`, {
		method: 'PUT',
		headers: jsonHeaders,
		body: JSON.stringify(doc)
	});
}

export function saveDocumentToNewVersion(doc: Document): Promise<Document> {
	return request<Document>(
		`/api/v1/cdm/documents/${encodeURIComponent(doc.document_key)}/versions`,
		{
			method: 'POST',
			headers: jsonHeaders,
			body: JSON.stringify(doc)
		}
	);
}

export interface DocumentSummary {
	document_key: string;
	title: string;
	content_version: number;
	doc_type?: string;
	rendering_type?: string;
	published: boolean;
	create_time: string;
	update_time: string;
}

export interface ListDocumentsResponse {
	status: true;
	results: DocumentSummary[];
	page: number;
	page_size: number;
}

export interface ListDocumentsOptions {
	/**
	 * A filter, not an isolation boundary: the CDM API takes tenant_id from
	 * the caller like the rest of this app's API does (see the note on
	 * ListDocuments in server/api/cdmhandler/documents.go). Omitting it
	 * returns every tenant's documents.
	 */
	tenantId?: string;
	page?: number;
	pageSize?: number;
}

export function listDocuments(opts: ListDocumentsOptions = {}): Promise<ListDocumentsResponse> {
	const params = new URLSearchParams();
	if (opts.tenantId) params.set('tenant_id', opts.tenantId);
	if (opts.page) params.set('page', String(opts.page));
	if (opts.pageSize) params.set('page_size', String(opts.pageSize));
	const qs = params.toString();
	return request<ListDocumentsResponse>(`/api/v1/cdm/documents${qs ? `?${qs}` : ''}`);
}

export interface PublishResponse {
	status: true;
	content_version: number;
	page_count: number;
}

/** Publishing freezes the document (D8): it becomes read-only afterwards. */
export function publishDocument(key: string): Promise<PublishResponse> {
	return request<PublishResponse>(`/api/v1/cdm/documents/${encodeURIComponent(key)}/publish`, {
		method: 'POST'
	});
}

export interface RenderResponse {
	status: true;
	content_version: number;
	/** One SVG document per page, in page order. */
	pages: string[];
}

/**
 * Renders the document's current content_version to SVG pages, the same
 * artifact publishing produces, without publishing it (design D4/D9). Safe
 * to call on a draft: it does not freeze the document. Cached server-side by
 * content_version, so call this on an explicit preview action, not on every
 * keystroke.
 */
export function renderDocument(key: string): Promise<RenderResponse> {
	return request<RenderResponse>(`/api/v1/cdm/documents/${encodeURIComponent(key)}/render`);
}

/** Renders the current in-memory draft without saving it. */
export function renderDraftDocument(doc: Document, signal?: AbortSignal): Promise<RenderResponse> {
	return request<RenderResponse>(
		`/api/v1/cdm/documents/${encodeURIComponent(doc.document_key)}/render-preview`,
		{
			method: 'POST',
			headers: jsonHeaders,
			body: JSON.stringify(doc),
			signal
		}
	);
}

export interface DocumentVersionNode {
	content_version: number;
	parent_content_version?: number;
	create_time: string;
	update_time: string;
	size_bytes: number;
	current: boolean;
}

export interface ListDocumentVersionsResponse {
	status: true;
	results: DocumentVersionNode[];
}

export function listDocumentVersions(key: string): Promise<ListDocumentVersionsResponse> {
	return request<ListDocumentVersionsResponse>(
		`/api/v1/cdm/documents/${encodeURIComponent(key)}/versions`
	);
}
