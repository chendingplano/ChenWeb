import test from 'node:test';
import assert from 'node:assert/strict';
import type { JsonObject } from '$lib/wpi/types';
import type { Node } from '@xyflow/svelte';
import { nodeData, nodeJson, pageJson, pageNodes } from './wpi-json';

function element(id: string, cls: string, x: number, y: number, attrs?: Record<string, unknown>): Node {
	return { id, type: 'wpi-element', position: { x, y }, data: { label: `name of ${id}`, element: cls, ...(attrs ? { attrs } : {}) } };
}

test('an element node is its class and the attributes set, not the node name', () => {
	const node = element('b', 'button', 0, 0, { label: 'Save', on_click: { action: 'save-page' } });
	assert.deepEqual(nodeJson(node), { element: 'button', label: 'Save', on_click: { action: 'save-page' } });
	assert.deepEqual(Object.keys(nodeJson(node))[0], 'element');
	assert.deepEqual(nodeJson(element('i', 'input', 0, 0)), { element: 'input' });
});

test('other nodes show their data', () => {
	const page: Node = { id: 'p', type: 'wpi-page', position: { x: 0, y: 0 }, data: { label: 'Page' } };
	assert.deepEqual(nodeJson(page), { label: 'Page' });
});

test('an edited element instance goes back into the node, which keeps its name', () => {
	const node = element('b', 'button', 0, 0, { label: 'Save' });
	const data = nodeData(node, { element: 'link', label: 'Send', width: 200 });
	assert.deepEqual(data, { label: 'name of b', element: 'link', attrs: { label: 'Send', width: 200 } });
	assert.deepEqual(nodeJson({ ...node, data }), { element: 'link', label: 'Send', width: 200 });
});

test('other nodes take the edited JSON as their data', () => {
	const page: Node = { id: 'p', type: 'wpi-page', position: { x: 0, y: 0 }, data: { label: 'Page' } };
	assert.deepEqual(nodeData(page, { label: 'Start', note: 'x' }), { label: 'Start', note: 'x' });
});

test('the page lists element nodes in reading order and skips other nodes', () => {
	const nodes: Node[] = [
		element('save', 'button', 200, 100),
		{ id: 'p', type: 'wpi-page', position: { x: 0, y: 0 }, data: { label: 'Page' } },
		element('name', 'input', 0, 100),
		element('title', 'text', 400, 0)
	];
	assert.deepEqual(pageJson(nodes), {
		schema_version: '0.2',
		layouter: 'flow-layouter',
		elements: [{ element: 'text' }, { element: 'input' }, { element: 'button' }]
	});
	assert.equal(nodes[0].id, 'save', 'the canvas order is left alone');
});

test('an empty canvas is a page with no elements', () => {
	assert.deepEqual(pageJson([]), { schema_version: '0.2', layouter: 'flow-layouter', elements: [] });
});

test('loading preserves instance order, duplicate classes and nested attributes', () => {
	const page: JsonObject = {
		elements: [
			{ element: 'input', binding: { path: '$input_doc.name' }, width: 200 },
			{ element: 'input', label: { en: 'Email', 'zh-cn': '邮箱' } },
			{ element: 'button', on_click: { action: 'save' } }
		]
	};
	const nodes = pageNodes(page);
	assert.equal(new Set(nodes.map((n) => n.id)).size, 3);
	assert.deepEqual(pageJson(nodes).elements, page.elements);
	assert.deepEqual(pageNodes({ elements: [] }), []);
	assert.throws(() => pageNodes({ elements: [null] }));
	assert.throws(() => pageNodes({}));
});

test('JSON editing accepts objects and reports invalid syntax or non-objects', async () => {
	const { parseJsonObject } = await import('./wpi-json');
	assert.deepEqual(parseJsonObject('{"name":"example"}'), { name: 'example' });
	assert.throws(() => parseJsonObject('{broken'));
	assert.throws(() => parseJsonObject('[]'));
	assert.throws(() => parseJsonObject('null'));
});

test('a table-backed list serializes its default load and list settings', () => {
 const node = element('list', 'list', 0, 0, { source: { table: 'kb.people', fields: [{ column: 'id', attribute: 'id' }] } });
 assert.deepEqual(nodeJson(node), { element: 'list', source: { table: 'kb.people', fields: [{ column: 'id', attribute: 'id' }], action: 'load', result: 'list' } });
});
