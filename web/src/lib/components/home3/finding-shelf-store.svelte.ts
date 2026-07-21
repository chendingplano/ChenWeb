import type { FindingItem } from '$lib/services/docReviewService';

// Bridges the Document Review report's currently-selected finding across the
// content/shelf boundary: the report view (rendered inside the content panel)
// publishes here, while the page-level context shelf and the PDF call-out read
// from here. `active` is true only while a Document Review report is mounted, so
// the shelf shows the Finding panel in that context and its normal system-status
// content everywhere else.
type FocusFn = (recordId: number, lineNumbers: number[]) => void;

export const findingShelf = $state<{
	active: boolean;
	finding: FindingItem | null;
	requestId: number | null;
	runId: number | null;
	onFocusMatchedUnit: FocusFn | null;
	onCloseMatchedUnits: (() => void) | null;
}>({
	active: false,
	finding: null,
	requestId: null,
	runId: null,
	onFocusMatchedUnit: null,
	onCloseMatchedUnits: null
});
