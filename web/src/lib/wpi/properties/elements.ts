// The property editors of the select, button, list, menu and value-editor
// elements (code modules of the same names; KnowledgeStore
// code_modules/interpreter-*.md, Property editor).
import { m } from '$lib/paraglide/messages.js';
import type { Json } from '../types';
import { bindingModeLabel, elementPlacement, elementSize } from './shared';
import type { EditorDef, PropertyDef } from './types';

export function actionLabel(id: string): string {
	switch (id) {
		case 'save': return m.propertiespanel_trigger_save();
		case 'save-page': return m.propertiespanel_trigger_save_page();
		case 'open-page': return m.propertiespanel_trigger_open_page();
		case 'close-page': return m.propertiespanel_trigger_close_page();
		default: return id.replace(/-/g, ' ');
	}
}

// An action-holding attribute picked by its action class: { "action": id }.
function actionPicker(name: string, label: () => string): PropertyDef {
	return {
		name,
		label,
		editor: 'value-picker',
		options: (ctx) => ({ choices: ctx.triggerActions, choiceLabel: actionLabel, emptyLabel: m.propertiespanel_select_action }),
		read: (v) => (v && typeof v === 'object' && !Array.isArray(v) && typeof v.action === 'string' ? v.action : undefined),
		write: (id) => (typeof id === 'string' && id ? { action: id } : undefined)
	};
}

// The IDs of a menu's items that have no children: the ones a menu can start on.
export function menuLeaves(items: Json | undefined): string[] {
	if (!Array.isArray(items)) return [];
	return items.flatMap((item) => {
		if (!item || typeof item !== 'object' || Array.isArray(item)) return [];
		if (Array.isArray(item.children) && item.children.length) return menuLeaves(item.children);
		return typeof item.item_id === 'string' ? [item.item_id] : [];
	});
}

const binding: PropertyDef[] = [
	{ name: 'data_binding', label: m.wpi_input_attr_data_binding, editor: 'text' },
	{ name: 'binding_mode', label: m.wpi_input_attr_binding_mode, editor: 'value-picker', options: { choices: ['read-only', 'read-write'], choiceLabel: bindingModeLabel } }
];

export const select: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'general',
			label: m.wpi_input_group_general,
			properties: [
				{ name: 'label', label: m.propertiespanel_attr_label, editor: 'localized-text' },
				...binding,
				{ name: 'options', label: m.wpi_prop_attr_options, editor: 'json', options: { rows: 3 } }
			]
		},
		elementSize()
	]
};

export const button: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'general',
			label: m.wpi_input_group_general,
			properties: [{ name: 'label', label: m.propertiespanel_attr_label, editor: 'localized-text' }, actionPicker('on_click', m.propertiespanel_attr_on_click)]
		},
		elementSize(m.wpi_element_width_limits_hint)
	]
};

export const list: EditorDef = {
	groups: [
		elementPlacement,
		{ id: 'data', label: m.wpi_prop_group_data, properties: [{ name: 'source', label: m.wpi_prop_attr_source, editor: 'data-source' }] },
		{
			id: 'columns',
			label: m.wpi_prop_group_columns,
			properties: [
				{ name: 'columns', label: m.wpi_prop_attr_columns, editor: 'column-list' },
				{ name: 'presentation', label: m.wpi_prop_attr_presentation, editor: 'value-picker', options: { choices: ['rows'], choiceLabel: () => m.wpi_prop_presentation_rows() } },
				{ name: 'pagination', label: m.wpi_prop_attr_pagination, editor: 'value-picker', options: { choices: ['yes', 'no'], choiceLabel: (v) => (v === 'yes' ? m.wpi_prop_yes() : m.wpi_prop_no()), emptyLabel: m.wpi_align_default } },
				{ name: 'on_row_click', label: m.wpi_prop_attr_on_row_click, editor: 'json' }
			]
		},
		elementSize()
	]
};

export const menu: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'items',
			label: m.wpi_prop_group_items,
			properties: [
				{ name: 'items', label: m.wpi_prop_attr_items, editor: 'json', options: { rows: 4 } },
				{ name: 'initial_item', label: m.wpi_prop_attr_initial_item, editor: 'value-picker', options: (ctx) => ({ choices: menuLeaves(ctx.get('items')), emptyLabel: m.wpi_chat_choice_first }) }
			]
		},
		elementSize()
	]
};

export const valueEditor: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'general',
			label: m.wpi_input_group_general,
			properties: [{ name: 'label', label: m.propertiespanel_attr_label, editor: 'localized-text' }, ...binding]
		},
		{
			id: 'editor',
			label: m.wpi_value_editor_group_editor,
			properties: [
				{ name: 'editor', label: m.wpi_value_editor_attr_editor, readOnly: true },
				{ name: 'editor_options.displayType', label: m.wpi_length_display_type, editor: 'switch', visible: (ctx) => ctx.get('editor') === 'length' },
				{ name: 'editor_options.defaultValue', label: m.wpi_class_default_value, editor: 'length', options: { keywords: ['auto', 'elastic'], units: ['px', 'rem', 'em', '%'] }, visible: (ctx) => ctx.get('editor') === 'length' },
				{ name: 'editor_options', label: m.wpi_value_editor_attr_options, editor: 'json', options: { rows: 3 } }
			]
		},
		elementSize()
	]
};
