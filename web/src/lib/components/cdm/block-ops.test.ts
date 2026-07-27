import test from 'node:test';
import assert from 'node:assert/strict';

import {
	insertBlockAt,
	deleteBlockById,
	moveBlock,
	changeContentBlockType,
	isContentBearingType
} from './block-ops.js';
import type { Block } from './types.js';

function para(id: string, text = ''): Block {
	return { id, type: 'paragraph', content: [{ type: 'text', text }] };
}

test('insertBlockAt inserts at the given index without disturbing other ids', () => {
	const blocks = [para('a'), para('c')];
	const result = insertBlockAt(blocks, 1, para('b'));
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['a', 'b', 'c']
	);
});

test('insertBlockAt does not mutate the original array', () => {
	const blocks = [para('a')];
	insertBlockAt(blocks, 1, para('b'));
	assert.equal(blocks.length, 1, 'original array must be unchanged');
});

test('deleteBlockById removes exactly the named block', () => {
	const blocks = [para('a'), para('b'), para('c')];
	const result = deleteBlockById(blocks, 'b');
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['a', 'c']
	);
});

test('deleteBlockById on an unknown id is a no-op copy', () => {
	const blocks = [para('a')];
	const result = deleteBlockById(blocks, 'does-not-exist');
	assert.deepStrictEqual(result, blocks);
	assert.notEqual(result, blocks, 'should still be a new array, not the same reference');
});

// This is the direct proof for task 4.4: reordering must not touch any
// block's id, since anchors, cross_references, and artifact provenance all
// bind to the id (D9), not to array position.
test('moveBlock swaps two blocks and every id survives unchanged', () => {
	const blocks = [para('a'), para('b'), para('c')];
	const result = moveBlock(blocks, 'b', 'up');
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['b', 'a', 'c']
	);
	// Same three blocks, not rebuilt copies with fresh identity.
	assert.equal(result[0], blocks[1]);
	assert.equal(result[1], blocks[0]);
});

test('moveBlock "down" swaps with the following block', () => {
	const blocks = [para('a'), para('b'), para('c')];
	const result = moveBlock(blocks, 'b', 'down');
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['a', 'c', 'b']
	);
});

test('moveBlock at the top boundary moving up is a no-op', () => {
	const blocks = [para('a'), para('b')];
	const result = moveBlock(blocks, 'a', 'up');
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['a', 'b']
	);
});

test('moveBlock at the bottom boundary moving down is a no-op', () => {
	const blocks = [para('a'), para('b')];
	const result = moveBlock(blocks, 'b', 'down');
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['a', 'b']
	);
});

test('moveBlock on an unknown id is a no-op', () => {
	const blocks = [para('a'), para('b')];
	const result = moveBlock(blocks, 'nope', 'up');
	assert.deepStrictEqual(
		result.map((b) => b.id),
		['a', 'b']
	);
});

test('isContentBearingType accepts exactly paragraph/heading/quote', () => {
	assert.ok(isContentBearingType('paragraph'));
	assert.ok(isContentBearingType('heading'));
	assert.ok(isContentBearingType('quote'));
	assert.ok(!isContentBearingType('table'));
	assert.ok(!isContentBearingType('callout'));
});

// The other half of task 4.4: an in-place edit -- here, a type change --
// must also leave the id untouched.
test('changeContentBlockType preserves id and content, id survives unchanged', () => {
	const block = para('intro', 'Some text');
	const changed = changeContentBlockType(block, 'heading');
	assert.equal(changed.id, 'intro');
	assert.equal(changed.type, 'heading');
	assert.deepStrictEqual(changed.content, block.content);
});

test('changeContentBlockType to heading assigns a default level when none exists', () => {
	const changed = changeContentBlockType(para('p'), 'heading');
	assert.equal(changed.level, 2);
});

test('changeContentBlockType to heading preserves an existing valid level', () => {
	const heading: Block = {
		id: 'h',
		type: 'heading',
		level: 4,
		content: [{ type: 'text', text: 'x' }]
	};
	const changed = changeContentBlockType(heading, 'heading');
	assert.equal(changed.level, 4);
});

test('changeContentBlockType away from heading drops level', () => {
	const heading: Block = {
		id: 'h',
		type: 'heading',
		level: 3,
		content: [{ type: 'text', text: 'x' }]
	};
	const changed = changeContentBlockType(heading, 'paragraph');
	assert.equal(changed.level, undefined);
});

test('changeContentBlockType rejects a non-content-bearing source type', () => {
	const table: Block = { id: 't', type: 'table', columns: [], rows: [] };
	assert.throws(() => changeContentBlockType(table, 'paragraph'), /only paragraph\/heading\/quote/);
});
