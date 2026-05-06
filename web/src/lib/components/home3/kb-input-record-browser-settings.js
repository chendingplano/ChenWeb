/**
 * @typedef {typeof KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS} KbInputRecordBrowserSettings
 * @typedef {{ getItem?: (key: string) => string | null, setItem?: (key: string, value: string) => void }} StorageLike
 */

export const KB_INPUT_RECORD_BROWSER_MIN_LIST_WIDTH = 280;
export const KB_INPUT_RECORD_BROWSER_MAX_LIST_WIDTH = 760;
export const KB_INPUT_RECORD_BROWSER_DEFAULT_LIST_WIDTH = 420;

export const KB_INPUT_RECORD_BROWSER_MIN_PAGE_SIZE = 10;
export const KB_INPUT_RECORD_BROWSER_MAX_PAGE_SIZE = 200;
export const KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE = 50;

export const KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS = {
	pageSize: KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE,
	listWidth: KB_INPUT_RECORD_BROWSER_DEFAULT_LIST_WIDTH
};

/**
 * @param {number} width
 * @returns {number}
 */
export function clampKbInputRecordBrowserListWidth(width) {
	const value = Number.isFinite(width) ? Math.round(width) : KB_INPUT_RECORD_BROWSER_DEFAULT_LIST_WIDTH;
	return Math.min(
		KB_INPUT_RECORD_BROWSER_MAX_LIST_WIDTH,
		Math.max(KB_INPUT_RECORD_BROWSER_MIN_LIST_WIDTH, value)
	);
}

/**
 * @param {number} pageSize
 * @returns {number}
 */
export function clampKbInputRecordBrowserPageSize(pageSize) {
	const value = Number.isFinite(pageSize) ? Math.round(pageSize) : KB_INPUT_RECORD_BROWSER_DEFAULT_PAGE_SIZE;
	return Math.min(
		KB_INPUT_RECORD_BROWSER_MAX_PAGE_SIZE,
		Math.max(KB_INPUT_RECORD_BROWSER_MIN_PAGE_SIZE, value)
	);
}

/**
 * @param {string | null | undefined} instanceKey
 * @returns {string}
 */
export function createKbInputRecordBrowserSettingsStorageKey(instanceKey) {
	return `chenweb:kb-input-record-browser:${instanceKey || 'default'}:settings`;
}

/**
 * @param {Partial<KbInputRecordBrowserSettings> | null | undefined} partial
 * @returns {KbInputRecordBrowserSettings}
 */
export function mergeKbInputRecordBrowserSettings(partial) {
	const merged = { ...KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS };
	if (!partial || typeof partial !== 'object') return merged;

	if (typeof partial.pageSize === 'number') {
		merged.pageSize = clampKbInputRecordBrowserPageSize(partial.pageSize);
	}
	if (typeof partial.listWidth === 'number') {
		merged.listWidth = clampKbInputRecordBrowserListWidth(partial.listWidth);
	}

	return merged;
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} instanceKey
 * @returns {KbInputRecordBrowserSettings}
 */
export function readKbInputRecordBrowserSettings(storage, instanceKey) {
	try {
		const raw = storage?.getItem?.(createKbInputRecordBrowserSettingsStorageKey(instanceKey));
		if (!raw) return { ...KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS };
		return mergeKbInputRecordBrowserSettings(JSON.parse(raw));
	} catch {
		return { ...KB_INPUT_RECORD_BROWSER_DEFAULT_SETTINGS };
	}
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} instanceKey
 * @param {Partial<KbInputRecordBrowserSettings>} settings
 */
export function writeKbInputRecordBrowserSettings(storage, instanceKey, settings) {
	storage?.setItem?.(
		createKbInputRecordBrowserSettingsStorageKey(instanceKey),
		JSON.stringify(mergeKbInputRecordBrowserSettings(settings))
	);
}
