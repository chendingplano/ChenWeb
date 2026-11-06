// Pane trees and their geometry → CSS (devdoc 2026102601 §3.2, §3.3, §3.7;
// OpenSpec wpi-pages-own-panes, design D7). The flow layouter is a wrapping flex
// container: a fixed size is used as given, "elastic" grows into the space left
// in its row within min/max, and "auto" (or nothing) sizes to content. Panes
// are flex items of their split parent; a leaf pane places what it shows inside
// its margins, with its alignment and scroll bars.
import type { ElementInstance, FlowLayouterConfig, PageDescriptor, PaneSpec, SurfaceColors, SurfaceColor, PaneBorder } from './types';

function isFixed(size: string | undefined): size is string {
	return !!size && size !== 'auto' && size !== 'elastic';
}

// A length as CSS: "30" and "30 px" are 30px (design: interpreter-page §4).
export function cssLength(value: string | undefined): string | undefined {
	return value?.trim().replace(/^([0-9.]+)\s*$/, '$1px').replace(/\s+/g, '');
}

// A page's pane tree: its page layouter's own root, else the root of the class
// it copied (devdoc §3.2.1).
export function effectiveRoot(page: Pick<PageDescriptor, 'page_layouter'>): PaneSpec {
	const pl = page.page_layouter;
	return pl?.root ?? pl?.class?.root ?? { pane_id: 'main' };
}

export function isLeaf(pane: PaneSpec): boolean {
	return !pane.split;
}

// The leaf pane IDs of a tree, in reading order.
export function leafPanes(pane: PaneSpec): string[] {
	return isLeaf(pane) ? [pane.pane_id] : (pane.children ?? []).flatMap(leafPanes);
}

export function findPane(pane: PaneSpec, id: string): PaneSpec | undefined {
	if (pane.pane_id === id) return pane;
	for (const child of pane.children ?? []) {
		const found = findPane(child, id);
		if (found) return found;
	}
	return undefined;
}

export function flowMarginStyle(config: FlowLayouterConfig = {}): string {
	return `padding: ${config.top_margin ?? '20px'} ${config.right_margin ?? '20px'} ${config.bottom_margin ?? '20px'} ${config.left_margin ?? '20px'}`;
}

// Safe alignment keeps the start of overflowing content reachable. Explicit
// align-content also prevents flexbox's default stretching of row heights.
export function flowStyle(config: FlowLayouterConfig = {}): string {
	const vertical = { top: 'flex-start', center: 'center', bottom: 'flex-end', stretch: 'stretch' };
	const rows = { top: 'flex-start', center: 'safe center', bottom: 'safe flex-end', 'space-between': 'space-between' };
	return [
		`column-gap: ${config.column_gap ?? '0.75rem'}`,
		`row-gap: ${config.row_gap ?? '0.75rem'}`,
		`justify-content: ${config.horizontal_alignment ?? 'start'}`,
		`align-items: ${vertical[config.vertical_alignment ?? 'top']}`,
		`align-content: ${rows[config.rows_alignment ?? 'top']}`
	].join('; ');
}

export function flowItemStyle(el: Pick<ElementInstance, 'width' | 'height' | 'min_width' | 'max_width'>, config?: FlowLayouterConfig, minimum?: { width: number; height: number }): string {
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
	if (config) {
		const min = config.min_elem_width ?? '20px';
		const max = config.max_elem_width ?? '300px';
		css.push(`min-width: ${el.min_width ? `max(${min}, ${el.min_width})` : min}`);
		css.push(`max-width: ${el.max_width ? `min(${max}, ${el.max_width})` : max}`);
		css.push(`min-height: ${config.min_row_height ?? '30px'}`, `max-height: ${config.max_row_height ?? '200px'}`);
	} else {
		if (el.min_width) css.push(`min-width: ${el.min_width}`);
		if (el.max_width) css.push(`max-width: ${el.max_width}`);
	}
	if (isFixed(height)) css.push(`height: ${height}`);
	if (minimum) {
		css.push(`min-width: max(${config ? (config.min_elem_width ?? '20px') : '0px'}, ${el.min_width ?? '0px'}, ${minimum.width}px)`, `min-height: max(${config ? (config.min_row_height ?? '30px') : '0px'}, ${minimum.height}px)`);
	}
	return css.join('; ');
}

