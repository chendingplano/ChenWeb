import { expect, test } from 'bun:test';
import { placePdfOverlay } from './pdf-overlay-placement';

const pane = { left: 8, top: 100, right: 792, bottom: 620 };
const caption = { left: 320, top: 130, right: 500, bottom: 150 };
const row = { left: 50, top: 258, right: 750, bottom: 280 };

test('keeps the full card clear of a separated table caption and row', () => {
	expect(placePdfOverlay(pane, [caption, row], 320, 205)).toEqual({
		left: 472, top: 292, maxHeight: 328
	});
});

test('keeps the usual position above a single highlight when it fits', () => {
	expect(placePdfOverlay(pane, [row], 320, 120)).toEqual({
		left: 472, top: 126, maxHeight: 120
	});
});

test('finds room to the left of a tall highlight at the right edge', () => {
	const rightColumn = { left: 550, top: 80, right: 795, bottom: 650 };
	expect(placePdfOverlay(pane, [rightColumn], 320, 205)).toEqual({
		left: 218, top: 415, maxHeight: 205
	});
});

test('uses the largest clear band when the visible page cannot fit the full card', () => {
	const wide = { left: 0, top: 190, right: 800, bottom: 230 };
	const scrolledPane = { ...pane, bottom: 340 };
	expect(placePdfOverlay(scrolledPane, [wide], 320, 205)).toEqual({
		left: 472, top: 242, maxHeight: 98
	});
});

test('returns no position when the highlight covers the visible page', () => {
	expect(placePdfOverlay(pane, [pane], 320, 205)).toBeNull();
});
