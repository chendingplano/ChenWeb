import test from 'node:test';
import assert from 'node:assert/strict';

import { buildKnowledgeStoreUserOptions } from './knowledge-store-user-options.js';

test('builds readable user options for the knowledge store dialog', () => {
	assert.deepEqual(
		buildKnowledgeStoreUserOptions([
			{ id: 'u-2', name: 'Ada Lovelace', email: 'ada@example.com' },
			{ id: 'u-1', name: '', email: 'grace@example.com' }
		]),
		[
			{ value: 'u-2', label: 'Ada Lovelace (ada@example.com)' },
			{ value: 'u-1', label: 'grace@example.com' }
		]
	);
});
