import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildArtifactObjectPatch,
	buildObjectNodePatch,
	createObjectNode,
	fieldDirty,
	getAmbiguousObjectDetail,
	listAmbiguousObjects,
	neighborAmbiguousId,
	nextIndexAfterResolve,
	updateArtifactObject,
	updateObjectNode,
	type ArtifactObjectDetail,
	type ObjectNodeCandidate
} from './resolve-ambiguous-objects-client.js';

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

const baseArtifactObject: ArtifactObjectDetail = {
	id: 101,
	source_record_id: 9,
	artifact_type: 'provision',
	artifact_id: '9_prv_1',
	object_name: 'pressure regulator',
	object_name_en: '',
	object_name_zh: '',
	language: '',
	object_type: 'equipment',
	object_role: 'regulated_object',
	aliases: ['reg'],
	acronyms: [],
	description: '',
	evidence_quote: '',
	object_id: '',
	reconcile_status: 'ambiguous',
	reconcile_confidence: 0
};

const baseCandidate: ObjectNodeCandidate = {
	object_id: 'obj_a',
	canonical_name: 'Pressure Regulator',
	canonical_name_en: '',
	canonical_name_zh: '',
	primary_language: '',
	object_type: 'equipment',
	aliases: [],
	acronyms: [],
	description: '',
	score: 0.85,
	method: 'lexical_name',
	recommended: true
};

test('listAmbiguousObjects loads the ambiguous rows list', async () => {
	const mock = installFetchMock(async () =>
		Response.json({
			status: true,
			rows: [
				{
					id: 101,
					artifact_type: 'provision',
					artifact_id: '9_prv_1',
					object_name: 'pressure regulator',
					object_name_en: '',
					confidence: 0.85
				}
			]
		})
	);
	try {
		const response = await listAmbiguousObjects();
		assert.equal(mock.calls.length, 1);
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/ambiguous');
		assert.equal(response.rows.length, 1);
		assert.equal(response.rows[0].object_name, 'pressure regulator');
	} finally {
		mock.restore();
	}
});

test('getAmbiguousObjectDetail loads the artifact object and candidates for one id', async () => {
	const mock = installFetchMock(async () =>
		Response.json({ status: true, artifact_object: baseArtifactObject, candidates: [baseCandidate] })
	);
	try {
		const response = await getAmbiguousObjectDetail(101);
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/ambiguous/101');
		assert.equal(response.artifact_object.object_name, 'pressure regulator');
		assert.equal(response.candidates[0].recommended, true);
	} finally {
		mock.restore();
	}
});

test('updateArtifactObject PATCHes the artifact-objects endpoint with the given patch', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true }));
	try {
		await updateArtifactObject(101, { object_id: 'obj_a', reconcile_status: 'ambiguous_resolved' });
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/artifact-objects/101');
		assert.equal(mock.calls[0].init?.method, 'PATCH');
		assert.equal(
			mock.calls[0].init?.body,
			JSON.stringify({ object_id: 'obj_a', reconcile_status: 'ambiguous_resolved' })
		);
	} finally {
		mock.restore();
	}
});

test('updateObjectNode PATCHes the object-nodes endpoint keyed by object_id', async () => {
	const mock = installFetchMock(async () => Response.json({ status: true }));
	try {
		await updateObjectNode('obj_a', { canonical_name: 'Pressure Regulator' });
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/object-nodes/obj_a');
		assert.equal(mock.calls[0].init?.method, 'PATCH');
	} finally {
		mock.restore();
	}
});

test('createObjectNode POSTs the create-node endpoint and returns a created node', async () => {
	const mock = installFetchMock(async () =>
		Response.json({ status: true, created: true, node: { ...baseCandidate, object_id: 'obj_new' } })
	);
	try {
		const fields = {
			object_name: '舒张压',
			object_name_en: 'diastolic blood pressure',
			object_name_zh: '舒张压',
			language: 'zh',
			object_type: 'concept',
			aliases: [],
			acronyms: ['DBP'],
			description: '80~88 mmHg'
		};
		const result = await createObjectNode(101, fields);
		assert.equal(String(mock.calls[0].input), '/api/v1/kb/objects/ambiguous/101/create-node');
		assert.equal(mock.calls[0].init?.method, 'POST');
		assert.equal(mock.calls[0].init?.body, JSON.stringify(fields));
		assert.equal(result.created, true);
		if (result.created) assert.equal(result.node.object_id, 'obj_new');
	} finally {
		mock.restore();
	}
});

