import test from 'node:test';
import assert from 'node:assert/strict';
import type { JsonObject } from '$lib/wpi/types';
import type { Node } from '@xyflow/svelte';
import { elementTree, nodeData, nodeJson, pageJson, pageNodes } from './wpi-json';

function element(id: string, cls: string, x: number, y: number, attrs?: Record<string, unknown>, pane = 'main'): Node {
	return { id, type: 'wpi-element', position: { x, y }, data: { label: `name of ${id}`, element: cls, pane, ...(attrs ? { attrs } : {}) } };
}

const single: JsonObject = { descriptor_id: 'p', schema_version: '0.5', page_layouter: { layouter: 'single-pane', root: { pane_id: 'main' } } };
const twoPanes: JsonObject = {
	descriptor_id: 'p',
	schema_version: '0.5',
	page_layouter: { layouter: 'single-pane', root: { pane_id: 'page', split: 'columns', children: [{ pane_id: 'list' }, { pane_id: 'detail' }] } },
	panels: { detail: { elem_layouter: { layouter: 'flow-layouter', config: { column_gap: '1rem' } }, elem_list: [] } }
};

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
	assert.deepEqual(data, { label: 'name of b', element: 'link', pane: 'main', attrs: { label: 'Send', width: 200 } });
	assert.deepEqual(nodeJson({ ...node, data }), { element: 'link', label: 'Send', width: 200 });
});

test('other nodes take the edited JSON as their data', () => {
	const page: Node = { id: 'p', type: 'wpi-page', position: { x: 0, y: 0 }, data: { label: 'Page' } };
	assert.deepEqual(nodeData(page, { label: 'Start', note: 'x' }), { label: 'Start', note: 'x' });
});

test('the page lists the element nodes of each pane in reading order and skips other nodes', () => {
	const nodes: Node[] = [
		element('save', 'button', 200, 100),
		{ id: 'p', type: 'wpi-page', position: { x: 0, y: 0 }, data: { label: 'Page' } },
		element('name', 'input', 0, 100),
		element('title', 'text', 400, 0)
	];
	assert.deepEqual(pageJson(nodes, single), {
		...single,
		panels: { main: { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [{ element: 'text' }, { element: 'input' }, { element: 'button' }] } }
	});
	assert.equal(nodes[0].id, 'save', 'the canvas order is left alone');
	const split = pageJson([element('a', 'text', 0, 0, {}, 'detail'), element('b', 'text', 0, 0, {}, 'list'), element('c', 'text', 0, 0, {}, 'gone')], twoPanes);
	assert.deepEqual(Object.keys(split.panels as JsonObject), ['list', 'detail']);
	assert.deepEqual((split.panels as JsonObject).detail, { elem_layouter: { layouter: 'flow-layouter', config: { column_gap: '1rem' } }, elem_list: [{ element: 'text' }] });
	assert.equal(((split.panels as JsonObject).list as JsonObject & { elem_list: unknown[] }).elem_list.length, 2, 'a node of an unknown pane goes to the first leaf');
});

test('an empty canvas is a page with no panels', () => {
	assert.deepEqual(pageJson([], single), { ...single, panels: {} });
});

test('class copies stay out of the node JSON and come back in the page', () => {
	const copy = { descriptor_id: 'text', schema_version: '0.4', code_module_id: 'text' };
	const node = { ...element('t', 'text', 0, 0, { text: 'Hi' }), data: { label: 't', element: 'text', attrs: { text: 'Hi' }, pane: 'main', class: copy } };
	assert.deepEqual(nodeJson(node), { element: 'text', text: 'Hi' });
	assert.equal(nodeData(node, { element: 'text', text: 'Bye' }).class, copy, 'editing the JSON keeps the copy');
	assert.deepEqual((pageJson([node], single).panels as JsonObject).main, { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [{ element: 'text', text: 'Hi', class: copy }] });
});

test('loading preserves each panel order, duplicate classes, copies and nested attributes', () => {
	const page: JsonObject = {
		...twoPanes,
		panels: {
			list: { elem_layouter: { layouter: 'flow-layouter' }, elem_list: [
				{ element: 'input', binding: { path: '$input_doc.name' }, width: 200 },
				{ element: 'input', label: { en: 'Email', 'zh-cn': '邮箱' }, class: { descriptor_id: 'input' } }
			] },
			detail: { elem_layouter: { layouter: 'flow-layouter', config: { column_gap: '1rem' } }, elem_list: [{ element: 'button', on_click: { action: 'save' } }] }
		}
	};
	const nodes = pageNodes(page);
	assert.equal(new Set(nodes.map((n) => n.id)).size, 3);
	assert.deepEqual(nodes.map((n) => n.data.pane), ['list', 'list', 'detail']);
	assert.deepEqual(pageJson(nodes, page), page);
	assert.deepEqual(pageNodes(single), []);
	assert.throws(() => pageNodes({ ...single, panels: { main: { elem_list: [null] } } }));
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

test('element tree follows the page pane hierarchy and element reading order', () => {
	const nodes = [element('b', 'button', 20, 10, undefined, 'list'), element('a', 'input', 0, 0, undefined, 'list'), element('c', 'text', 0, 0, undefined, 'detail')];
	assert.equal(elementTree(nodes, twoPanes, { pane: 'pane', panel: 'panel', layouter: 'Layouter', elements: 'Element List' }), 'page pane\n  list pane\n    panel\n      Layouter: flow-layouter\n      Element List\n        input\n        button\n  detail pane\n    panel\n      Layouter: flow-layouter\n      Element List\n        text');
});

test('an empty leaf without a panel has no layouter name', () => {
	assert.equal(elementTree([], single, { pane: 'pane', panel: 'panel', layouter: 'Layouter', elements: 'Element List' }), 'main pane\n  panel\n    Layouter: \n    Element List');
});

test('element tree shows the configured panel layouter name', () => {
	const page = { ...single, panels: { main: { elem_layouter: { layouter: 'custom-layouter' } } } };
	assert.match(elementTree([], page, { pane: 'pane', panel: 'panel', layouter: 'Layouter', elements: 'Element List' }), /Layouter: custom-layouter/);
});
