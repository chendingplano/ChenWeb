/**
 * @typedef {typeof CHUNK_PANEL_DEFAULT_SETTINGS} ChunkPanelSettings
 * @typedef {{ getItem?: (key: string) => string | null, setItem?: (key: string, value: string) => void }} StorageLike
 */

export const CHUNK_LIST_MIN_WIDTH = 320;
export const CHUNK_LIST_MAX_WIDTH = 740;
export const CHUNK_LIST_DEFAULT_WIDTH = 520;
export const ZOOM_MIN = 0.1;
export const ZOOM_MAX = 3;
export const ZOOM_DEFAULT = 0.5;
export const ZOOM_STEP = 0.1;

export const CHUNK_PANEL_DEFAULT_SETTINGS = {
	chunkListWidth: CHUNK_LIST_DEFAULT_WIDTH,
	chunkCardBackground: '#161A22',
	summaryBackground: '#1C212C',
	contentBackground: '#131720',
	pdfZoom: ZOOM_DEFAULT
};

const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

/**
 * @param {number} width
 * @returns {number}
 */
export function clampChunkListWidth(width) {
	const value = Number.isFinite(width) ? Math.round(width) : CHUNK_LIST_DEFAULT_WIDTH;
	return Math.min(CHUNK_LIST_MAX_WIDTH, Math.max(CHUNK_LIST_MIN_WIDTH, value));
}

/**
 * @param {number} zoom
 * @returns {number}
 */
export function clampZoom(zoom) {
	const value = Number.isFinite(zoom) ? Number(zoom.toFixed(2)) : ZOOM_DEFAULT;
	return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, value));
}

/**
 * @param {string | null | undefined} userId
 * @returns {string}
 */
export function createChunkPanelSettingsStorageKey(userId) {
	return `chenweb:chunk-mgmt:${userId || 'anonymous'}:settings`;
}

/**
 * @param {unknown} value
 * @returns {value is string}
 */
function isHexColor(value) {
	return typeof value === 'string' && HEX_COLOR_RE.test(value);
}

/**
 * @param {Partial<ChunkPanelSettings> | null | undefined} partial
 * @returns {ChunkPanelSettings}
 */
export function mergeChunkPanelSettings(partial) {
	const merged = { ...CHUNK_PANEL_DEFAULT_SETTINGS };
	if (!partial || typeof partial !== 'object') return merged;

	if (typeof partial.chunkListWidth === 'number') {
		merged.chunkListWidth = clampChunkListWidth(partial.chunkListWidth);
	}
	if (isHexColor(partial.chunkCardBackground)) {
		merged.chunkCardBackground = partial.chunkCardBackground;
	}
	if (isHexColor(partial.summaryBackground)) {
		merged.summaryBackground = partial.summaryBackground;
	}
	if (isHexColor(partial.contentBackground)) {
		merged.contentBackground = partial.contentBackground;
	}
	if (typeof partial.pdfZoom === 'number') {
		merged.pdfZoom = clampZoom(partial.pdfZoom);
	}

	return merged;
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} userId
 * @returns {ChunkPanelSettings}
 */
export function readChunkPanelSettings(storage, userId) {
	try {
		const raw = storage?.getItem?.(createChunkPanelSettingsStorageKey(userId));
		if (!raw) return { ...CHUNK_PANEL_DEFAULT_SETTINGS };
		return mergeChunkPanelSettings(JSON.parse(raw));
	} catch {
		return { ...CHUNK_PANEL_DEFAULT_SETTINGS };
	}
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} userId
 * @param {Partial<ChunkPanelSettings>} settings
 */
export function writeChunkPanelSettings(storage, userId, settings) {
	storage?.setItem?.(
		createChunkPanelSettingsStorageKey(userId),
		JSON.stringify(mergeChunkPanelSettings(settings))
	);
}
