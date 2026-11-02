// The input element's property editor (code module `input`; KnowledgeStore
// code_modules/interpreter-input.md, Property editor).
import { m } from '$lib/paraglide/messages.js';
import { textChoiceLabel } from '$lib/components/shared-ui/canvas/text-property-labels';
import { bindingModeLabel, elementPlacement, elementSize, fontWeights, weightText, weightValue } from './shared';
import type { JsonObject } from '../types';
import type { EditorDef, PropertyContext } from './types';

// What InputElement.svelte uses when neither the instance nor its class sets a value.
export const inputDefaults: JsonObject = {
	input_type: 'text',
	label_placement: 'left',
	label_horizontal_gap: '10px',
	label_vertical_gap: '10px',
	label_horizontal_align: 'right',
	label_vertical_align: 'center',
	label_max_width: '160px',
	label_max_lines: 1,
	label_font_size: '14px',
	label_font_weight: 500,
	label_font_style: 'normal'
};

const px = { units: ['px'] };
const horizontal = (ctx: PropertyContext) => ['left', 'right'].includes(String(ctx.get('label_placement') ?? 'left'));
const pick = (choices: string[]) => ({ choices, choiceLabel: textChoiceLabel });

export const input: EditorDef = {
	groups: [
		elementPlacement,
		{
			id: 'general',
			label: m.wpi_input_group_general,
			properties: [
				{ name: 'label', label: m.propertiespanel_attr_label, editor: 'localized-text' },
				{ name: 'data_binding', label: m.wpi_input_attr_data_binding, editor: 'text' },
				{ name: 'binding_mode', label: m.wpi_input_attr_binding_mode, editor: 'value-picker', options: { choices: ['read-only', 'read-write'], choiceLabel: bindingModeLabel } },
				{ name: 'input_type', label: m.wpi_input_attr_input_type, editor: 'value-picker', options: { choices: ['text', 'password', 'email', 'number', 'tel', 'url', 'search'] } }
			]
		},
		{
			id: 'caption_layout',
			label: m.wpi_input_group_caption_layout,
			properties: [
				{ name: 'label_placement', label: m.wpi_input_attr_label_placement, editor: 'value-picker', options: pick(['left', 'top', 'right', 'bottom']) },
				{ name: 'label_horizontal_gap', label: m.wpi_input_attr_label_horizontal_gap, editor: 'length', options: px, visible: horizontal },
				{ name: 'label_vertical_gap', label: m.wpi_input_attr_label_vertical_gap, editor: 'length', options: px, visible: (ctx) => !horizontal(ctx) },
				{ name: 'label_horizontal_align', label: m.wpi_input_attr_label_horizontal_align, editor: 'value-picker', options: pick(['left', 'center', 'right']) },
				{ name: 'label_vertical_align', label: m.wpi_input_attr_label_vertical_align, editor: 'value-picker', options: pick(['top', 'center', 'bottom']), visible: horizontal },
				{ name: 'label_max_width', label: m.wpi_input_attr_label_max_width, editor: 'length', options: px },
				{ name: 'label_max_lines', label: m.wpi_input_attr_label_max_lines, editor: 'number', options: { min: 1, integer: true } }
			]
		},
		{
			id: 'caption_appearance',
			label: m.wpi_input_group_caption_appearance,
			properties: [
				{ name: 'label_font_family', label: m.wpi_input_attr_label_font_family, editor: 'text' },
				{ name: 'label_font_size', label: m.wpi_input_attr_label_font_size, editor: 'length', options: px },
				{ name: 'label_color', label: m.wpi_input_attr_label_color, editor: 'color' },
				{ name: 'label_font_weight', label: m.wpi_input_attr_label_font_weight, editor: 'value-picker', options: pick(fontWeights), read: weightText, write: weightValue },
				{ name: 'label_font_style', label: m.wpi_input_attr_label_font_style, editor: 'value-picker', options: pick(['normal', 'italic', 'oblique']) }
			]
		},
		{
			id: 'input_help',
			label: m.wpi_input_group_input_help,
			properties: [
				{ name: 'placeholder', label: m.wpi_input_attr_placeholder, editor: 'localized-text' },
				{ name: 'help_text', label: m.wpi_input_attr_help_text, editor: 'localized-text' },
				{ name: 'help_tip', label: m.wpi_input_attr_help_tip, editor: 'localized-text' },
				{ name: 'autocomplete', label: m.wpi_input_attr_autocomplete, editor: 'text' }
			]
		},
		{
			id: 'validation',
			label: m.wpi_input_group_validation,
			properties: [
				{ name: 'required', label: m.wpi_input_attr_required, editor: 'switch' },
				{ name: 'max_length', label: m.wpi_input_attr_max_length, editor: 'number', options: { min: 0, integer: true }, visible: (ctx) => ctx.get('input_type') !== 'number' },
				{ name: 'validation', label: m.wpi_input_attr_validation, editor: 'validation', options: (ctx) => ({ numberRules: ctx.get('input_type') === 'number' }) }
			]
		},
		elementSize(m.wpi_element_width_limits_hint)
	]
};
