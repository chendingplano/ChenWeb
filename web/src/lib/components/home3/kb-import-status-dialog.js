/**
 * @param {number} scrollHeight
 * @param {number} clientHeight
 * @param {number} [tolerance=1]
 * @returns {boolean}
 */
export function shouldShowOverflowScrollbar(scrollHeight, clientHeight, tolerance = 1) {
	if (!Number.isFinite(scrollHeight) || !Number.isFinite(clientHeight)) {
		return false;
	}

	return scrollHeight - clientHeight > tolerance;
}
