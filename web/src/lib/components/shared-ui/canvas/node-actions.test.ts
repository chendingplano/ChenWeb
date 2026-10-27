import test from 'node:test';
import assert from 'node:assert/strict';
import type { Edge, Node } from '@xyflow/svelte';
import { duplicateNode, removeNode } from './node-actions';

const node = (id: string, x: number, y: number, extra: Partial<Node> = {}): Node => ({ id, position: { x, y }, data: {}, ...extra });

test('a copy keeps the data and size, sits one step away and becomes the selection', () => {
	const nodes = [
		node('a', 0, 0, { type: 'wpi-element', selected: true, width: 200, height: 80, data: { label: 'Name', element: 'input', attrs: { label: 'Name' } } }),
		node('b', 100, 0)
	];
	const got = duplicateNode(nodes, 'a', 'a2', 20);
	assert.equal(got.length, 3);
	assert.deepEqual(got[2], { ...nodes[0], id: 'a2', position: { x: 20, y: 20 }, selected: true });
	assert.ok(got.slice(0, 2).every((n) => !n.selected));
	// The copy's data is its own.
	(got[2].data.attrs as Record<string, unknown>).label = 'Other';
	assert.equal((nodes[0].data.attrs as Record<string, unknown>).label, 'Name');
});

test('a copy steps past nodes already at its place', () => {
	const got = duplicateNode([node('a', 0, 0), node('b', 20, 20), node('c', 40, 40)], 'a', 'a2', 20);
	assert.deepEqual(got[3].position, { x: 60, y: 60 });
});

test('an unknown node is not copied', () => {
	const nodes = [node('a', 0, 0)];
	assert.equal(duplicateNode(nodes, 'x', 'x2', 20), nodes);
});

test('removing a node removes the edges that touch it', () => {
	const edges: Edge[] = [
		{ id: 'e1', source: 'a', target: 'b' },
		{ id: 'e2', source: 'b', target: 'c' },
		{ id: 'e3', source: 'c', target: 'a' }
	];
	const got = removeNode([node('a', 0, 0), node('b', 0, 0), node('c', 0, 0)], edges, 'a');
	assert.deepEqual(got.nodes.map((n) => n.id), ['b', 'c']);
	assert.deepEqual(got.edges.map((e) => e.id), ['e2']);
});
