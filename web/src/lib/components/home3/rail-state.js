/**
 * @typedef {object} RailState
 * @property {boolean} autoShrinkExpand
 * @property {boolean} railExpanded
 */

/**
 * @param {{ autoShrinkExpand?: boolean, railExpanded?: boolean }} [options]
 * @returns {RailState}
 */
export function createRailState({ autoShrinkExpand = false, railExpanded } = {}) {
	return {
		autoShrinkExpand,
		railExpanded: railExpanded ?? !autoShrinkExpand
	};
}

/**
 * @param {RailState} state
 * @returns {RailState}
 */
export function handleRailButtonAction(state) {
	if (state.autoShrinkExpand) {
		return {
			autoShrinkExpand: false,
			railExpanded: true
		};
	}

	return {
		autoShrinkExpand: false,
		railExpanded: !state.railExpanded
	};
}

/**
 * @param {RailState} state
 * @param {boolean} hovered
 * @returns {RailState}
 */
export function handleRailHoverChange(state, hovered) {
	if (!state.autoShrinkExpand) {
		return state;
	}

	return {
		...state,
		railExpanded: hovered
	};
}
