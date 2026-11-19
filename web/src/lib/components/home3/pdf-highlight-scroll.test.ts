import test from 'node:test';
import assert from 'node:assert/strict';
import { tableHighlightScrollDelta } from './pdf-highlight-scroll';

test('scrolls just enough to show the caption and cited row together', () => {
	const caption = { top: 300, bottom: 320 };
	const row = { top: 710, bottom: 730 };
	assert.equal(tableHighlightScrollDelta([caption, row], [row], 640), 120);
});

test('leaves the viewport alone when both highlights are already visible', () => {
	const caption = { top: 5, bottom: 25 };
	const cell = { top: 590, bottom: 624 };
	assert.equal(tableHighlightScrollDelta([caption, cell], [cell], 640), 0);
});

test('shows the cited cell when the whole span cannot fit', () => {
	const caption = { top: 180, bottom: 210 };
	const cell = { top: 980, bottom: 1000 };
	assert.equal(tableHighlightScrollDelta([caption, cell], [cell], 640), 390);
});

test('keeps the visible cited row in place when the distant caption cannot fit', () => {
	const caption = { top: -750, bottom: -730 };
	const row = { top: 180, bottom: 205 };
	assert.equal(tableHighlightScrollDelta([caption, row], [row], 640), 0);
});

test('can scroll up to reveal a clipped caption while keeping the row visible', () => {
	const caption = { top: -40, bottom: -15 };
	const row = { top: 455, bottom: 480 };
	assert.equal(tableHighlightScrollDelta([caption, row], [row], 640), -70);
});

test('uses the available height when both highlights fit edge to edge', () => {
	const caption = { top: 90, bottom: 110 };
	const row = { top: 705, bottom: 730 };
	assert.equal(tableHighlightScrollDelta([caption, row], [row], 640), 90);
});

test('starts at the first cited table mark when even the table marks cannot fit together', () => {
	const caption = { top: 50, bottom: 70 };
	const firstRow = { top: 750, bottom: 770 };
	const lastRow = { top: 1450, bottom: 1470 };
	assert.equal(tableHighlightScrollDelta([caption, firstRow, lastRow], [lastRow, firstRow], 640), 160);
});