test('createObjectNode returns existing matches without created when the name already exists', async () => {
	const mock = installFetchMock(async () =>
		Response.json({ status: true, created: false, nodes: [baseCandidate] })
	);
	try {
		const result = await createObjectNode(101, {
			object_name: 'Pressure Regulator',
			object_name_en: '',
			object_name_zh: '',
			language: '',
			object_type: 'equipment',
			aliases: [],
			acronyms: [],
			description: ''
		});
		assert.equal(result.created, false);
		if (!result.created) assert.equal(result.nodes[0].object_id, 'obj_a');
	} finally {
		mock.restore();
	}
});

test('buildArtifactObjectPatch only includes changed editable fields', () => {
	const edited = { ...baseArtifactObject, object_name: 'Pressure Regulator', aliases: ['reg', 'regulator'] };
	const patch = buildArtifactObjectPatch(baseArtifactObject, edited);
	assert.deepEqual(patch, { object_name: 'Pressure Regulator', aliases: ['reg', 'regulator'] });
});

test('buildArtifactObjectPatch returns empty object when nothing changed', () => {
	const patch = buildArtifactObjectPatch(baseArtifactObject, { ...baseArtifactObject });
	assert.deepEqual(patch, {});
});

test('buildObjectNodePatch only includes changed editable fields', () => {
	const edited = { ...baseCandidate, description: 'Regulates line pressure' };
	const patch = buildObjectNodePatch(baseCandidate, edited);
	assert.deepEqual(patch, { description: 'Regulates line pressure' });
});

test('neighborAmbiguousId moves within the id list and returns null past the ends', () => {
	const ids = [101, 102, 103];
	assert.equal(neighborAmbiguousId(ids, 101, 1), 102);
	assert.equal(neighborAmbiguousId(ids, 103, 1), null);
	assert.equal(neighborAmbiguousId(ids, 101, -1), null);
	assert.equal(neighborAmbiguousId(ids, 102, -1), 101);
});

test('nextIndexAfterResolve selects the row that took the resolved row\'s place', () => {
	assert.equal(nextIndexAfterResolve([101, 102, 103], 101), 0); // 102 shifts into index 0
	assert.equal(nextIndexAfterResolve([101, 102, 103], 102), 1); // 103 shifts into index 1
});

test('nextIndexAfterResolve selects the previous row when the resolved row was last', () => {
	assert.equal(nextIndexAfterResolve([101, 102, 103], 103), 1); // 102 is now the last row, at index 1
});

test('nextIndexAfterResolve returns null when no rows remain', () => {
	assert.equal(nextIndexAfterResolve([101], 101), null);
});

test('fieldDirty is false for equal primitives and true for different primitives', () => {
	assert.equal(fieldDirty('a', 'a'), false);
	assert.equal(fieldDirty('a', 'b'), true);
	assert.equal(fieldDirty(0.85, 0.85), false);
	assert.equal(fieldDirty(0.85, 0.9), true);
});

test('fieldDirty compares arrays by content and order, not reference', () => {
	assert.equal(fieldDirty(['reg', 'regulator'], ['reg', 'regulator']), false);
	assert.equal(fieldDirty(['reg', 'regulator'], ['reg']), true);
	assert.equal(fieldDirty(['reg', 'regulator'], ['regulator', 'reg']), true);
});

test('fieldDirty treats a missing snapshot as dirty only when current differs from it', () => {
	assert.equal(fieldDirty('Pressure Regulator', undefined), true);
	assert.equal(fieldDirty(undefined, undefined), false);
});
