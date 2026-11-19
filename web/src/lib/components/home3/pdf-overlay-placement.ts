export type PdfOverlayRect = { left: number; top: number; right: number; bottom: number };

type Placement = { left: number; top: number; maxHeight: number };

const highlightGap = 12;

export function placePdfOverlay(
	pane: PdfOverlayRect,
	highlights: PdfOverlayRect[],
	width: number,
	height: number
) {
	const anchor = highlights[0];
	const rightmost = pane.right - width;
	if (!anchor || width <= 0 || height <= 0 || rightmost < pane.left || pane.bottom <= pane.top) {
		return null;
	}

	const above = anchor.top - height - highlightGap;
	const preferredTop = above >= pane.top ? above : anchor.bottom + highlightGap;
	const ordered = [...highlights].sort((a, b) => a.top - b.top);
	// A clear region can start at the pane edge or alongside a highlight.
	const leftPositions = new Set([rightmost, pane.left]);
	for (const rect of ordered) {
		leftPositions.add(rect.left - highlightGap - width);
		leftPositions.add(rect.right + highlightGap);
	}

	let fallback: Placement | null = null;
	for (const left of [...leftPositions].sort((a, b) => b - a)) {
		if (left < pane.left || left > rightmost) continue;
		let best: Placement | null = null;
		let gapTop = pane.top;
		const considerGap = (bottom: number) => {
			const space = bottom - gapTop;
			if (space <= 0) return;
			if (!fallback || space > fallback.maxHeight) {
				fallback = { left, top: gapTop, maxHeight: space };
			}
			if (space < height) return;
			const top = Math.max(gapTop, Math.min(preferredTop, bottom - height));
			if (!best || Math.abs(top - preferredTop) < Math.abs(best.top - preferredTop)) {
				best = { left, top, maxHeight: bottom - top };
			}
		};
		for (const rect of ordered) {
			if (rect.left - highlightGap >= left + width || rect.right + highlightGap <= left) continue;
			considerGap(Math.min(pane.bottom, rect.top - highlightGap));
			gapTop = Math.max(gapTop, rect.bottom + highlightGap);
			if (gapTop >= pane.bottom) break;
		}
		considerGap(pane.bottom);
		if (best) return best;
	}
	return fallback;
}
