/**
 * @typedef {{ coords?: number[]; line_number?: number }} HighlightLine
 * @typedef {{ width: number; height: number }} HighlightViewport
 */

const HIGHLIGHT_EXPAND_TOP_PX = 10;
const HIGHLIGHT_EXPAND_RIGHT_PX = 20;
const MINERU_COORD_SIZE = 1000;

/**
 * @param {HighlightLine[] | null | undefined} lines
 * @param {HighlightViewport | null | undefined} viewport
 */
export function buildSemanticProjectionHighlightRect(lines, viewport) {
	if (!Array.isArray(lines) || !viewport || typeof viewport.width !== 'number') {
		return null;
	}

	/** @type {Array<{ left: number; top: number; rawBottom: number; width: number }>} */
	const rects = [];

	for (const line of lines) {
		if (!Array.isArray(line?.coords) || line.coords.length < 4) continue;
		const vx1 = line.coords[0] * viewport.width / MINERU_COORD_SIZE;
		const vy1 = line.coords[1] * viewport.height / MINERU_COORD_SIZE;
		const vx2 = line.coords[2] * viewport.width / MINERU_COORD_SIZE;
		const vy2 = line.coords[3] * viewport.height / MINERU_COORD_SIZE;
		rects.push({
			left: Math.min(vx1, vx2),
			top: Math.max(0, Math.min(vy1, vy2) - HIGHLIGHT_EXPAND_TOP_PX),
			rawBottom: Math.max(vy1, vy2),
			width: Math.abs(vx2 - vx1) + HIGHLIGHT_EXPAND_RIGHT_PX
		});
	}

	if (rects.length === 0) return null;

	rects.sort((a, b) => a.top - b.top);

	let left = Infinity;
	let top = Infinity;
	let right = -Infinity;
	let bottom = -Infinity;
	for (let i = 0; i < rects.length; i += 1) {
		const rect = rects[i];
		const nextRect = rects[i + 1];
		const rectBottom = nextRect ? Math.max(rect.rawBottom, nextRect.top) : rect.rawBottom;
		left = Math.min(left, rect.left);
		top = Math.min(top, rect.top);
		right = Math.max(right, rect.left + rect.width);
		bottom = Math.max(bottom, rectBottom);
	}

	const width = right - left;
	const height = bottom - top;
	if (width < 1 || height < 1) return null;

	return {
		left,
		top,
		width,
		height,
		lineCount: rects.length
	};
}
