import test from 'node:test';
import assert from 'node:assert/strict';
import { sameCanvasContent } from './canvas-dirty';

const baseline = { nodes: [{ id: 'one', position: { x: 0, y: 0 }, data: { label: 'One' } }], edges: [] };
test('selection and measurements do not mark a canvas dirty', () => {
	assert.equal(sameCanvasContent(baseline, { ...baseline, nodes: [{ ...baseline.nodes[0], selected: true, measured: { width: 100, height: 50 } }] }), true);
});
test('changes to node data or position mark a canvas dirty', () => {
	assert.equal(sameCanvasContent(baseline, { ...baseline, nodes: [{ ...baseline.nodes[0], data: { label: 'Two' } }] }), false);
	assert.equal(sameCanvasContent(baseline, { ...baseline, nodes: [{ ...baseline.nodes[0], position: { x: 1, y: 0 } }] }), false);
});
