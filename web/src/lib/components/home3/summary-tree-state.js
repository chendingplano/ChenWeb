/**
 * @typedef {{ inputId: number, page: number, summaryId: string }} SummaryPdfTarget
 * @typedef {{ listMode: 'compact' | 'cards', selectedRecordId: number | null, selectedSummaryId: string | null, selectedPdfTarget: SummaryPdfTarget | null }} SummaryTreeState
 */

/**
 * @returns {SummaryTreeState}
 */
export function createSummaryTreeState() {
	return {
		listMode: 'compact',
		selectedRecordId: null,
		selectedSummaryId: null,
		selectedPdfTarget: null
	};
}

/**
 * @param {SummaryTreeState} state
 * @returns {SummaryTreeState}
 */
export function toggleSummaryTreeListMode(state) {
	return {
		...state,
		listMode: state.listMode === 'compact' ? 'cards' : 'compact'
	};
}

/**
 * @param {SummaryTreeState} state
 * @param {number} recordId
 * @returns {SummaryTreeState}
 */
export function selectSummaryTreeRecord(state, recordId) {
	return {
		...state,
		selectedRecordId: recordId,
		selectedSummaryId: null,
		selectedPdfTarget: null
	};
}

/**
 * @param {SummaryTreeState} state
 * @param {{ recordId: number, summaryId: string, inputId: number, page: number }} target
 * @returns {SummaryTreeState}
 */
export function selectRecordSummaryTarget(state, target) {
	return {
		...state,
		selectedRecordId: target.recordId,
		selectedSummaryId: target.summaryId,
		selectedPdfTarget: {
			inputId: target.inputId,
			page: target.page,
			summaryId: target.summaryId
		}
	};
}
