// Groups and choices several property editor definitions share.
import { m } from '$lib/paraglide/messages.js';
import type { Json } from '../types';
import type { GroupDef, PropertyContext, PropertyDef } from './types';

export const allUnits = ['px', 'rem', 'em', '%'];

export function positionText(ctx: PropertyContext): Json | undefined {
	return ctx.position ? m.wpi_prop_position_value({ x: ctx.position.x, y: ctx.position.y }) : undefined;
}

export const position: PropertyDef = {
	name: 'position',
	label: m.wpi_prop_position,
	helpTip: m.wpi_prop_tip_position,
	computed: positionText
};

// Where an element is: its pane and position, both read-only.
export const elementPlacement: GroupDef = {
	id: 'placement',
	label: m.wpi_prop_group_placement,
	properties: [{ name: 'pane', label: m.wpi_element_pane, computed: (ctx) => ctx.pane }, position]
};

// The size every element module accepts (devdoc 2026102601 §3.5).
export function elementSize(widthNote?: () => string): GroupDef {
	const length = { keywords: ['auto', 'elastic'], units: allUnits };
	return {
		id: 'size',
		label: m.wpi_input_group_size,
		properties: [
			{ name: 'width', label: m.propertiespanel_attr_width, editor: 'length', options: length, note: widthNote },
			{ name: 'height', label: m.propertiespanel_attr_height, editor: 'length', options: length },
			{ name: 'min_width', label: m.wpi_input_attr_min_width, editor: 'length', options: length },
			{ name: 'max_width', label: m.wpi_input_attr_max_width, editor: 'length', options: length }
		]
	};
}

export function bindingModeLabel(value: string): string {
	return value === 'read-only' ? m.wpi_prop_binding_read_only() : value === 'read-write' ? m.wpi_prop_binding_read_write() : value;
}

export const fontWeights = ['normal', 'bold', '100', '200', '300', '400', '500', '600', '700', '800', '900'];

// A weight chosen as text is stored as a number when it is one.
export function weightValue(value: Json | undefined): Json | undefined {
	return typeof value === 'string' && /^\d+$/.test(value) ? Number(value) : value;
}

export function weightText(value: Json | undefined): Json | undefined {
	return typeof value === 'number' ? String(value) : value;
}
