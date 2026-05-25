import test from 'node:test';
import assert from 'node:assert/strict';

import {
	getFixedTreeLayoutWidth,
	getFixedTreeLayoutHeight,
	getPanOffsetToRevealRect,
	getVisibleTreeDepth
} from './graph-layout-utils.js';

test('getVisibleTreeDepth counts only expanded visible descendants', () => {
	const nodes = [
		{ id: 'root', childIds: ['a', 'b'], expanded: true },
		{ id: 'a', childIds: ['a1'], expanded: false },
		{ id: 'a1', childIds: [], expanded: true },
		{ id: 'b', childIds: ['b1'], expanded: true },
		{ id: 'b1', childIds: [], expanded: true }
	];

	assert.equal(getVisibleTreeDepth(nodes), 2);
});

test('getFixedTreeLayoutWidth accounts for the hidden ECharts root level', () => {
	assert.equal(
		getFixedTreeLayoutWidth({
			visibleDepth: 1,
			parentChildDistance: 260
		}),
		520
	);
});

test('getFixedTreeLayoutWidth does not stretch shallow trees to the viewport', () => {
	assert.equal(
		getFixedTreeLayoutWidth({
			visibleDepth: 2,
			parentChildDistance: 260
		}),
		780
	);
});

test('getFixedTreeLayoutHeight grows by one canvas page per visible pagination page', () => {
	assert.equal(
		getFixedTreeLayoutHeight({
			visiblePageCount: 2,
			pageHeight: 500,
			minHeight: 520
		}),
		1000
	);
});

test('getPanOffsetToRevealRect pans left when children overflow the right edge', () => {
	assert.deepEqual(
		getPanOffsetToRevealRect({
			rect: { left: 900, right: 1180, top: 120, bottom: 260 },
			offsetX: -50,
			offsetY: 0,
			stageWidth: 1000,
			stageHeight: 600,
			margin: 80
		}),
		{ x: -310, y: 0, changed: true }
	);
});

test('getPanOffsetToRevealRect leaves the canvas alone when children are visible', () => {
	assert.deepEqual(
		getPanOffsetToRevealRect({
			rect: { left: 420, right: 760, top: 120, bottom: 260 },
			offsetX: -50,
			offsetY: 0,
			stageWidth: 1000,
			stageHeight: 600,
			margin: 80
		}),
		{ x: -50, y: 0, changed: false }
	);
});
