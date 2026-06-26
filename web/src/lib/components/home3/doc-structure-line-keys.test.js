import test from 'node:test';
import assert from 'node:assert/strict';

import { buildDocStructureLineViews, docStructureLineKey } from './doc-structure-line-keys.js';

test('doc structure line key matches page and line number contract', () => {
	assert.equal(docStructureLineKey({ page_number: 11, line_number: 203 }), '11:203');
});

test('doc structure line views produce unique ui keys for duplicate page-line pairs', () => {
	const views = buildDocStructureLineViews([
		{ page_number: 11, line_number: 202 },
		{ page_number: 11, line_number: 203 },
		{ page_number: 11, line_number: 203 },
		{ page_number: 12, line_number: 203 }
	]);

	assert.deepEqual(
		views.map((view) => view.lineKey),
		['11:202', '11:203', '11:203', '12:203']
	);
	assert.deepEqual(
		views.map((view) => view.uiKey),
		['11:202', '11:203', '11:203#2', '12:203']
	);
	assert.equal(new Set(views.map((view) => view.uiKey)).size, views.length);
});
