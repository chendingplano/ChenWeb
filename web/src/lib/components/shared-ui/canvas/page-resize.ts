import type { JsonObject } from '$lib/wpi/types';

export type PageSide = 'left' | 'right' | 'top' | 'bottom';

export function pageDragSize(initial: number, movement: number, zoom: number, side: PageSide): number {
	const sign = side === 'left' || side === 'top' ? -1 : 1;
	return Math.max(50, Math.round(initial + sign * movement / (zoom || 1)));
}

export function resizePageDimension(page: JsonObject, axis: 'width' | 'height', value: number): JsonObject {
	const next = { ...page, [axis]: `${value}px` };
	for (const limit of ['min', 'max'] as const) {
		const key = `${limit}_${axis}`;
		const match = typeof page[key] === 'string' ? (page[key] as string).match(/^([\d.]+)\s*(?:px)?$/) : null;
		if (match && (limit === 'min' ? value < Number(match[1]) : value > Number(match[1]))) next[key] = `${value}px`;
	}
	return next;
}
