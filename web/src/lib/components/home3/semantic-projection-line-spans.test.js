import test from 'node:test';
import assert from 'node:assert/strict';

import {
	formatSemanticProjectionLineSpans,
	normalizeSemanticProjectionLineRefs
} from './semantic-projection-line-spans.js';

test('formatSemanticProjectionLineSpans keeps compact span labels', () => {
	assert.deepEqual(
		formatSemanticProjectionLineSpans(['12:14', '18', ' ', 7, { line_number: 9 }]),
		['12:14', '18', '7', '9']
	);
});

test('normalizeSemanticProjectionLineRefs resolves page numbers from raw lines', () => {
	const refs = normalizeSemanticProjectionLineRefs(['12:13', '18'], [
		{ page_number: 2, line_number: 12 },
		{ page_number: 2, line_number: 13 },
		{ page_number: 3, line_number: 18 }
	]);

	assert.deepEqual(refs, [
		{ page_number: 2, line_number: 12 },
		{ page_number: 2, line_number: 13 },
		{ page_number: 3, line_number: 18 }
	]);
});
