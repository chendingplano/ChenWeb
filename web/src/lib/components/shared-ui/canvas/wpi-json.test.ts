import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import { nodeJson, pageJson } from './wpi-json';

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
