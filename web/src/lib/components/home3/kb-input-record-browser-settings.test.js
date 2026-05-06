import test from 'node:test';
import assert from 'node:assert/strict';

import {
	KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE,
	KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS,
	clampKbInputRecordBrowserListWidth,
	clampKbInputRecordBrowserPageSize,
	createKbInputRecordBrowserSettingsStorageKey,
	readKbInputRecordBrowserSettings,
	writeKbInputRecordBrowserSettings
} from './kb-input-record-browser-settings.js';

test('kb input record browser defaults use page size 50', () => {
	assert.equal(KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE, 50);
	assert.equal(KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS.pageSize, 50);
});

test('page size and width are clamped into supported ranges', () => {
	assert.equal(clampKbInputRecordBrowserPageSize(0), 10);
	assert.equal(clampKbInputRecordBrowserPageSize(999), 200);
	assert.equal(clampKbInputRecordBrowserListWidth(100), 280);
	assert.equal(clampKbInputRecordBrowserListWidth(9999), 760);
});

test('storage keys are isolated by instance key', () => {
	assert.notEqual(
		createKbInputRecordBrowserSettingsStorageKey('metrics'),
		createKbInputRecordBrowserSettingsStorageKey('chunks')
	);
});

test('settings persist and reload by instance key', () => {
	const storage = new Map();
	/** @type {{ getItem: (key: string) => string | null, setItem: (key: string, value: string) => void }} */
	const adapter = {
		getItem: (key) => storage.get(key) ?? null,
		setItem: (key, value) => storage.set(key, value)
	};

	writeKbInputRecordBrowserSettings(adapter, 'metrics', {
		pageSize: 75,
		listWidth: 610
	});
	writeKbInputRecordBrowserSettings(adapter, 'chunks', {
		pageSize: 30,
		listWidth: 420
	});

	assert.deepEqual(readKbInputRecordBrowserSettings(adapter, 'metrics'), {
		...KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS,
		pageSize: 75,
		listWidth: 610
	});
	assert.deepEqual(readKbInputRecordBrowserSettings(adapter, 'chunks'), {
		...KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS,
		pageSize: 30,
		listWidth: 420
	});
});
