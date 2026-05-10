import { selectRecordTopicTarget } from './topic-tree-state.js';

/**
 * @param {import('./topic-tree-state.js').TopicTreeState} state
 * @param {{ recordId: number, topicId: string, inputId: number, page: number }} target
 * @returns {{ treeState: import('./topic-tree-state.js').TopicTreeState, page: number }}
 */
export function selectTopicTreeViewerTarget(state, target) {
	return {
		treeState: selectRecordTopicTarget(state, target),
		page: Number.isFinite(target.page) && target.page > 0 ? Math.trunc(target.page) : 1
	};
}
