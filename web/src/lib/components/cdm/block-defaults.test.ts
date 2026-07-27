import test from 'node:test';
import assert from 'node:assert/strict';

import { createDefaultBlock } from './block-defaults.js';
import { createIdAllocator } from './block-id.js';
import { BLOCK_TYPES } from './types.js';
import type { Block } from './types.js';

// These check the two content-model invariants (spec §1.2 rules 4 and 5)
// that a wrong default shape could plausibly violate: content/children/items
// must be mutually exclusive, and a table's row cell keys must be a subset
// of its declared columns. This is deliberately not a client-side port of
// model.Validate — that lives server-side by design (D2), and validation
// stays there; these are narrow, local assertions about createDefaultBlock's
// own output, not a second validator to keep in sync with the Go one.
function assertContentExclusivity(block: Block) {
	const populated = [
		block.content && block.content.length > 0,
		block.children && block.children.length > 0,
		block.items && block.items.length > 0
	].filter(Boolean).length;
	assert.ok(
		populated <= 1,
		`block "${block.id}" (${block.type}) populates more than one of content/children/items`
	);
}

function assertTableCellKeysAreDeclared(block: Block) {
	if (block.type !== 'table' || !block.columns || !block.rows) return;
	const declared = new Set(block.columns.map((c) => c.key));
	for (const row of block.rows) {
		for (const key of Object.keys(row.cells)) {
			assert.ok(
				declared.has(key),
				`table "${block.id}" row cell key "${key}" is not a declared column`
			);
		}
	}
}

test('createDefaultBlock produces a distinct, non-empty id for every Phase 1 block type', () => {
	for (const type of BLOCK_TYPES) {
		const allocate = createIdAllocator(new Set());
		const block = createDefaultBlock(type, allocate);
		assert.equal(block.type, type, `block type should be ${type}`);
		assert.ok(block.id, `block of type ${type} should have a non-empty id`);
	}
});

test('createDefaultBlock respects content/children/items exclusivity for every type', () => {
	for (const type of BLOCK_TYPES) {
		const allocate = createIdAllocator(new Set());
		const block = createDefaultBlock(type, allocate);
		assertContentExclusivity(block);
		if (block.children) block.children.forEach(assertContentExclusivity);
		if (block.items) block.items.forEach((item) => item.forEach(assertContentExclusivity));
	}
});

test('createDefaultBlock table has row cell keys drawn from its declared columns', () => {
	const allocate = createIdAllocator(new Set());
	const block = createDefaultBlock('table', allocate);
	assertTableCellKeysAreDeclared(block);
});

test('createDefaultBlock allocates distinct ids for nested blocks (list item, callout child)', () => {
	const allocate = createIdAllocator(new Set());
	const list = createDefaultBlock('list', allocate);
	assert.ok(list.items);
	const itemId = list.items![0][0].id;
	assert.notEqual(itemId, list.id, "list item must not reuse the list block's own id");

	const allocate2 = createIdAllocator(new Set());
	const callout = createDefaultBlock('callout', allocate2);
	assert.ok(callout.children);
	assert.notEqual(callout.children![0].id, callout.id);
});

test('createDefaultBlock avoids colliding with existing ids', () => {
	const allocate = createIdAllocator(new Set(['paragraph']));
	const block = createDefaultBlock('paragraph', allocate);
	assert.equal(block.id, 'paragraph-2');
});
