import test from 'node:test';
import assert from 'node:assert/strict';

import { buildSemanticProjectionHighlightRect } from './semantic-projection-highlight-rect.js';

/** @param {number[]} rect */
function identityViewportRect(rect) {
	return rect;
}

test('buildSemanticProjectionHighlightRect merges many line boxes into one rectangle', () => {
	const viewport = {
		convertToViewportRectangle: identityViewportRect
	};

	const rect = buildSemanticProjectionHighlightRect(
		[
			{ line_number: 12, coords: [100, 210, 240, 230] },
			{ line_number: 13, coords: [110, 240, 300, 260] },
			{ line_number: 14, coords: [120, 270, 280, 290] }
		],
		viewport
	);

	assert.deepEqual(rect, {
		left: 100,
		top: 200,
		width: 220,
		height: 90,
		lineCount: 3
	});
});

test('buildSemanticProjectionHighlightRect returns null when no usable coordinates exist', () => {
	const viewport = {
		convertToViewportRectangle: identityViewportRect
	};

	assert.equal(buildSemanticProjectionHighlightRect([{ line_number: 12, coords: [] }], viewport), null);
});
