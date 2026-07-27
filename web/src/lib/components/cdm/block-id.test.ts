import test from 'node:test';
import assert from 'node:assert/strict';

import { slugify, allocateBlockId, collectBlockIds, createIdAllocator } from './block-id.js';
import type { Block } from './types.js';

test('slugify collapses non-alphanumeric runs to a single hyphen', () => {
	assert.equal(slugify('Score Range'), 'score-range');
	assert.equal(slugify('  Leading/Trailing  '), 'leading-trailing');
	assert.equal(slugify('Multiple---Hyphens___Here'), 'multiple-hyphens-here');
});

test('slugify of non-Latin text strips to empty', () => {
	// ASCII-oriented by design (see block-id.ts doc comment): the caller
	// falls back to the block type when this happens.
	assert.equal(slugify('日本語のテキスト'), '');
});

// This is the exact example from spec 2026072502-spec-cdm-editor.md §6
// Open Question / D9 discussion: a heading block with the text "Score
// Range" should receive a readable slug, not a UUID.
test('allocateBlockId prefers a slug derived from heading text', () => {
	const id = allocateBlockId({ type: 'heading', headingText: 'Score Range' }, new Set());
	assert.equal(id, 'score-range');
});

test('allocateBlockId falls back to the block type when there is no heading text', () => {
	const id = allocateBlockId({ type: 'paragraph' }, new Set());
	assert.equal(id, 'paragraph');
});

test('allocateBlockId falls back to the block type when heading text strips to empty', () => {
	const id = allocateBlockId({ type: 'heading', headingText: '日本語' }, new Set());
	assert.equal(id, 'heading');
});

test('allocateBlockId disambiguates on collision with a numeric suffix', () => {
	const existing = new Set(['score-range']);
	const id = allocateBlockId({ type: 'heading', headingText: 'Score Range' }, existing);
	assert.equal(id, 'score-range-2');
});

test('allocateBlockId finds the next free suffix past multiple collisions', () => {
	const existing = new Set(['paragraph', 'paragraph-2', 'paragraph-3']);
	const id = allocateBlockId({ type: 'paragraph' }, existing);
	assert.equal(id, 'paragraph-4');
});

test('collectBlockIds walks children (callout) and list items', () => {
	const blocks: Block[] = [
		{ id: 'intro', type: 'paragraph', content: [{ type: 'text', text: 'x' }] },
		{
			id: 'list1',
			type: 'list',
			items: [
				[{ id: 'list1-1', type: 'paragraph', content: [] }],
				[{ id: 'list1-2', type: 'paragraph', content: [] }]
			]
		},
		{
			id: 'callout1',
			type: 'callout',
			role: 'warning',
			children: [{ id: 'callout1-body', type: 'paragraph', content: [] }]
		}
	];

	const ids = collectBlockIds(blocks);
	assert.deepStrictEqual(
		[...ids].sort(),
		['callout1', 'callout1-body', 'intro', 'list1', 'list1-1', 'list1-2'].sort()
	);
});

test('collectBlockIds on an empty document returns an empty set', () => {
	assert.equal(collectBlockIds([]).size, 0);
});

test('createIdAllocator reserves each allocated id so a later call in the same batch cannot reuse it', () => {
	const allocate = createIdAllocator(new Set());
	const first = allocate({ type: 'paragraph' });
	const second = allocate({ type: 'paragraph' });
	assert.equal(first, 'paragraph');
	assert.equal(second, 'paragraph-2');
});

test('createIdAllocator honors ids already present in the seed set', () => {
	const allocate = createIdAllocator(new Set(['heading']));
	assert.equal(allocate({ type: 'heading' }), 'heading-2');
});