// A panel is as wide as its content; elastic and percentage widths make it as
// wide as its pane's content area (devdoc §3.3). In a pane whose width is auto
// that area is the panel's own width, so they act as auto.
export function panelStyle(elements: Pick<ElementInstance, 'width'>[]): string {
	const stretches = elements.some((e) => e.width === 'elastic' || (typeof e.width === 'string' && e.width.endsWith('%')));
	return stretches ? 'flex: none; width: 100%; min-width: 0' : 'flex: none; max-width: 100%; min-width: 0';
}

// Root dimensions are relative to the page box, not a sibling split axis.
export function rootPaneStyle(spec: PaneSpec): string {
 const length = (value: string | undefined) => value && value !== 'auto' && value !== 'elastic' ? cssLength(value) : '100%';
 const css = ['flex: 0 0 auto', 'box-sizing: border-box', `width: ${length(spec.width)}`, `height: ${length(spec.height)}`, 'min-width: 0', 'min-height: 0'];
 for (const [key, value] of [['min-width', spec.min_width], ['min-height', spec.min_height], ['max-width', spec.max_width], ['max-height', spec.max_height]] as const) {
  if (value) css.push(`${key}: ${cssLength(value)}`);
 }
 return css.join('; ');
}

// A split pane's child along its parent's axis: rows divide the height, columns
// the width. size is a width or height the user dragged to, in pixels.
export function paneStyle(spec: PaneSpec, axis: 'width' | 'height', size?: number): string {
	const along = size !== undefined ? `${size}px` : cssLength(spec[axis]);
	const css: string[] = [];
	if (along === undefined || along === 'elastic') {
		css.push('flex: 1 1 0px');
	} else if (along === 'auto') {
		css.push('flex: 0 0 auto');
	} else {
		css.push(`flex: 0 0 ${along}`);
	}
	for (const [k, v] of [['min-width', spec.min_width ?? '50px'], ['min-height', spec.min_height ?? '50px'], ['max-width', spec.max_width], ['max-height', spec.max_height]] as const) {
		if (v) css.push(`${k}: ${cssLength(v)}`);
	}
	return css.join('; ');
}

// A split pane lays its children out in a row (columns) or a column (rows).
export function splitStyle(spec: PaneSpec): string {
	const m = (value: string | undefined) => cssLength(value) ?? '0px';
	return `display: flex; flex-direction: ${spec.split === 'columns' ? 'row' : 'column'}; box-sizing: border-box; width: 100%; height: 100%; min-width: 0; min-height: 0; padding: ${m(spec.top_margin)} ${m(spec.right_margin)} ${m(spec.bottom_margin)} ${m(spec.left_margin)}; overflow-x: ${overflow(spec.hori_scrollbar)}; overflow-y: ${overflow(spec.vert_scrollbar)}`;
}

const crossAlign = { left: 'safe flex-start', center: 'safe center', right: 'safe flex-end' };
const mainAlign = { top: 'flex-start', center: 'safe center', bottom: 'safe flex-end' };
const overflow = (bar: 'auto' | 'hide' | undefined) => (bar === 'hide' ? 'hidden' : 'auto');

// What a leaf pane, or the page frame in the app's page area, does with what it
// shows: margins as padding, alignment when it is smaller, scroll bars when it
// is bigger. Margins count: content fits only if it and the margins fit.
export function placeStyle(spec: Pick<PaneSpec, 'left_margin' | 'right_margin' | 'top_margin' | 'bottom_margin' | 'hori_align' | 'vert_align' | 'hori_scrollbar' | 'vert_scrollbar'>, defaultAlign: 'left' | 'center' = 'center'): string {
	const m = (v: string | undefined) => cssLength(v) ?? '0px';
	return [
		'display: flex',
		'flex-direction: column',
		'box-sizing: border-box',
		'width: 100%',
		'height: 100%',
		`padding: ${m(spec.top_margin)} ${m(spec.right_margin)} ${m(spec.bottom_margin)} ${m(spec.left_margin)}`,
		`align-items: ${crossAlign[spec.hori_align ?? defaultAlign]}`,
		`justify-content: ${mainAlign[spec.vert_align ?? 'top']}`,
		`overflow-x: ${overflow(spec.hori_scrollbar)}`,
		`overflow-y: ${overflow(spec.vert_scrollbar)}`
	].join('; ');
}

