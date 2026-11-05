import test from 'node:test';
import assert from 'node:assert/strict';
import type { Node } from '@xyflow/svelte';
import { dropIndex, moveElementNode, type Box } from './element-order';

const node = (id: string, x: number, y: number, pane = 'main'): Node => ({ id, type: 'wpi-element', position: { x, y }, data: { label: id, element: 'button', pane } });
const order = (nodes: Node[], pane = 'main') => nodes.filter((n) => n.data.pane === pane).sort((a, b) => a.position.y - b.position.y || a.position.x - b.position.x).map((n) => n.id);

test('dragging A between B and C gives B, A, C', () => {
	const nodes = [node('A', 0, 0), node('B', 240, 0), node('C', 480, 0)];
	assert.deepEqual(order(moveElementNode(nodes, 'A', 1)), ['B', 'A', 'C']);
});

test('dragging an element backwards and to the end', () => {
	const nodes = [node('A', 0, 0), node('B', 240, 0), node('C', 480, 0)];
	assert.deepEqual(order(moveElementNode(nodes, 'C', 0)), ['C', 'A', 'B']);
	assert.deepEqual(order(moveElementNode(nodes, 'A', 2)), ['B', 'C', 'A']);
});

test('the slots stay where they were; only the elements in them change', () => {
	const nodes = [node('A', 0, 0), node('B', 240, 0), node('C', 480, 0)];
	const got = moveElementNode(nodes, 'A', 1);
	assert.deepEqual(got.map((n) => n.position), [{ x: 240, y: 0 }, { x: 0, y: 0 }, { x: 480, y: 0 }]);
});

test('other panes are not touched', () => {
	const nodes = [node('A', 0, 0), node('B', 240, 0), node('X', 0, 0, 'side')];
	const got = moveElementNode(nodes, 'A', 1);
	assert.deepEqual(got[2], nodes[2]);
	assert.deepEqual(order(got), ['B', 'A']);
});

test('an unknown element is not moved', () => {
	const nodes = [node('A', 0, 0)];
	assert.equal(moveElementNode(nodes, 'x', 0), nodes);
});

// Two rows of two elements: B C / D E.
const grid: Box[] = [
	{ left: 0, top: 0, right: 100, bottom: 50 },
	{ left: 100, top: 0, right: 200, bottom: 50 },
	{ left: 0, top: 60, right: 100, bottom: 110 },
	{ left: 100, top: 60, right: 200, bottom: 110 },
];

test('a drop left of an element’s centre lands before it', () => {
	assert.equal(dropIndex(grid, 10, 25), 0);
	assert.equal(dropIndex(grid, 140, 25), 1);
});

test('a drop right of an element’s centre lands after it', () => {
	assert.equal(dropIndex(grid, 90, 25), 1);
	assert.equal(dropIndex(grid, 190, 25), 2);
});

test('a drop at the end of a row lands before the next row', () => {
	assert.equal(dropIndex(grid, 190, 55), 2);
	assert.equal(dropIndex(grid, 10, 90), 2);
	assert.equal(dropIndex(grid, 190, 100), 4);
});

test('a drop into an empty pane lands at 0', () => {
	assert.equal(dropIndex([], 10, 10), 0);
});
