import test from 'node:test';
import assert from 'node:assert/strict';

import {
	CHUNK_PANEL_DEFAULT_SETTINGS,
	clampChunkListWidth,
	clampZoom,
	createChunkPanelSettingsStorageKey,
	mergeChunkPanelSettings,
	readChunkPanelSettings
} from './chunk-mgmt-settings.js';

test('clampChunkListWidth keeps width within allowed range', () => {
	assert.equal(clampChunkListWidth(100), 320);
	assert.equal(clampChunkListWidth(370), 370);
	assert.equal(clampChunkListWidth(999), 740);
});

test('clampZoom keeps zoom within allowed range', () => {
	assert.equal(clampZoom(0), 0.1);
	assert.equal(clampZoom(0.75), 0.75);
	assert.equal(clampZoom(5), 3);
	assert.equal(clampZoom(NaN), 0.5);
});

test('mergeChunkPanelSettings keeps defaults and sanitizes values', () => {
	const merged = mergeChunkPanelSettings({
		chunkListWidth: 1000,
		chunkCardBackground: '#123456',
		summaryBackground: 'invalid',
		contentBackground: '#654321'
	});

	assert.deepEqual(merged, {
		...CHUNK_PANEL_DEFAULT_SETTINGS,
		chunkListWidth: 740,
		chunkCardBackground: '#123456',
		contentBackground: '#654321'
	});
});

test('createChunkPanelSettingsStorageKey scopes settings by user id', () => {
	assert.equal(
		createChunkPanelSettingsStorageKey('user-42'),
		'chenweb:chunk-mgmt:user-42:settings'
	);
	assert.equal(
		createChunkPanelSettingsStorageKey(null),
		'chenweb:chunk-mgmt:anonymous:settings'
	);
});

test('readChunkPanelSettings returns persisted settings when JSON is valid', () => {
	const storage = {
		/** @param {string} key */
		getItem(key) {
			assert.equal(key, 'chenweb:chunk-mgmt:user-42:settings');
			return JSON.stringify({
				chunkListWidth: 512,
				chunkCardBackground: '#111111',
				summaryBackground: '#222222',
				contentBackground: '#333333',
				pdfZoom: 0.75
			});
		}
	};

	assert.deepEqual(readChunkPanelSettings(storage, 'user-42'), {
		chunkListWidth: 512,
		chunkCardBackground: '#111111',
		summaryBackground: '#222222',
		contentBackground: '#333333',
		pdfZoom: 0.75
	});
});

test('readChunkPanelSettings falls back to defaults when JSON is invalid', () => {
	const storage = {
		getItem() {
			return '{oops';
		}
	};

	assert.deepEqual(readChunkPanelSettings(storage, 'user-42'), CHUNK_PANEL_DEFAULT_SETTINGS);
});
