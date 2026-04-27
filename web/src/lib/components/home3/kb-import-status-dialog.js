export function shouldShowOverflowScrollbar(scrollHeight, clientHeight, tolerance = 1) {
	if (!Number.isFinite(scrollHeight) || !Number.isFinite(clientHeight)) {
		return false;
	}

	return scrollHeight - clientHeight > tolerance;
}
