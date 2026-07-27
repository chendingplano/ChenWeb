import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createDocument,
	getDocument,
	saveDocument,
	saveDocumentToNewVersion,
	listDocuments,
	listDocumentVersions,
	publishDocument,
	renderDocument,
	renderDraftDocument,
	CdmValidationError,
	CdmStaleVersionError,
	CdmFrozenError,
	CdmBlockConflictError,
	CdmApiError
} from './cdm-client.js';
import type { Document } from './types.js';

type FetchCall = { input: string | URL | Request; init?: RequestInit };

function installFetchMock(handler: (call: FetchCall) => Promise<Response>) {
	const originalFetch = globalThis.fetch;
	const calls: FetchCall[] = [];
	globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
		const call = { input, init };
		calls.push(call);
		return handler(call);
	}) as typeof fetch;
	return {
		calls,
		restore() {
			globalThis.fetch = originalFetch;
		}
	};
}

function minimalDoc(overrides: Partial<Document> = {}): Document {
	return {
		document_key: 'doc:test',
		title: 'Test',
		language: 'en',
		schema_version: '1.0',
		content_version: 1,
		edit_version: 1,
		metadata: {},
		blocks: [],
		...overrides
	};
}

test('createDocument posts to /documents with tenant_id and returns the created document', async () => {
	const mock = installFetchMock(async (call) => {
		assert.match(String(call.input), /^\/api\/v1\/cdm\/documents\?tenant_id=tenant-x$/);
		assert.equal(call.init?.method, 'POST');
		const sent = JSON.parse(String(call.init?.body)) as Document;
		assert.equal(sent.title, 'Smoke Test');
		return Response.json(minimalDoc({ document_key: 'doc:smoke-test', title: 'Smoke Test' }), {
			status: 201
		});
	});
	try {
		const doc = await createDocument(minimalDoc({ document_key: '', title: 'Smoke Test' }), {
			tenantId: 'tenant-x'
		});
		assert.equal(doc.document_key, 'doc:smoke-test');
	} finally {
		mock.restore();
	}
});

test('getDocument fetches by key', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ajaro-winkler');
		return Response.json(minimalDoc({ document_key: 'doc:jaro-winkler' }));
	});
	try {
		const doc = await getDocument('doc:jaro-winkler');
		assert.equal(doc.document_key, 'doc:jaro-winkler');
	} finally {
		mock.restore();
	}
});

test('saveDocument PUTs to the document_key from the body', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ajaro-winkler');
		assert.equal(call.init?.method, 'PUT');
		return Response.json(
			minimalDoc({ document_key: 'doc:jaro-winkler', content_version: 7, edit_version: 8 })
		);
	});
	try {
		const saved = await saveDocument(
			minimalDoc({ document_key: 'doc:jaro-winkler', content_version: 7, edit_version: 7 })
		);
		assert.equal(saved.content_version, 7);
		assert.equal(saved.edit_version, 8);
	} finally {
		mock.restore();
	}
});

test('saveDocumentToNewVersion POSTs to the versions endpoint', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ajaro-winkler/versions');
		assert.equal(call.init?.method, 'POST');
		return Response.json(
			minimalDoc({ document_key: 'doc:jaro-winkler', content_version: 8, edit_version: 9 })
		);
	});
	try {
		const saved = await saveDocumentToNewVersion(
			minimalDoc({ document_key: 'doc:jaro-winkler', content_version: 7, edit_version: 8 })
		);
		assert.equal(saved.content_version, 8);
		assert.equal(saved.edit_version, 9);
	} finally {
		mock.restore();
	}
});

test('renderDraftDocument renders the in-memory document without saving it', async () => {
	const draft = minimalDoc({
		document_key: 'doc:jaro-winkler',
		title: 'Unsaved title'
	});
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ajaro-winkler/render-preview');
		assert.equal(call.init?.method, 'POST');
		assert.deepEqual(JSON.parse(String(call.init?.body)), draft);
		return Response.json({
			status: true,
			content_version: 7,
			pages: ['<svg><text>Unsaved title</text></svg>']
		});
	});
	try {
		const rendered = await renderDraftDocument(draft);
		assert.equal(rendered.pages[0], '<svg><text>Unsaved title</text></svg>');
	} finally {
		mock.restore();
	}
});

test('saveDocument surfaces a stale version as CdmStaleVersionError with the current version', async () => {
	const mock = installFetchMock(async () =>
		Response.json(
			{
				status: false,
				error_msg: 'cdm: document "doc:x" changed since it was loaded',
				conflict: 'stale_version',
				edit_version: 9
			},
			{ status: 409 }
		)
	);
	try {
		await assert.rejects(
			() =>
				saveDocument(minimalDoc({ document_key: 'doc:x', content_version: 7, edit_version: 7 })),
			(err: unknown) => {
				assert.ok(err instanceof CdmStaleVersionError);
				assert.equal(err.currentVersion, 9);
				return true;
			}
		);
	} finally {
		mock.restore();
	}
});

