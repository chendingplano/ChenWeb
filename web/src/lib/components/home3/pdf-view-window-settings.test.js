import test from 'node:test';
import assert from 'node:assert/strict';

import {
	PDF_VIEW_WINDOW_DEFAULT_SETTINGS,
	PDF_VIEW_WINDOW_ZOOM_DEFAULT,
	clampPdfSidebarWidth,
	clampPdfViewWindowZoom,
	createPdfViewWindowSettingsStorageKey,
	mergePdfViewWindowSettings,
	readPdfViewWindowSettings
} from './pdf-view-window-settings.js';

test('clampPdfSidebarWidth keeps the sidebar width within the allowed range', () => {
	assert.equal(clampPdfSidebarWidth(100, { minWidth: 140, maxWidth: 420, defaultWidth: 270 }), 140);
	assert.equal(clampPdfSidebarWidth(318, { minWidth: 140, maxWidth: 420, defaultWidth: 270 }), 318);
	assert.equal(clampPdfSidebarWidth(999, { minWidth: 140, maxWidth: 420, defaultWidth: 270 }), 420);
	assert.equal(clampPdfSidebarWidth(NaN, { minWidth: 140, maxWidth: 420, defaultWidth: 270 }), 270);
});

test('clampPdfViewWindowZoom keeps zoom within the allowed range', () => {
	assert.equal(clampPdfViewWindowZoom(0), 0.1);
	assert.equal(clampPdfViewWindowZoom(0.7), 0.7);
	assert.equal(clampPdfViewWindowZoom(5), 3);
	assert.equal(clampPdfViewWindowZoom(NaN), PDF_VIEW_WINDOW_ZOOM_DEFAULT);
});

test('mergePdfViewWindowSettings keeps defaults and sanitizes sidebar width', () => {
	assert.deepEqual(
		mergePdfViewWindowSettings(
			{ sidebarWidth: 520, zoom: 9 },
			{ minWidth: 180, maxWidth: 440, defaultWidth: 300 }
		),
		{ sidebarWidth: 440, zoom: 3 }
	);
});

test('createPdfViewWindowSettingsStorageKey scopes settings by instance key', () => {
	assert.equal(
		createPdfViewWindowSettingsStorageKey('metrics-metadata'),
		'chenweb:pdf-view-window:metrics-metadata:settings'
	);
	assert.equal(
		createPdfViewWindowSettingsStorageKey(null),
		'chenweb:pdf-view-window:default:settings'
	);
});

test('readPdfViewWindowSettings returns persisted settings when JSON is valid', () => {
	const storage = {
		/** @param {string} key */
		getItem(key) {
			assert.equal(key, 'chenweb:pdf-view-window:metrics-metadata:settings');
			return JSON.stringify({ sidebarWidth: 388, zoom: 0.85 });
		}
	};

	assert.deepEqual(
		readPdfViewWindowSettings(storage, 'metrics-metadata', {
			minWidth: 180,
			maxWidth: 440,
			defaultWidth: 300
		}),
		{ sidebarWidth: 388, zoom: 0.85 }
	);
});

test('readPdfViewWindowSettings falls back to defaults when JSON is invalid', () => {
	const storage = {
		getItem() {
			return '{oops';
		}
	};
	const options = {
		minWidth: 140,
		maxWidth: 420,
		defaultWidth: 270
	};

	assert.deepEqual(
		readPdfViewWindowSettings(storage, 'metrics-metadata', options),
		PDF_VIEW_WINDOW_DEFAULT_SETTINGS
	);
});
