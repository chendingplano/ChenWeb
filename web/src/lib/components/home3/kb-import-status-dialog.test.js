import test from 'node:test';
import assert from 'node:assert/strict';

import { shouldShowOverflowScrollbar } from './kb-import-status-dialog.js';

test('returns false when content fits in the visible area', () => {
	assert.equal(shouldShowOverflowScrollbar(440, 440), false);
	assert.equal(shouldShowOverflowScrollbar(440.5, 440), false);
});

test('returns true when content exceeds the visible area', () => {
	assert.equal(shouldShowOverflowScrollbar(442, 440), true);
});

test('returns false for non-numeric measurements', () => {
	assert.equal(shouldShowOverflowScrollbar(Number.NaN, 440), false);
	assert.equal(shouldShowOverflowScrollbar(440, Number.NaN), false);
});