test('saveDocument surfaces a published document as CdmFrozenError', async () => {
	const mock = installFetchMock(async () =>
		Response.json(
			{ status: false, error_msg: 'cdm: document "doc:x" is published', conflict: 'frozen' },
			{ status: 409 }
		)
	);
	try {
		await assert.rejects(
			() => saveDocument(minimalDoc({ document_key: 'doc:x' })),
			(err: unknown) => err instanceof CdmFrozenError
		);
	} finally {
		mock.restore();
	}
});

test('createDocument surfaces a block slug collision as CdmBlockConflictError', async () => {
	const mock = installFetchMock(async () =>
		Response.json(
			{ status: false, error_msg: 'cdm: block id "intro" already exists', conflict: 'block_slug' },
			{ status: 409 }
		)
	);
	try {
		await assert.rejects(
			() => createDocument(minimalDoc(), { tenantId: 'tenant-x' }),
			(err: unknown) => err instanceof CdmBlockConflictError
		);
	} finally {
		mock.restore();
	}
});

test('createDocument surfaces a validation failure with all violations', async () => {
	const mock = installFetchMock(async () =>
		Response.json(
			{
				status: false,
				error_msg: 'document failed validation',
				violations: ['duplicate block id "intro"', 'block "bad" has unsupported type "bogus"']
			},
			{ status: 400 }
		)
	);
	try {
		await assert.rejects(
			() => createDocument(minimalDoc(), { tenantId: 'tenant-x' }),
			(err: unknown) => {
				assert.ok(err instanceof CdmValidationError);
				assert.equal(err.violations.length, 2);
				return true;
			}
		);
	} finally {
		mock.restore();
	}
});

test('getDocument surfaces a 404 as a plain CdmApiError, not one of the conflict types', async () => {
	const mock = installFetchMock(async () =>
		Response.json(
			{ status: false, error_msg: 'cdm: document "doc:nope" not found' },
			{ status: 404 }
		)
	);
	try {
		await assert.rejects(
			() => getDocument('doc:nope'),
			(err: unknown) => {
				assert.ok(err instanceof CdmApiError);
				assert.ok(!(err instanceof CdmValidationError));
				assert.ok(!(err instanceof CdmStaleVersionError));
				assert.ok(!(err instanceof CdmFrozenError));
				assert.ok(!(err instanceof CdmBlockConflictError));
				assert.equal(err.status, 404);
				return true;
			}
		);
	} finally {
		mock.restore();
	}
});

test('listDocuments passes tenant_id, page, and page_size as query parameters', async () => {
	const mock = installFetchMock(async (call) => {
		const url = new URL(String(call.input), 'http://localhost');
		assert.equal(url.pathname, '/api/v1/cdm/documents');
		assert.equal(url.searchParams.get('tenant_id'), 'tenant-x');
		assert.equal(url.searchParams.get('page'), '2');
		assert.equal(url.searchParams.get('page_size'), '10');
		return Response.json({ status: true, results: [], page: 2, page_size: 10 });
	});
	try {
		const res = await listDocuments({ tenantId: 'tenant-x', page: 2, pageSize: 10 });
		assert.equal(res.page, 2);
	} finally {
		mock.restore();
	}
});

test('publishDocument posts to the publish endpoint', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ax/publish');
		assert.equal(call.init?.method, 'POST');
		return Response.json({ status: true, content_version: 1, page_count: 1 });
	});
	try {
		const res = await publishDocument('doc:x');
		assert.equal(res.page_count, 1);
	} finally {
		mock.restore();
	}
});

test('listDocumentVersions GETs the versions endpoint', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ax/versions');
		return Response.json({
			status: true,
			results: [
				{
					content_version: 3,
					parent_content_version: 2,
					create_time: '2026-07-27T10:00:00Z',
					update_time: '2026-07-27T10:05:00Z',
					size_bytes: 1234,
					current: true
				}
			]
		});
	});
	try {
		const res = await listDocumentVersions('doc:x');
		assert.equal(res.results[0].content_version, 3);
		assert.equal(res.results[0].size_bytes, 1234);
	} finally {
		mock.restore();
	}
});

test('renderDocument GETs the render endpoint and returns SVG pages', async () => {
	const mock = installFetchMock(async (call) => {
		assert.equal(String(call.input), '/api/v1/cdm/documents/doc%3Ax/render');
		return Response.json({ status: true, content_version: 1, pages: ['<svg>...</svg>'] });
	});
	try {
		const res = await renderDocument('doc:x');
		assert.equal(res.pages.length, 1);
	} finally {
		mock.restore();
	}
});
