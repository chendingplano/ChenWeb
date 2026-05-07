/**
 * @typedef {{ sidebarWidth: number, zoom: number }} PdfViewWindowSettings
 * @typedef {{ minWidth: number, maxWidth: number, defaultWidth: number }} PdfSidebarWidthOptions
 * @typedef {{ getItem?: (key: string) => string | null, setItem?: (key: string, value: string) => void }} StorageLike
 */

export const PDF_VIEW_WINDOW_DEFAULT_SETTINGS = {
	sidebarWidth: 270,
	zoom: 0.7
};

export const PDF_VIEW_WINDOW_ZOOM_MIN = 0.1;
export const PDF_VIEW_WINDOW_ZOOM_MAX = 3;
export const PDF_VIEW_WINDOW_ZOOM_DEFAULT = 0.7;

/**
 * @param {string | null | undefined} instanceKey
 * @returns {string}
 */
export function createPdfViewWindowSettingsStorageKey(instanceKey) {
	return `chenweb:pdf-view-window:${instanceKey || 'default'}:settings`;
}

/**
 * @param {number} width
 * @param {PdfSidebarWidthOptions} options
 * @returns {number}
 */
export function clampPdfSidebarWidth(width, options) {
	const minWidth = Number.isFinite(options?.minWidth) ? Math.round(options.minWidth) : 140;
	const maxWidth = Number.isFinite(options?.maxWidth) ? Math.round(options.maxWidth) : 420;
	const normalizedMax = Math.max(minWidth, maxWidth);
	const fallback = Number.isFinite(options?.defaultWidth)
		? Math.round(options.defaultWidth)
		: PDF_VIEW_WINDOW_DEFAULT_SETTINGS.sidebarWidth;
	const value = Number.isFinite(width) ? Math.round(width) : fallback;
	return Math.min(normalizedMax, Math.max(minWidth, value));
}

/**
 * @param {number} zoom
 * @returns {number}
 */
export function clampPdfViewWindowZoom(zoom) {
	const value = Number.isFinite(zoom) ? Number(zoom.toFixed(2)) : PDF_VIEW_WINDOW_ZOOM_DEFAULT;
	return Math.min(PDF_VIEW_WINDOW_ZOOM_MAX, Math.max(PDF_VIEW_WINDOW_ZOOM_MIN, value));
}

/**
 * @param {Partial<PdfViewWindowSettings> | null | undefined} partial
 * @param {PdfSidebarWidthOptions} options
 * @returns {PdfViewWindowSettings}
 */
export function mergePdfViewWindowSettings(partial, options) {
	const merged = {
		sidebarWidth: clampPdfSidebarWidth(options?.defaultWidth, options),
		zoom: PDF_VIEW_WINDOW_ZOOM_DEFAULT
	};
	if (!partial || typeof partial !== 'object') return merged;
	if (typeof partial.sidebarWidth === 'number') {
		merged.sidebarWidth = clampPdfSidebarWidth(partial.sidebarWidth, options);
	}
	if (typeof partial.zoom === 'number') {
		merged.zoom = clampPdfViewWindowZoom(partial.zoom);
	}
	return merged;
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} instanceKey
 * @param {PdfSidebarWidthOptions} options
 * @returns {PdfViewWindowSettings}
 */
export function readPdfViewWindowSettings(storage, instanceKey, options) {
	try {
		const raw = storage?.getItem?.(createPdfViewWindowSettingsStorageKey(instanceKey));
		if (!raw) return mergePdfViewWindowSettings(null, options);
		return mergePdfViewWindowSettings(JSON.parse(raw), options);
	} catch {
		return mergePdfViewWindowSettings(null, options);
	}
}

/**
 * @param {StorageLike | null | undefined} storage
 * @param {string | null | undefined} instanceKey
 * @param {Partial<PdfViewWindowSettings>} settings
 * @param {PdfSidebarWidthOptions} options
 */
export function writePdfViewWindowSettings(storage, instanceKey, settings, options) {
	storage?.setItem?.(
		createPdfViewWindowSettingsStorageKey(instanceKey),
		JSON.stringify(mergePdfViewWindowSettings(settings, options))
	);
}
