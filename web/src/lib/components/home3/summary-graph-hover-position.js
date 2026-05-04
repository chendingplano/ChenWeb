/**
 * @typedef {{
 *   nodeX: number;
 *   nodeY: number;
 *   nodeRadius?: number;
 *   stageWidth?: number;
 *   stageHeight?: number;
 *   cardWidth?: number;
 *   cardHeight?: number;
 *   nodeWidth?: number;
 *   nodeHeight?: number;
 *   margin?: number;
 *   gap?: number;
 *   gapLeftX?: number;
 *   gapRightX?: number;
 *   gapTopY?: number;
 *   gapBelowY?: number;
 *   debug?: boolean;
 * }} HoverCardPlacementInput
 */

/**
 * @typedef {{
 *   pointX: number;
 *   pointY: number;
 *   nodeX: number;
 *   nodeY: number;
 *   nodeRadius?: number;
 *   cardX: number;
 *   cardY: number;
 *   cardWidth?: number;
 *   cardHeight?: number;
 *   buffer?: number;
 * }} HoverKeepAliveInput
 */

/**
 * @param {HoverCardPlacementInput} input
 */
export function computeHoverCardPosition(input) {
	const {
		nodeX,
		nodeY,
		nodeRadius = 8,
		stageWidth = 1200,
		stageHeight = 720,
		cardWidth = 428,
		cardHeight = 320,
		nodeWidth,
		nodeHeight,
		margin = 16,
		gap = 10,
		gapLeftX = -30,
		gapRightX = 95,
		gapTopY = 0,
		gapBelowY = 70,
		debug = false
	} = input;

	const safeGap = Math.max(0, gap);
	const safeGapLeftX = gapLeftX;
	const safeGapRightX = gapRightX;
	const safeGapTopY = gapTopY;
	const safeGapBelowY = gapBelowY;
	const clampedCardWidth = Math.max(220, Math.min(cardWidth, stageWidth - margin * 2));
	const clampedCardHeight = Math.max(160, Math.min(cardHeight, stageHeight - margin * 2));
	const nodeHalfWidth = Math.max(0, Number(nodeWidth ?? nodeRadius * 2)) / 2;
	const nodeHalfHeight = Math.max(0, Number(nodeHeight ?? nodeRadius * 2)) / 2;
	const rightX = nodeX + nodeHalfWidth + safeGapRightX;
	const leftX = nodeX - nodeHalfWidth - safeGapLeftX - clampedCardWidth;
	const belowY = nodeY + nodeHalfHeight + safeGapBelowY;
	const aboveY = nodeY - nodeHalfHeight - safeGapTopY - clampedCardHeight;
	const centeredX = nodeX - clampedCardWidth / 2;
	const centeredY = nodeY - clampedCardHeight / 2;
	const checks = {
		belowFits: belowY >= margin && belowY + clampedCardHeight <= stageHeight - margin,
		leftFits: leftX >= margin,
		rightFits: rightX + clampedCardWidth <= stageWidth - margin,
		topFits: aboveY >= margin
	};
	/** @param {number} x */
	const clampX = (x) => Math.max(margin, Math.min(x, stageWidth - clampedCardWidth - margin));
	/** @param {number} y */
	const clampY = (y) => Math.max(margin, Math.min(y, stageHeight - clampedCardHeight - margin));
	/**
	 * @param {string} decision
	 * @param {{ x: number; y: number }} pos
	 */
	const finish = (decision, pos) => {
		if (debug) {
			/*
			console.debug('[summary-hover-placement]', {
				decision,
				input: {
					nodeX,
					nodeY,
					nodeRadius,
					nodeWidth,
					nodeHeight,
					stageWidth,
					stageHeight,
					cardWidth,
					cardHeight,
					margin,
					gap,
					gapLeftX,
					gapRightX,
					gapTopY,
					gapBelowY
				},
				derived: {
					safeGap,
					clampedCardWidth,
					clampedCardHeight,
					nodeHalfWidth,
					nodeHalfHeight,
					rightX,
					leftX,
					belowY,
					aboveY,
					centeredX,
					centeredY,
					checks
				},
				pos
			});
			*/
		}
		return pos;
	};

	if (checks.leftFits) {
		return finish('left', { x: leftX, y: clampY(centeredY) });
	}

	if (checks.rightFits) {
		return finish('right', { x: rightX, y: clampY(centeredY) });
	}

	if (checks.topFits) {
		return finish('top', { x: clampX(centeredX), y: aboveY });
	}

	if (checks.belowFits) {
		return finish('below', { x: clampX(centeredX), y: belowY });
	}

	return finish('clamped-center', { x: clampX(centeredX), y: clampY(centeredY) });
}

/**
 * @param {HoverKeepAliveInput} input
 */
export function isPointInHoverKeepAliveZone(input) {
	const {
		pointX,
		pointY,
		nodeX,
		nodeY,
		nodeRadius = 8,
		cardX,
		cardY,
		cardWidth = 428,
		cardHeight = 320,
		buffer = 20
	} = input;

	const minX = Math.min(nodeX - nodeRadius, cardX) - buffer;
	const maxX = Math.max(nodeX + nodeRadius, cardX + cardWidth) + buffer;
	const minY = Math.min(nodeY - nodeRadius, cardY) - buffer;
	const maxY = Math.max(nodeY + nodeRadius, cardY + cardHeight) + buffer;

	return pointX >= minX && pointX <= maxX && pointY >= minY && pointY <= maxY;
}

/**
 * @param {{
 *   globalX: number;
 *   globalY: number;
 *   containerLeft?: number;
 *   containerTop?: number;
 * }} input
 */
export function toContainerLocalPoint(input) {
	const {
		globalX,
		globalY,
		containerLeft = 0,
		containerTop = 0
	} = input;

	return {
		x: globalX - containerLeft,
		y: globalY - containerTop
	};
}
