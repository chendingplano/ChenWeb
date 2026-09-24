import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildKbInputRecordMetadataRows,
	buildUserSelectOptions,
	buildKbInputUpdatePayloadForMetadataEdit
} from './kb-input-metadata.js';

const input = {
	id: 7,
	type: 'pdf',
	tenant_id: 'user-2',
	create_time: '2026-09-24T10:00:00Z',
	modify_time: '2026-09-24T10:00:00Z'
};

test('builds tenant_id as a user selector row with readable user options', () => {
	const users = [
		{ id: 'user-2', name: 'Ada Lovelace', email: 'ada@example.com' },
		{ id: 'user-1', name: '', email: 'grace@example.com' }
	];
	const options = buildUserSelectOptions(users);
	const row = buildKbInputRecordMetadataRows(input, options).find(
		(candidate) => candidate.key === 'tenant_id'
	);
	assert.ok(row);

	assert.deepEqual(options, [
		{ value: 'user-2', label: 'Ada Lovelace (ada@example.com)' },
		{ value: 'user-1', label: 'grace@example.com' }
	]);
	assert.equal(row.editable, true);
	assert.equal(row.editor, 'user-select');
	assert.deepEqual(row.options, options);
});

test('maps the selected user and cleared selector to tenant_id payloads', () => {
	const row = {
		label: 'tenant_id',
		key: 'tenant_id',
		value: 'user-2',
		rawValue: 'user-2',
		editable: true,
		editKey: 'field:tenant_id'
	};

	assert.deepEqual(
		buildKbInputUpdatePayloadForMetadataEdit(input, row, ' user-3 ', 'user-select'),
		{ tenant_id: 'user-3' }
	);
	assert.deepEqual(buildKbInputUpdatePayloadForMetadataEdit(input, row, '', 'user-select'), {
		tenant_id: null
	});
});
