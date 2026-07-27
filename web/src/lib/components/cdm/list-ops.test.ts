import test from 'node:test';
import assert from 'node:assert/strict';

import { addListItem, removeListItem } from './list-ops.js';
import { createIdAllocator } from './block-id.js';
import type { Block } from './types.js';

function list(): Block {
	return {
		id: 'list1',
		type: 'list',
		ordered: false,
		items: [
			[{ id: 'list1-1', type: 'paragraph', content: [{ type: 'text', text: 'First' }] }],
			[{ id: 'list1-2', type: 'paragraph', content: [{ type: 'text', text: 'Second' }] }]
		]
	};
}

test('addListItem appends a new item with one default paragraph', () => {
	const allocate = createIdAllocator(new Set(['list1', 'list1-1', 'list1-2']));
	const result = addListItem(list(), allocate);
	assert.equal(result.items!.length, 3);
	assert.equal(result.items![2].length, 1);
	assert.equal(result.items![2][0].type, 'paragraph');
	assert.ok(result.items![2][0].id, 'the new item block should have an allocated id');
});

test('addListItem does not mutate the original block', () => {
	const original = list();
	addListItem(original, createIdAllocator(new Set()));
	assert.equal(original.items!.length, 2);
});

test('removeListItem removes exactly the item at the given index, others unaffected', () => {
	const result = removeListItem(list(), 0);
	assert.equal(result.items!.length, 1);
	assert.equal(result.items![0][0].id, 'list1-2');
});
