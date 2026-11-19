type VerticalBounds = { top: number; bottom: number };

function verticalBounds(marks: VerticalBounds[]) {
	return marks.reduce((bounds, mark) => ({
		top: Math.min(bounds.top, mark.top),
		bottom: Math.max(bounds.bottom, mark.bottom)
	}));
}

// Bounds are relative to the top of the PDF pane at its current scroll position.
export function tableHighlightScrollDelta(
	highlights: VerticalBounds[],
	tableHighlights: VerticalBounds[],
	paneHeight: number
) {
	let bounds = verticalBounds(highlights);
	if (bounds.bottom - bounds.top > paneHeight) {
		bounds = verticalBounds(tableHighlights);
		if (bounds.bottom - bounds.top > paneHeight) {
			// Several distant table marks may also exceed the viewport.
			bounds = tableHighlights.reduce((first, mark) => mark.top < first.top ? mark : first);
		}
	}

	if (bounds.top >= 0 && bounds.bottom <= paneHeight) return 0;

	const height = bounds.bottom - bounds.top;
	const margin = Math.min(30, Math.max(0, (paneHeight - height) / 2));
	return Math.min(bounds.top - margin, Math.max(0, bounds.bottom + margin - paneHeight));
}
