/**
 * @typedef {typeof DOC_STRUCTURE_DEFAULT_SETTINGS} DocStructureSettings
 * @typedef {{ getItem?: (key: string) => string | null, setItem?: (key: string, value: string) => void }} StorageLike
 */

export const DOC_STRUCTURE_LINE_LIST_MIN_WIDTH = 280;
export const DOC_STRUCTURE_LINE_LIST_MAX_WIDTH = 960;
export const DOC_STRUCTURE_LINE_LIST_DEFAULT_WIDTH = 420;

export const DOC_STRUCTURE_RECORD_MIN_HEIGHT = 32;
export const DOC_STRUCTURE_RECORD_MAX_HEIGHT = 88;
export const DOC_STRUCTURE_RECORD_DEFAULT_HEIGHT = 44;

export const DOC_STRUCTURE_RECORD_GAP_MIN = 2;
export const DOC_STRUCTURE_RECORD_GAP_MAX = 20;
export const DOC_STRUCTURE_RECORD_GAP_DEFAULT = 6;

export const DOC_STRUCTURE_RECORD_DEFAULT_BACKGROUND = '#1C212C';

export const DOC_STRUCTURE_DEFAULT_SETTINGS = {
	lineListWidth: DOC_STRUCTURE_LINE_LIST_DEFAULT_WIDTH,
	recordBackground: DOC_STRUCTURE_RECORD_DEFAULT_BACKGROUND,
	recordHeight: DOC_STRUCTURE_RECORD_DEFAULT_HEIGHT,
	recordGap: DOC_STRUCTURE_RECORD_GAP_DEFAULT
};

const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

/**
 * @param {number} width
 * @returns {number}
 */
export function clampDocStructureLineListWidth(width) {
	const value = Number.isFinite(width) ? Math.round(width) : DOC_STRUCTURE_LINE_LIST_DEFAULT_WIDTH;
	return Math.min(
		DOC_STRUCTURE_LINE_LIST_MAX_WIDTH,
		Math.max(DOC_STRUCTURE_LINE_LIST_MIN_WIDTH, value)
	);
}

/**
 * @param {number} height
 * @returns {number}
 */
export function clampDocStructureRecordHeight(height) {
	const value = Number.isFinite(height) ? Math.round(height) : DOC_STRUCTURE_RECORD_DEFAULT_HEIGHT;
	return Math.min(
		DOC_STRUCTURE_RECORD_MAX_HEIGHT,
		Math.max(DOC_STRUCTURE_RECORD_MIN_HEIGHT, value)
	);
}

/**
 * @param {number} gap
 * @returns {number}
 */
export function clampDocStructureRecordGap(gap) {
	const value = Number.isFinite(gap) ? Math.round(gap) : DOC_STRUCTURE_RECORD_GAP_DEFAULT;
	return Math.min(DOC_STRUCTURE_RECORD_GAP_MAX, Math.max(DOC_STRUCTURE_RECORD_GAP_MIN, value));
}

/**
 * @param {string | null | undefined} userId
 * @returns {string}
 */
export function createDocStructureSettingsStorageKey(userId) {
	return `chenweb:doc-structure:${userId || 'anonymous'}:settings`;
}

/**
 * @param {unknown} value
 * @returns {value is string}
 */
function isHexColor(value) {
	return typeof value === 'string' && HEX_COLOR_RE.test(value);
}

/**
 * @param {Partial<DocStructureSettings> | null | undefined} partial
 * @returns {DocStructureSettings}
 */
export function mergeDocStructureSettings(partial) {
	const merged = { ...DOC_STRUCTURE_DEFAULT_SETTINGS };
	if (!partial || typeof partial !== 'object') return merged;

	if (typeof partial.lineListWidth === 'number') {
		merged.lineListWidth = clampDocStructureLineListWidth(partial.lineListWidth);
	}
	if (isHexColor(partial.recordBackground)) {
		merged.recordBackground = partial.recordBackground;
	}
	if (typeof partial.recordHeight === 'number') {
		merged.recordHeight = clampDocStructureRecordHeight(partial.recordHeight);
	}
	if (typeof partial.recordGap === 'number') {
		merged.recordGap = clampDocStructureRecordGap(partial.recordGap);
	}

	return merged;
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} userId
 * @returns {DocStructureSettings}
 */
export function readDocStructureSettings(storage, userId) {
	try {
		const raw = storage?.getItem?.(createDocStructureSettingsStorageKey(userId));
		if (!raw) return { ...DOC_STRUCTURE_DEFAULT_SETTINGS };
		return mergeDocStructureSettings(JSON.parse(raw));
	} catch {
		return { ...DOC_STRUCTURE_DEFAULT_SETTINGS };
	}
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} userId
 * @param {Partial<DocStructureSettings>} settings
 */
export function writeDocStructureSettings(storage, userId, settings) {
	storage?.setItem?.(
		createDocStructureSettingsStorageKey(userId),
		JSON.stringify(mergeDocStructureSettings(settings))
	);
}
