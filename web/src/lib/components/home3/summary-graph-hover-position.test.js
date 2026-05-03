import test from 'node:test';
import assert from 'node:assert/strict';

import {
	computeHoverCardPosition,
	isPointInHoverKeepAliveZone,
	toContainerLocalPoint
} from './summary-graph-hover-position.js';

test('places the hover card left when there is enough room on the left', () => {
	const pos = computeHoverCardPosition({
		nodeX: 1200,
		nodeY: 280,
		nodeRadius: 8,
		stageWidth: 1600,
		stageHeight: 900
	});

	assert.equal(pos.x + 428, 1492, 'expected the left side to win before any vertical placement');
});

test('places the hover card right when left does not fit but right does', () => {
	const pos = computeHoverCardPosition({
		nodeX: 120,
		nodeY: 280,
		nodeRadius: 8,
		stageWidth: 1600,
		stageHeight: 900
	});

	assert.equal(pos.x, 513, 'expected the right side to win after left is rejected');
});

test('places the hover card above when left and right do not fit but top does', () => {
	const pos = computeHoverCardPosition({
		nodeX: 240,
		nodeY: 500,
		nodeRadius: 6,
		nodeWidth: 460,
		nodeHeight: 64,
		stageWidth: 520,
		stageHeight: 900
	});

	assert.equal(pos.y + 320, 468, 'expected the top side to win before bottom');
});

test('uses bottom only when left, right, and top do not fit', () => {
	const pos = computeHoverCardPosition({
		nodeX: 240,
		nodeY: 60,
		nodeRadius: 6,
		nodeWidth: 460,
		nodeHeight: 64,
		stageWidth: 520,
		stageHeight: 900
	});

	assert.equal(pos.y, 162, 'expected the bottom side to be the last fallback');
});

test('uses rectangular node height when placing below topic graph nodes', () => {
	const pos = computeHoverCardPosition({
		nodeX: 240,
		nodeY: 60,
		nodeRadius: 6,
		nodeWidth: 460,
		nodeHeight: 64,
		stageWidth: 520,
		stageHeight: 900,
		gap: 5
	});

	assert.equal(pos.y, 162, 'expected the rectangular node height to affect bottom placement');
});

test('places the hover card left when the node is near the right edge', () => {
	const pos = computeHoverCardPosition({
		nodeX: 1045,
		nodeY: 840,
		nodeRadius: 32,
		nodeWidth: 160,
		nodeHeight: 64,
		stageWidth: 1399,
		stageHeight: 900
	});

	assert.equal(pos.x, 837, 'expected the card to open to the left of the hovered topic node');
	assert.equal(pos.y, 564, 'expected the left placement to stay as centered as the stage bounds allow');
});

test('never uses bottom when top is available and horizontal placement is impossible', () => {
	const pos = computeHoverCardPosition({
		nodeX: 240,
		nodeY: 500,
		nodeRadius: 6,
		nodeWidth: 460,
		nodeHeight: 64,
		stageWidth: 520,
		stageHeight: 620
	});

	assert.equal(pos.y + 320, 468, 'expected the top side to beat bottom when both are possible');
});

test('never allows a negative gap to push the card back over the node', () => {
	const pos = computeHoverCardPosition({
		nodeX: 900,
		nodeY: 500,
		nodeRadius: 8,
		stageWidth: 1600,
		stageHeight: 900,
		gap: -20
	});

	assert.equal(pos.x + 428, 1192, 'expected negative gap values to clamp without breaking left preference');
});

test('keeps the hover card alive while the pointer moves through the buffer between node and card', () => {
	assert.equal(
		isPointInHoverKeepAliveZone({
			pointX: 1490,
			pointY: 360,
			nodeX: 1520,
			nodeY: 360,
			nodeRadius: 8,
			cardX: 1082,
			cardY: 200
		}),
		true
	);
});

test('allows the hover card to hide once the pointer leaves the buffered bridge area', () => {
	assert.equal(
		isPointInHoverKeepAliveZone({
			pointX: 980,
			pointY: 560,
			nodeX: 1520,
			nodeY: 360,
			nodeRadius: 8,
			cardX: 1082,
			cardY: 200
		}),
		false
	);
});

test('converts global chart coordinates into graph-stage local coordinates', () => {
	assert.deepEqual(
		toContainerLocalPoint({
			globalX: 990,
			globalY: 680,
			containerLeft: 210,
			containerTop: 180
		}),
		{ x: 780, y: 500 }
	);
});
