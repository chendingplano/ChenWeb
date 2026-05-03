/**
 * @typedef {{ inputId: number, page: number, topicId: string }} TopicPdfTarget
 * @typedef {{ listMode: 'compact' | 'cards', selectedRecordId: number | null, selectedTopicId: string | null, selectedPdfTarget: TopicPdfTarget | null }} TopicTreeState
 */

/**
 * @returns {TopicTreeState}
 */
export function createTopicTreeState() {
	return {
		listMode: 'compact',
		selectedRecordId: null,
		selectedTopicId: null,
		selectedPdfTarget: null
	};
}

/**
 * @param {TopicTreeState} state
 * @returns {TopicTreeState}
 */
export function toggleTopicTreeListMode(state) {
	return {
		...state,
		listMode: state.listMode === 'compact' ? 'cards' : 'compact'
	};
}

/**
 * @param {TopicTreeState} state
 * @param {number} recordId
 * @returns {TopicTreeState}
 */
export function selectTopicTreeRecord(state, recordId) {
	return {
		...state,
		selectedRecordId: recordId,
		selectedTopicId: null,
		selectedPdfTarget: null
	};
}

/**
 * @param {TopicTreeState} state
 * @param {{ recordId: number, topicId: string, inputId: number, page: number }} target
 * @returns {TopicTreeState}
 */
export function selectRecordTopicTarget(state, target) {
	return {
		...state,
		selectedRecordId: target.recordId,
		selectedTopicId: target.topicId,
		selectedPdfTarget: {
			inputId: target.inputId,
			page: target.page,
			topicId: target.topicId
		}
	};
}