// A page without a height fills its page area when its root pane is split into
// columns, or into rows with an elastic row; otherwise it is as tall as its
// content (devdoc §3.2.3).
export function fillsHeight(page: PageDescriptor): boolean {
	const root = effectiveRoot(page);
	if (root.split === 'columns') return true;
	return root.split === 'rows' && (root.children ?? []).some((c) => !c.height || c.height === 'elastic');
}

// The page's box: its sizes and colors, measured inside its page area. host is
// the pane it was opened in, whose alignment also places the page's content.
export function pageBoxStyle(page: PageDescriptor, host?: PaneSpec): string {
	const css = ['display: flex', 'flex-direction: column', 'box-sizing: border-box', 'flex: none', 'min-width: 0'];
	css.push(isFixed(page.width) ? `width: ${cssLength(page.width)}` : 'width: 100%');
	if (isFixed(page.height)) css.push(`height: ${cssLength(page.height)}`);
	else if (fillsHeight(page)) css.push('height: 100%');
	css.push(`min-width: ${cssLength(page.min_width) ?? '50px'}`, `min-height: ${cssLength(page.min_height) ?? '50px'}`);
	if (page.max_width) css.push(`max-width: ${cssLength(page.max_width)}`);
	if (page.max_height) css.push(`max-height: ${cssLength(page.max_height)}`);
	if (host) css.push(`align-items: ${crossAlign[host.hori_align ?? 'center']}`, `justify-content: ${mainAlign[host.vert_align ?? 'top']}`);
	if (page.bkgd_color && page.bkgd_color !== 'auto') css.push(`background-color: ${surfaceColorValue(page.bkgd_color)}`);
	if (page.frgd_color && page.frgd_color !== 'auto') css.push(`color: ${surfaceColorValue(page.frgd_color)}`);
	return css.join('; ');
}

// A pane can be resized by dragging when it is resizable and has a pixel size
// along its parent's axis.
export function isResizable(spec: PaneSpec, axis: 'width' | 'height'): boolean {
	const size = cssLength(spec[axis]);
	return !!spec.resizable && isFixed(size) && size.endsWith('px');
}

export function clampPx(value: number, min?: string, max?: string): number {
	const lo = min?.endsWith('px') ? parseFloat(min) : 0;
	const hi = max?.endsWith('px') ? parseFloat(max) : Infinity;
	return Math.min(Math.max(value, lo), hi);
}

export function surfaceColorValue(c: SurfaceColor): string {
 return typeof c === 'string' ? c : `light-dark(${c.light}, ${c.dark})`;
}

// Theme pairs follow the active application color scheme; omitted values inherit.
export function surfaceColorStyle(colors: SurfaceColors, suppressBackground = false): string {
 const css: string[] = [];
 if (colors.background_color && !suppressBackground) css.push(`background-color: ${surfaceColorValue(colors.background_color)}`);
 if (colors.foreground_color) css.push(`color: ${surfaceColorValue(colors.foreground_color)}`);
 return css.join('; ');
}

// Each pane inherits each unset color independently from its closest ancestor.
export function inheritPaneColors(pane: SurfaceColors, parent: SurfaceColors = {}): SurfaceColors {
 const background = pane.background_color ?? parent.background_color;
 const foreground = pane.foreground_color ?? parent.foreground_color;
 return { ...(background != null ? { background_color: background } : {}), ...(foreground != null ? { foreground_color: foreground } : {}) };
}

// Each border attribute inherits independently; zero is an explicit override.
export function inheritPaneBorder(pane: PaneBorder, parent: PaneBorder = {}): PaneBorder {
 const border_thickness = pane.border_thickness ?? parent.border_thickness;
 const border_light_color = pane.border_light_color ?? parent.border_light_color;
 const border_dark_color = pane.border_dark_color ?? parent.border_dark_color;
 return { ...(border_thickness != null ? { border_thickness } : {}), ...(border_light_color != null ? { border_light_color } : {}), ...(border_dark_color != null ? { border_dark_color } : {}) };
}

export function paneBorderStyle(border: PaneBorder): string {
 if (!border.border_thickness || parseFloat(border.border_thickness) <= 0) return '';
 return `outline: ${border.border_thickness} solid light-dark(${border.border_light_color ?? '#000'}, ${border.border_dark_color ?? '#fff'}); outline-offset: -${border.border_thickness}`;
}
