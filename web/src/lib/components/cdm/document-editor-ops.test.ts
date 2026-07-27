import test from 'node:test';
import assert from 'node:assert/strict';

import { extractBlockId, attributeToBlocks } from './document-editor-ops.js';

test('extractBlockId reads the id out of a model.Validate message', () => {
	assert.equal(extractBlockId('block "p1" has no type'), 'p1');
});

test('extractBlockId reads the id out of a message with two quoted strings, taking the first', () => {
	assert.equal(extractBlockId('block "p1" has unsupported type "bogus"'), 'p1');
});

test('extractBlockId reads the id out of a store.ConflictError message', () => {
	assert.equal(extractBlockId('cdm: block id "p1" already exists in this document'), 'p1');
});

test('extractBlockId reads the id out of a "duplicate block id" message', () => {
	assert.equal(extractBlockId('duplicate block id "p1"'), 'p1');
});

test('extractBlockId returns null for a message with no quoted id (block itself has no id)', () => {
	assert.equal(extractBlockId('block at position 2 in the document has an empty id'), null);
});

test('attributeToBlocks maps a violation list, preserving order and the original message', () => {
	const result = attributeToBlocks([
		'block "p1" has no type',
		'block at position 2 in the document has an empty id'
	]);
	assert.deepStrictEqual(result, [
		{ blockId: 'p1', message: 'block "p1" has no type' },
		{ blockId: null, message: 'block at position 2 in the document has an empty id' }
	]);
});
