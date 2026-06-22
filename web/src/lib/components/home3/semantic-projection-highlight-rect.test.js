import test from 'node:test';
import assert from 'node:assert/strict';

import { buildSemanticProjectionHighlightRect } from './semantic-projection-highlight-rect.js';

// width=1000, height=1000 → MinerU coords map 1:1 to viewport pixels
const identityViewport = { width: 1000, height: 1000 };

test('buildSemanticProjectionHighlightRect merges many line boxes into one rectangle', () => {
	const rect = buildSemanticProjectionHighlightRect(
		[
			{ line_number: 12, coords: [100, 210, 240, 230] },
			{ line_number: 13, coords: [110, 240, 300, 260] },
			{ line_number: 14, coords: [120, 270, 280, 290] }
		],
		identityViewport
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
	assert.equal(buildSemanticProjectionHighlightRect([{ line_number: 12, coords: [] }], identityViewport), null);
});
