// Size attributes → CSS (design D9). The flow layouter is a wrapping flex
// container: a fixed size is used as given, "elastic" grows into the space
// left in its row within min/max, and "auto" (or nothing) sizes to content.
import type { ElementInstance, PageDescriptor, PaneSpec } from './types';

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

export function flowItemStyle(el: Pick<ElementInstance, 'width' | 'height' | 'min_width' | 'max_width'>): string {
	const css: string[] = [];
	const width = typeof el.width === 'number' ? `${el.width}px` : el.width;
	const height = typeof el.height === 'number' ? `${el.height}px` : el.height;
	if (width === 'elastic') {
		css.push(`flex: 1 1 ${el.min_width ?? '0px'}`);
	} else if (isFixed(width)) {
		css.push(`flex: 0 0 ${width}`, `width: ${width}`);
	} else {
		css.push('flex: 0 0 auto');
	}
	if (el.min_width) css.push(`min-width: ${el.min_width}`);
	if (el.max_width) css.push(`max-width: ${el.max_width}`);
	if (isFixed(height)) css.push(`height: ${height}`);
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

// A page length as CSS: "30" and "30 px" are 30px (design: interpreter-page §4).
export function cssLength(value: string | undefined): string | undefined {
	return value?.trim().replace(/^([0-9.]+)\s*$/, '$1px').replace(/\s+/g, '');
}

const justify = { top: 'flex-start', center: 'center', bottom: 'flex-end' };
const alignSelf = { left: 'flex-start', center: 'center', right: 'flex-end' };

// The page's frame in its pane: the pane edges to the page edges are the four
// margins, and the page is placed vertically inside it. Percentages in the
// page box resolve against the frame's content area, which excludes the margins.
export function pageFrameStyle(page: PageDescriptor): string {
	const margin = (v: string | undefined) => cssLength(v) ?? '30px';
	return [
		'display: flex',
		'flex-direction: column',
		'box-sizing: border-box',
		`justify-content: ${justify[page.vert_align ?? 'top']}`,
		`padding: ${margin(page.top_margin)} ${margin(page.right_margin)} ${margin(page.bottom_margin)} ${margin(page.left_margin)}`
	].join('; ');
}

// The page's box: its sizes, horizontal placement and colors. Without a width
// it fills the frame, so placement shows with a width or max_width. A max size
// that the content exceeds makes the box scroll.
export function pageStyle(page: PageDescriptor): string {
	const css = [`flex: 0 0 auto`, `align-self: ${alignSelf[page.hori_align ?? 'center']}`];
	css.push(isFixed(page.width) ? `width: ${cssLength(page.width)}` : 'width: 100%');
	if (isFixed(page.height)) css.push(`height: ${cssLength(page.height)}`);
	css.push(`min-width: ${cssLength(page.min_width) ?? '50px'}`, `min-height: ${cssLength(page.min_height) ?? '50px'}`);
	if (page.max_width) css.push(`max-width: ${cssLength(page.max_width)}`);
	if (page.max_height) css.push(`max-height: ${cssLength(page.max_height)}`);
	if (page.max_width || page.max_height) css.push('overflow: auto');
	if (page.bkgd_color && page.bkgd_color !== 'auto') css.push(`background-color: ${page.bkgd_color}`);
	if (page.frgd_color && page.frgd_color !== 'auto') css.push(`color: ${page.frgd_color}`);
	return css.join('; ');
}
