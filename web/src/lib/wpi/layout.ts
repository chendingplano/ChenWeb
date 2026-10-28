// Size attributes → CSS (design D9). The flow layouter is a wrapping flex
// container: a fixed size is used as given, "elastic" grows into the space
// left in its row within min/max, and "auto" (or nothing) sizes to content.
import type { PageDescriptor, PaneSpec, SizeAttrs } from './types';

function isFixed(size: string | undefined): size is string {
	return !!size && size !== 'auto' && size !== 'elastic';
}

// Safe alignment keeps the start of overflowing content reachable. Explicit
// align-content also prevents flexbox's default stretching of row heights.
export function flowStyle(page: Pick<PageDescriptor, 'height' | 'layouter_config'>): string {
	const c = page.layouter_config ?? {};
	const vertical = { top: 'flex-start', center: 'center', bottom: 'flex-end', stretch: 'stretch' };
	const rows = { top: 'flex-start', center: 'safe center', bottom: 'safe flex-end', 'space-between': 'space-between' };
	const css = [
		`column-gap: ${c.column_gap ?? '0.75rem'}`,
		`row-gap: ${c.row_gap ?? '0.75rem'}`,
		`justify-content: ${c.horizontal_alignment ?? 'start'}`,
		`align-items: ${vertical[c.vertical_alignment ?? 'top']}`,
		`align-content: ${rows[c.rows_alignment ?? 'top']}`
	];
	if (isFixed(page.height)) css.push('height: 100%');
	return css.join('; ');
}

export function flowItemStyle(el: SizeAttrs): string {
	const css: string[] = [];
	if (el.width === 'elastic') {
		css.push(`flex: 1 1 ${el.min_width ?? '0px'}`);
	} else if (isFixed(el.width)) {
		css.push(`flex: 0 0 ${el.width}`, `width: ${el.width}`);
	} else {
		css.push('flex: 0 0 auto');
	}
	if (el.min_width) css.push(`min-width: ${el.min_width}`);
	if (el.max_width) css.push(`max-width: ${el.max_width}`);
	if (isFixed(el.height)) css.push(`height: ${el.height}`);
	return css.join('; ');
}

// Style of a pane along the axis its row or column lays it out on.
export function paneStyle(spec: PaneSpec, axis: 'width' | 'height', size?: number): string {
	const fixed = size !== undefined ? `${size}px` : spec[axis];
	const min = axis === 'width' ? spec.min_width : spec.min_height;
	const max = axis === 'width' ? spec.max_width : spec.max_height;
	const css: string[] = [];
	if (isFixed(fixed)) {
		css.push(`flex: 0 0 ${fixed}`);
	} else if (fixed === 'elastic') {
		css.push('flex: 1 1 0px');
	} else {
		css.push('flex: 0 0 auto');
	}
	if (min) css.push(`min-${axis}: ${min}`);
	if (max) css.push(`max-${axis}: ${max}`);
	css.push(`overflow: ${spec.scroll === 'none' ? 'hidden' : 'auto'}`);
	return css.join('; ');
}

// A pane can be resized by dragging when it is resizable and has a fixed width.
export function isResizable(spec: PaneSpec): boolean {
	return !!spec.resizable && isFixed(spec.width) && spec.width.endsWith('px');
}

export function clampPx(value: number, min?: string, max?: string): number {
	const lo = min?.endsWith('px') ? parseFloat(min) : 0;
	const hi = max?.endsWith('px') ? parseFloat(max) : Infinity;
	return Math.min(Math.max(value, lo), hi);
}

// The page's box inside its pane (devdoc 2026102601 §3.2.1): its sizes as
// given, and auto margins that place it when it is narrower than the pane.
// Without a width it fills the pane, so alignment only shows with a width or
// max_width.
export function pageStyle(page: PageDescriptor): string {
	const css: string[] = [];
	if (isFixed(page.width)) css.push(`width: ${page.width}`);
	if (isFixed(page.height)) css.push(`height: ${page.height}`);
	if (page.min_width) css.push(`min-width: ${page.min_width}`);
	if (page.max_width) css.push(`max-width: ${page.max_width}`);
	if (page.page_alignment === 'center') css.push('margin-left: auto', 'margin-right: auto');
	if (page.page_alignment === 'right') css.push('margin-left: auto');
	return css.join('; ');
}
