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

/**
 * Empty means "follow the view's theme": the surface inherits its theme token and
 * flips with light/dark. Only an explicit hex is a user override.
 */
export const CHUNK_THEME_BACKGROUND = '';

/**
 * The former defaults. Each is the dark-mode value of the token the surface was
 * meant to be (panelBg, panelBgAlt, and the content slab), frozen as a constant, so
 * they painted dark under theme-aware ink in light mode. A stored value equal to one
 * of these was never chosen by anyone, so it migrates back to theme-following.
 */
export const CHUNK_LEGACY_BACKGROUNDS = {
	chunkCardBackground: '#161A22',
	summaryBackground: '#1C212C',
	contentBackground: '#131720'
};

export const CHUNK_PANEL_DEFAULT_SETTINGS = {
	chunkListWidth: CHUNK_LIST_DEFAULT_WIDTH,
	chunkCardBackground: CHUNK_THEME_BACKGROUND,
	summaryBackground: CHUNK_THEME_BACKGROUND,
	contentBackground: CHUNK_THEME_BACKGROUND,
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
	for (const key of /** @type {const} */ ([
		'chunkCardBackground',
		'summaryBackground',
		'contentBackground'
	])) {
		const value = partial[key];
		if (value === CHUNK_THEME_BACKGROUND) {
			merged[key] = CHUNK_THEME_BACKGROUND;
		} else if (isHexColor(value)) {
			merged[key] =
				value.toUpperCase() === CHUNK_LEGACY_BACKGROUNDS[key]
					? CHUNK_THEME_BACKGROUND
					: value;
		}
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
