import { afterEach, describe, expect, test } from 'bun:test';
import {
	MemberRefusedError,
	buildPage,
	createGroup,
	groupLabel,
	hasPage,
	nextPosition,
	parseRoles,
	syncGroup,
	type DocGroup
} from './doc-groups-client';

describe('parseRoles', () => {
	test('trims, lower-cases and de-duplicates', () => {
		expect(parseRoles(' Admin, dev ,ADMIN,,')).toEqual(['admin', 'dev']);
		expect(parseRoles('')).toEqual([]);
	});
});

// A fetch stand-in that answers each request with the next scripted response and records the calls.
type Reply = { status: number; body?: unknown };
const realFetch = globalThis.fetch;
let calls: { url: string; method: string; body?: unknown }[] = [];
function script(...replies: Reply[]) {
	calls = [];
	globalThis.fetch = (async (url: string, init?: RequestInit) => {
		calls.push({
			url,
			method: init?.method ?? 'GET',
			body: init?.body ? JSON.parse(String(init.body)) : undefined
		});
		const r = replies.shift() ?? { status: 500 };
		return new Response(r.status === 204 ? null : JSON.stringify(r.body ?? {}), {
			status: r.status
		});
	}) as typeof fetch;
}
afterEach(() => {
	globalThis.fetch = realFetch;
});

const group = (over: Partial<DocGroup> = {}): DocGroup => ({
	group_id: 1,
	group_key: 'devdocs',
	label: { en: 'Dev Documents', 'zh-cn': '开发文档' },
	parent_menu_id: 'documents',
	sort_order: 10,
	access_role: ['admin'],
	enabled: false,
	members: [],
	documents: 0,
	uncategorized: 0,
	...over
});

describe('syncGroup', () => {
	test('rescans, then classifies only documents without a category', async () => {
		script({ status: 204 }, { status: 200, body: { classified: 3 } });
		expect(await syncGroup('devdocs')).toEqual({ classified: 3 });
		expect(calls.map((c) => `${c.method} ${c.url}`)).toEqual([
			'POST /api/v1/doc-groups/admin/groups/devdocs/rescan',
			'POST /api/v1/doc-groups/admin/groups/devdocs/classify'
		]);
		expect(calls[1].body).toEqual({ force: false });
	});

	test('keeps the rescan and reports a missing model instead of failing', async () => {
		script(
			{ status: 204 },
			{ status: 503, body: { error: 'classification model is not available' } }
		);
		expect(await syncGroup('devdocs')).toEqual({ classified: null });
	});

	test('a failed rescan is an error and nothing is classified', async () => {
		script({ status: 500, body: { error: 'request failed' } });
		await expect(syncGroup('devdocs')).rejects.toThrow('request failed');
		expect(calls).toHaveLength(1);
	});
});

describe('page', () => {
	test('a group has its page exactly while it is enabled', () => {
		expect(hasPage(group())).toBe(false);
		expect(hasPage(group({ enabled: true }))).toBe(true);
	});

	test('buildPage saves the group unchanged except enabled', async () => {
		script({ status: 200, body: {} });
		await buildPage(
			group({ members: [{ member_id: 1, member_type: 'dir', path: 'a', exists: true }] })
		);
		expect(calls[0].method).toBe('PUT');
		expect(calls[0].url).toBe('/api/v1/doc-groups/admin/groups/devdocs');
		expect(calls[0].body).toEqual({
			group_key: 'devdocs',
			label: { en: 'Dev Documents', 'zh-cn': '开发文档' },
			parent_menu_id: 'documents',
			sort_order: 10,
			access_role: ['admin'],
			enabled: true
		});
	});
});

describe('createGroup', () => {
	const input = {
		group_key: 'specs',
		label: { en: 'Specs', 'zh-cn': '规格' },
		sort_order: 30,
		access_role: ['admin'],
		members: [
			{ member_type: 'dir' as const, path: 'KnowledgeStore/doc-repo/specs' },
			{ member_type: 'file' as const, path: 'KnowledgeStore/README.md' }
		]
	};

	test('creates the group without a page under Documents, then adds each member', async () => {
		script({ status: 200, body: { group_key: 'specs' } }, { status: 200 }, { status: 200 });
		await createGroup(input);
		expect(calls[0].method).toBe('POST');
		expect(calls[0].body).toEqual({
			group_key: 'specs',
			label: { en: 'Specs', 'zh-cn': '规格' },
			sort_order: 30,
			access_role: ['admin'],
			parent_menu_id: 'documents',
			enabled: false
		});
		expect(calls.slice(1).map((c) => c.body)).toEqual([
			{ member_type: 'dir', path: 'KnowledgeStore/doc-repo/specs' },
			{ member_type: 'file', path: 'KnowledgeStore/README.md' }
		]);
	});

	test('a refused member deletes the new group and names the path', async () => {
		script(
			{ status: 200, body: { group_key: 'specs' } },
			{ status: 200 },
			{ status: 400, body: { error: 'bad path' } },
			{ status: 204 }
		);
		const err = await createGroup(input).catch((e) => e);
		expect(err).toBeInstanceOf(MemberRefusedError);
		expect(err.path).toBe('KnowledgeStore/README.md');
		expect(calls.at(-1)).toEqual({
			url: '/api/v1/doc-groups/admin/groups/specs',
			method: 'DELETE',
			body: undefined
		});
	});

	test('a refused group adds no members', async () => {
		script({ status: 409, body: { error: 'a group with this key exists' } });
		await expect(createGroup(input)).rejects.toThrow('a group with this key exists');
		expect(calls).toHaveLength(1);
	});
});

describe('nextPosition', () => {
	test('places a new group after the last one under the parent', () => {
		const groups = [
			group({ sort_order: 10 }),
			group({ sort_order: 20 }),
			group({ sort_order: 90, parent_menu_id: 'other' })
		];
		expect(nextPosition(groups)).toBe(30);
		expect(nextPosition([])).toBe(10);
	});
});

describe('groupLabel', () => {
	test('uses the locale, then English, then the key', () => {
		expect(groupLabel(group(), 'zh-cn')).toBe('开发文档');
		expect(groupLabel(group({ label: { en: 'Dev Documents' } }), 'zh-cn')).toBe('Dev Documents');
		expect(groupLabel(group({ label: {} }), 'en')).toBe('devdocs');
	});
});
