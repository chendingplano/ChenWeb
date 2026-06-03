import test from 'node:test';
import assert from 'node:assert/strict';

import { buildKbSearchPaginationItems, clampKbSearchPage } from './kb-search-pagination.js';

test('clampKbSearchPage keeps invalid values within the available page range', () => {
	assert.equal(clampKbSearchPage(0, 5), 1);
	assert.equal(clampKbSearchPage(3, 5), 3);
	assert.equal(clampKbSearchPage(9, 5), 5);
	assert.equal(clampKbSearchPage(Number.NaN, 5), 1);
});

test('buildKbSearchPaginationItems shows nearby pages and ellipses for long result sets', () => {
	assert.deepEqual(buildKbSearchPaginationItems(6, 12), [1, 'ellipsis', 5, 6, 7, 'ellipsis', 12]);
});

test('buildKbSearchPaginationItems expands small result sets without ellipses', () => {
	assert.deepEqual(buildKbSearchPaginationItems(3, 5), [1, 2, 3, 4, 5]);
});
