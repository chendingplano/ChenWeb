// The page, pane and panel property editors (KnowledgeStore
// code_modules/interpreter-page.md, interpreter-layouter-page.md and
// interpreter-layouter-flow.md, Property editor).
import { m } from '$lib/paraglide/messages.js';
import { flowDefaults, flowOptions, type FlowField } from '$lib/components/shared-ui/canvas/flow-config';
import type { Json } from '../types';
import { allUnits, position } from './shared';
import type { EditorDef, PropertyDef } from './types';

// Colour strings the server accepts on pages (colorPattern, server/api/wpi/validate.go).
const colorPattern = /^(auto|#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})|(rgb|rgba|hsl|hsla)\([0-9.,%\s/-]+\)|[a-zA-Z]{3,30})$/;

function validColor(value: Json, auto: boolean): boolean {
	const ok = (s: Json | undefined) => typeof s === 'string' && colorPattern.test(s) && (auto || s !== 'auto');
	if (typeof value === 'string') return ok(value);
	return !!value && typeof value === 'object' && !Array.isArray(value) && ok(value.light) && ok(value.dark) && value.light !== 'auto' && value.dark !== 'auto';
}

function color(name: string, label: () => string, helpTip: () => string, theme: boolean, auto = false): PropertyDef {
	return { name, label, helpTip, editor: 'color', options: { theme }, validate: (v) => (validColor(v, auto) ? null : m.wpi_prop_invalid_color()) };
}

function alignChoice(value: string): string {
	switch (value) {
		case 'left': return m.wpi_align_left();
		case 'right': return m.wpi_align_right();
		case 'center': return m.wpi_flow_center();
		case 'top': return m.wpi_flow_top();
		case 'bottom': return m.wpi_flow_bottom();
		default: return value;
	}
}

function align(name: 'hori_align' | 'vert_align', helpTip: () => string): PropertyDef {
	return {
		name,
		label: name === 'hori_align' ? m.wpi_prop_hori_align : m.wpi_prop_vert_align,
		helpTip,
		editor: 'value-picker',
		options: { choices: name === 'hori_align' ? ['left', 'center', 'right'] : ['top', 'center', 'bottom'], choiceLabel: alignChoice, emptyLabel: m.wpi_align_default }
	};
}

// A scroll bar is shown when needed ("auto") or hidden ("hide").
function scrollbar(name: 'hori_scrollbar' | 'vert_scrollbar'): PropertyDef {
	return {
		name,
		label: name === 'hori_scrollbar' ? m.wpi_prop_hori_scrollbar : m.wpi_prop_vert_scrollbar,
		helpTip: m.wpi_prop_tip_scrollbar,
		editor: 'switch',
		read: (v) => v !== 'hide',
		write: (on) => (on ? 'auto' : 'hide')
	};
}

const sizeLabels: Record<string, () => string> = {
	width: m.propertiespanel_attr_width,
	height: m.propertiespanel_attr_height,
	min_width: m.wpi_input_attr_min_width,
	max_width: m.wpi_input_attr_max_width,
	min_height: m.wpi_prop_min_height,
	max_height: m.wpi_prop_max_height
};

function size(name: string, keywords: string[], helpTip: () => string): PropertyDef {
	return { name, label: sizeLabels[name], helpTip, editor: 'length', options: { keywords, units: allUnits } };
}

export const page: EditorDef = {
	groups: [
		{
			id: 'basic',
			label: m.wpi_page_group_basic,
			properties: [
				{ name: 'descriptor_id', label: m.wpi_prop_descriptor_id, helpTip: m.wpi_prop_tip_descriptor_id, readOnly: true },
				{ name: 'schema_version', label: m.wpi_prop_schema_version, helpTip: m.wpi_prop_tip_schema_version, readOnly: true },
				{ name: 'page_name', label: m.wpi_prop_page_name, helpTip: m.wpi_prop_tip_page_name, editor: 'localized-text' },
				{ name: 'description', label: m.wpi_prop_description, helpTip: m.wpi_prop_tip_description, editor: 'localized-text', options: { rows: 2 } },
				{ name: 'keywords', label: m.wpi_prop_keywords, helpTip: m.wpi_prop_tip_keywords, editor: 'string-list' },
				{ name: 'page_tags', label: m.wpi_prop_page_tags, helpTip: m.wpi_prop_tip_page_tags, editor: 'string-list' },
				{ name: 'page_image', label: m.wpi_prop_page_image, helpTip: m.wpi_prop_tip_page_image, readOnly: true }
			]
		},
		{
			id: 'geometry',
			label: m.wpi_page_group_geometry,
			properties: [
				{
					name: 'page_layouter',
					label: m.wpi_page_layouter_type,
					helpTip: m.wpi_prop_tip_page_layouter,
					editor: 'value-picker',
					options: (ctx) => ({ choices: ctx.pageLayouters }),
					read: (v) => (v && typeof v === 'object' && !Array.isArray(v) && typeof v.layouter === 'string' ? v.layouter : undefined)
				},
				size('width', ['auto'], m.wpi_prop_tip_page_width),
				size('height', ['auto'], m.wpi_prop_tip_page_height),
				size('min_width', [], m.wpi_prop_tip_min_size),
				size('min_height', [], m.wpi_prop_tip_min_size),
				size('max_width', [], m.wpi_prop_tip_max_size),
				size('max_height', [], m.wpi_prop_tip_max_size),
				align('hori_align', m.wpi_prop_tip_page_align),
				align('vert_align', m.wpi_prop_tip_page_align),
				scrollbar('hori_scrollbar'),
				scrollbar('vert_scrollbar')
			]
		},
		{
			id: 'colors',
			label: m.wpi_page_group_colors,
			properties: [
				color('bkgd_color', m.wpi_prop_bkgd_color, m.wpi_prop_tip_page_color, true, true),
				color('frgd_color', m.wpi_prop_frgd_color, m.wpi_prop_tip_page_color, true, true)
			]
		},
		{
			id: 'actions',
			label: m.wpi_page_group_actions,
			properties: [
				{ name: 'save_action', label: m.wpi_prop_save_action, helpTip: m.wpi_prop_tip_save_action, editor: 'json', options: { rows: 2 } },
				{
					name: 'refresh',
					label: m.wpi_prop_refresh,
					helpTip: m.wpi_prop_tip_refresh,
					editor: 'switch',
					read: (v) => !!v && typeof v === 'object' && !Array.isArray(v) && v.on_show === true,
					write: (on) => (on ? { on_show: true } : undefined)
				}
			]
		},
		{
			id: 'misc',
			label: m.wpi_page_group_misc,
			properties: [
				{ name: 'access_controls', label: m.wpi_prop_access_controls, helpTip: m.wpi_prop_tip_access_controls, editor: 'string-list' },
				{ name: 'page_policy', label: m.wpi_prop_page_policy, helpTip: m.wpi_prop_tip_reserved, editor: 'text' },
				{ name: 'page_style', label: m.wpi_prop_page_style, helpTip: m.wpi_prop_tip_reserved, editor: 'text' },
				{
					name: 'binding_mode',
					label: m.wpi_input_attr_binding_mode,
					helpTip: m.wpi_prop_tip_page_binding_mode,
					editor: 'value-picker',
					options: { choices: ['read-only', 'read-write'], choiceLabel: (v) => (v === 'read-only' ? m.wpi_prop_binding_read_only() : m.wpi_prop_binding_read_write()), emptyLabel: m.wpi_align_default }
				},
				{ name: 'input', label: m.wpi_prop_input, helpTip: m.wpi_prop_tip_input, editor: 'json', options: { rows: 2 } }
			]
		}
	]
};

const paneSize = (name: string, keywords: string[], helpTip: () => string) => size(name, keywords, helpTip);
const margin = (name: string, label: () => string): PropertyDef => ({ name, label, helpTip: m.wpi_prop_tip_margin, editor: 'length', options: { units: ['px', 'rem', 'em'] } });

export const pane: EditorDef = {
	groups: [
		{
			id: 'pane',
			label: m.wpi_prop_group_pane,
			properties: [
				{
					name: 'pane_id',
					label: m.wpi_prop_pane_id,
					helpTip: m.wpi_prop_tip_pane_id,
					editor: 'text',
					options: { commitOnBlur: true },
					validate: (v, ctx) => ctx.paneIdProblem?.(String(v)) ?? null
				},
				position
			]
		},
		{
			id: 'size',
			label: m.wpi_input_group_size,
			properties: [
				paneSize('width', ['auto', 'elastic'], m.wpi_prop_tip_pane_size),
				paneSize('height', ['auto', 'elastic'], m.wpi_prop_tip_pane_size),
				paneSize('min_width', [], m.wpi_prop_tip_min_size),
				paneSize('min_height', [], m.wpi_prop_tip_min_size),
				paneSize('max_width', [], m.wpi_prop_tip_max_size),
				paneSize('max_height', [], m.wpi_prop_tip_max_size),
				{ name: 'resizable', label: m.wpi_prop_resizable, helpTip: m.wpi_prop_tip_resizable, editor: 'switch' }
			]
		},
		{
			id: 'margins',
			label: m.wpi_prop_group_margins,
			visible: (ctx) => !!ctx.leaf,
			properties: [
				margin('left_margin', m.wpi_prop_left_margin),
				margin('right_margin', m.wpi_prop_right_margin),
				margin('top_margin', m.wpi_prop_top_margin),
				margin('bottom_margin', m.wpi_prop_bottom_margin)
			]
		},
		{
			id: 'colors',
			label: m.wpi_prop_group_colors_border,
			properties: [
				color('background_color', m.wpi_prop_background_color, m.wpi_prop_tip_surface_color, true),
				color('foreground_color', m.wpi_prop_foreground_color, m.wpi_prop_tip_surface_color, true),
				{ name: 'border_thickness', label: m.wpi_prop_border_thickness, helpTip: m.wpi_prop_tip_border_thickness, editor: 'length', options: { units: ['px'] } },
				color('border_light_color', m.wpi_prop_border_light_color, m.wpi_prop_tip_border_color, false),
				color('border_dark_color', m.wpi_prop_border_dark_color, m.wpi_prop_tip_border_color, false)
			]
		},
		{
			id: 'content',
			label: m.wpi_prop_group_content_placement,
			visible: (ctx) => !!ctx.leaf,
			properties: [align('hori_align', m.wpi_prop_tip_pane_align), align('vert_align', m.wpi_prop_tip_pane_align), scrollbar('hori_scrollbar'), scrollbar('vert_scrollbar')]
		}
	]
};

function flowLabel(key: FlowField): string {
	const labels: Record<FlowField, () => string> = {
		column_gap: m.wpi_flow_column_gap,
		row_gap: m.wpi_flow_row_gap,
		horizontal_alignment: m.wpi_flow_horizontal_alignment,
		vertical_alignment: m.wpi_flow_vertical_alignment,
		rows_alignment: m.wpi_flow_rows_alignment,
		min_row_height: m.wpi_flow_min_row_height,
		max_row_height: m.wpi_flow_max_row_height,
		min_elem_width: m.wpi_flow_min_elem_width,
		max_elem_width: m.wpi_flow_max_elem_width,
		max_num_elems: m.wpi_flow_max_num_elems,
		row_separator: m.wpi_flow_row_separator,
		elem_separator: m.wpi_flow_elem_separator
	};
	return labels[key]();
}

export function flowOptionLabel(value: string): string {
	const labels: Record<string, () => string> = {
		start: m.wpi_flow_start, center: m.wpi_flow_center, end: m.wpi_flow_end,
		top: m.wpi_flow_top, bottom: m.wpi_flow_bottom, stretch: m.wpi_flow_stretch,
		'space-between': m.wpi_flow_space_between, none: m.wpi_flow_none,
		thin_solid_line: m.wpi_flow_thin_solid, norm_solid_line: m.wpi_flow_normal_solid,
		bold_solid_line: m.wpi_flow_bold_solid, thin_dash_line: m.wpi_flow_thin_dashed,
		norm_dash_line: m.wpi_flow_normal_dashed, bold_dash_line: m.wpi_flow_bold_dashed
	};
	return labels[value]?.() ?? value;
}

function flowProperty(key: FlowField): PropertyDef {
	const choices = flowOptions[key];
	const base = { name: key, label: () => flowLabel(key), helpTip: m.wpi_prop_tip_flow };
	if (choices) return { ...base, editor: 'value-picker', options: { choices: [...choices], choiceLabel: flowOptionLabel } };
	if (key === 'max_num_elems') return { ...base, editor: 'number', options: { min: 0, integer: true }, note: m.wpi_flow_unlimited_hint };
	return { ...base, editor: 'length', options: { units: ['px', 'rem'] } };
}

export const panel: EditorDef = {
	groups: [
		{
			id: 'layouter',
			label: m.wpi_layouter_title,
			properties: [{ name: 'layouter', label: m.wpi_layouter_type, helpTip: m.wpi_prop_tip_panel_layouter, readOnly: true }]
		},
		{
			id: 'colors',
			label: m.wpi_page_group_colors,
			properties: [
				color('background_color', m.wpi_prop_background_color, m.wpi_prop_tip_surface_color, true),
				color('foreground_color', m.wpi_prop_foreground_color, m.wpi_prop_tip_surface_color, true)
			]
		},
		{ id: 'flow', label: m.wpi_prop_group_flow, properties: (Object.keys(flowDefaults) as FlowField[]).map(flowProperty) }
	]
};
