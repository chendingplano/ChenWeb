import test from 'node:test';
import assert from 'node:assert/strict';

import {
	DOC_STRUCTURE_DEFAULT_SETTINGS,
	DOC_STRUCTURE_LEGACY_RECORD_BACKGROUND,
	DOC_STRUCTURE_RECORD_THEME_BACKGROUND,
	clampDocStructureLineListWidth,
	clampDocStructureRecordGap,
	clampDocStructureRecordHeight,
	createDocStructureSettingsStorageKey,
	mergeDocStructureSettings,
	readDocStructureSettings
} from './doc-structure-settings.js';

test('doc structure width clamp keeps the line list width within allowed range', () => {
	assert.equal(clampDocStructureLineListWidth(120), 280);
	assert.equal(clampDocStructureLineListWidth(420), 420);
	assert.equal(clampDocStructureLineListWidth(999), 760);
});

test('doc structure height and gap clamps keep values within allowed range', () => {
	assert.equal(clampDocStructureRecordHeight(10), 32);
	assert.equal(clampDocStructureRecordHeight(52), 52);
	assert.equal(clampDocStructureRecordHeight(200), 88);

	assert.equal(clampDocStructureRecordGap(0), 2);
	assert.equal(clampDocStructureRecordGap(8), 8);
	assert.equal(clampDocStructureRecordGap(99), 20);
});

test('mergeDocStructureSettings keeps defaults and sanitizes values', () => {
	const merged = mergeDocStructureSettings({
		lineListWidth: 800,
		recordBackground: '#123456',
		recordHeight: 20,
		recordGap: 30
	});

	assert.deepEqual(merged, {
		...DOC_STRUCTURE_DEFAULT_SETTINGS,
		lineListWidth: 760,
		recordBackground: '#123456',
		recordHeight: 32,
		recordGap: 20
	});
});

test('record background follows the theme by default so line cards flip with light/dark', () => {
	assert.equal(
		DOC_STRUCTURE_DEFAULT_SETTINGS.recordBackground,
		DOC_STRUCTURE_RECORD_THEME_BACKGROUND
	);
});

test('the legacy dark record background migrates back to theme-following', () => {
	const merged = mergeDocStructureSettings({
		recordBackground: DOC_STRUCTURE_LEGACY_RECORD_BACKGROUND
	});
	assert.equal(merged.recordBackground, DOC_STRUCTURE_RECORD_THEME_BACKGROUND);

	// Case-insensitively: the picker persists lowercase hex.
	const lowercased = mergeDocStructureSettings({
		recordBackground: DOC_STRUCTURE_LEGACY_RECORD_BACKGROUND.toLowerCase()
	});
	assert.equal(lowercased.recordBackground, DOC_STRUCTURE_RECORD_THEME_BACKGROUND);
});

test('an explicit record background override is kept, and can be cleared back to the theme', () => {
	assert.equal(mergeDocStructureSettings({ recordBackground: '#123456' }).recordBackground, '#123456');
	assert.equal(
		mergeDocStructureSettings({ recordBackground: DOC_STRUCTURE_RECORD_THEME_BACKGROUND })
			.recordBackground,
		DOC_STRUCTURE_RECORD_THEME_BACKGROUND
	);
});

test('createDocStructureSettingsStorageKey scopes settings by user id', () => {
	assert.equal(
		createDocStructureSettingsStorageKey('user-42'),
		'chenweb:doc-structure:user-42:settings'
	);
	assert.equal(
		createDocStructureSettingsStorageKey(null),
		'chenweb:doc-structure:anonymous:settings'
	);
});

test('readDocStructureSettings returns persisted settings when JSON is valid', () => {
	const storage = {
		/** @param {string} key */
		getItem(key) {
			assert.equal(key, 'chenweb:doc-structure:user-42:settings');
			return JSON.stringify({
				lineListWidth: 512,
				recordBackground: '#111111',
				recordHeight: 56,
				recordGap: 10
			});
		}
	};

	assert.deepEqual(readDocStructureSettings(storage, 'user-42'), {
		lineListWidth: 512,
		recordBackground: '#111111',
		recordHeight: 56,
		recordGap: 10
	});
});

test('readDocStructureSettings falls back to defaults when JSON is invalid', () => {
	const storage = {
		getItem() {
			return '{oops';
		}
	};

	assert.deepEqual(readDocStructureSettings(storage, 'user-42'), DOC_STRUCTURE_DEFAULT_SETTINGS);
});
