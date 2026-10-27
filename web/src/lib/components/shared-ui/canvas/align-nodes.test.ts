import test from 'node:test';
import assert from 'node:assert/strict';
import { alignNodes, distributeNodes, selectedCount, type AlignableNode } from './align-nodes';

function node(id: string, x: number, y: number, w: number, h: number, selected = true): AlignableNode {
	return { id, position: { x, y }, selected, measured: { width: w, height: h } };
}

const pos = (nodes: AlignableNode[]) => Object.fromEntries(nodes.map((n) => [n.id, n.position]));

test('align left, centre and right use the selection bounds', () => {
	const nodes = [node('a', 10, 0, 100, 40), node('b', 50, 100, 200, 40)];
	assert.deepEqual(pos(alignNodes(nodes, 'left')), { a: { x: 10, y: 0 }, b: { x: 10, y: 100 } });
	// bounds 10..250, centre 130
	assert.deepEqual(pos(alignNodes(nodes, 'center')), { a: { x: 80, y: 0 }, b: { x: 30, y: 100 } });
	assert.deepEqual(pos(alignNodes(nodes, 'right')), { a: { x: 150, y: 0 }, b: { x: 50, y: 100 } });
});

test('align top, middle and bottom use the selection bounds', () => {
	const nodes = [node('a', 0, 20, 50, 60), node('b', 100, 0, 50, 20)];
	assert.deepEqual(pos(alignNodes(nodes, 'top')), { a: { x: 0, y: 0 }, b: { x: 100, y: 0 } });
	// bounds 0..80, middle 40
	assert.deepEqual(pos(alignNodes(nodes, 'middle')), { a: { x: 0, y: 10 }, b: { x: 100, y: 30 } });
	assert.deepEqual(pos(alignNodes(nodes, 'bottom')), { a: { x: 0, y: 20 }, b: { x: 100, y: 60 } });
});

test('align leaves unselected nodes alone and needs two selected', () => {
	const nodes = [node('a', 10, 0, 100, 40), node('b', 50, 0, 100, 40), node('c', 500, 0, 10, 10, false)];
	const out = alignNodes(nodes, 'left');
	assert.equal(out[2], nodes[2]);
	assert.deepEqual(out[1].position, { x: 10, y: 0 });

	const one = [node('a', 10, 0, 100, 40), node('b', 50, 0, 100, 40, false)];
	assert.equal(alignNodes(one, 'left'), one);
});

test('align falls back to width/height, then zero, when a node is not measured', () => {
	const nodes: AlignableNode[] = [
		{ id: 'a', position: { x: 0, y: 0 }, selected: true, width: 100 },
		{ id: 'b', position: { x: 40, y: 0 }, selected: true }
	];
	assert.deepEqual(pos(alignNodes(nodes, 'right')), { a: { x: 0, y: 0 }, b: { x: 100, y: 0 } });
});

test('distribute horizontally makes equal gaps and keeps the ends', () => {
	// spans 0..300; widths 50+100+50 = 200; gap 50
	const nodes = [node('c', 250, 5, 50, 10), node('a', 0, 0, 50, 10), node('b', 60, 9, 100, 10)];
	assert.deepEqual(pos(distributeNodes(nodes, 'horizontal')), {
		a: { x: 0, y: 0 },
		b: { x: 100, y: 9 },
		c: { x: 250, y: 5 }
	});
});

test('distribute vertically makes equal gaps and keeps the ends', () => {
	// spans 0..120; heights 20*3 = 60; gap 30
	const nodes = [node('a', 0, 0, 10, 20), node('b', 3, 90, 10, 20), node('c', 7, 100, 10, 20)];
	assert.deepEqual(pos(distributeNodes(nodes, 'vertical')), {
		a: { x: 0, y: 0 },
		b: { x: 3, y: 50 },
		c: { x: 7, y: 100 }
	});
});

test('distribute needs three selected nodes', () => {
	const nodes = [node('a', 0, 0, 10, 10), node('b', 50, 0, 10, 10), node('c', 70, 0, 10, 10, false)];
	assert.equal(distributeNodes(nodes, 'horizontal'), nodes);
	assert.equal(selectedCount(nodes), 2);
});
