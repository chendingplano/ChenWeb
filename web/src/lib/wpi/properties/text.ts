// The text element's property editor (code module `text`; KnowledgeStore
// code_modules/interpreter-text.md, Property editor).
import { m } from '$lib/paraglide/messages.js';
import { localized } from '../text';
import { textChoices, textGroups, validTextAttribute } from '../text-attributes';
import type { Json, Text } from '../types';
import { textAttributeLabel, textChoiceLabel, textGroupHelp, textGroupLabel } from '$lib/components/shared-ui/canvas/text-property-labels';
import { elementPlacement, weightText, weightValue } from './shared';
import type { EditorDef, PropertyContext, PropertyDef, ValueEditorId } from './types';
import type { ValueEditorOptions } from '$lib/components/shared-ui/value-editors/types';

const sizeNames = ['width', 'height', 'min_width', 'max_width'];
const length = /^(\d+(?:\.\d+)?)(px|rem|em|%)$/;

// Whether a width or height is above its maximum in the same unit.
function exceedsMaximum(name: string, value: Json, ctx: PropertyContext): boolean {
	if (name !== 'width' && name !== 'height') return false;
	const max = ctx.get(`max_${name}`);
	if (typeof value !== 'string' || typeof max !== 'string') return false;
	const a = length.exec(value);
	const b = length.exec(max);
	return !!a && !!b && a[2] === b[2] && Number(a[1]) > Number(b[1]);
}

// The panel's limit a width or height is above, if any (px and rem compared).
function flowLimit(name: string, value: Json, ctx: PropertyContext): string | undefined {
	if (!ctx.flowConfig || (name !== 'width' && name !== 'height') || typeof value !== 'string') return;
	const limit = name === 'width' ? (ctx.flowConfig.max_elem_width ?? '300px') : (ctx.flowConfig.max_row_height ?? '200px');
	const actual = length.exec(value);
	const maximum = length.exec(limit);
	if (!actual || !maximum) return;
	if (actual[2] === maximum[2]) return Number(actual[1]) > Number(maximum[1]) ? limit : undefined;
	if (typeof window === 'undefined' || !['px', 'rem'].includes(actual[2]) || !['px', 'rem'].includes(maximum[2])) return;
	const root = parseFloat(getComputedStyle(document.documentElement).fontSize);
	const px = (x: RegExpExecArray) => Number(x[1]) * (x[2] === 'rem' ? root : 1);
	return px(actual) > px(maximum) ? limit : undefined;
}

function editorOf(name: string): { editor: ValueEditorId; options?: ValueEditorOptions } {
	if (name === 'text') return { editor: 'localized-text', options: { rows: 2, emptyAction: m.wpi_text_empty } };
	if (name === 'help_tip') return { editor: 'localized-text', options: { rows: 2, emptyAction: m.wpi_text_empty_help } };
	if (name === 'label') return { editor: 'localized-text' };
	if (name === 'icon')
		return { editor: 'value-picker', options: { choices: [NO_ICON, ...textChoices.icon.filter((c) => c !== '')], choiceLabel: (c) => textChoiceLabel(c === NO_ICON ? '' : c) } };
	if (name in textChoices) return { editor: 'value-picker', options: { choices: textChoices[name], choiceLabel: textChoiceLabel } };
	if (['color', 'icon_color', 'background_color'].includes(name)) return { editor: 'color' };
	if (['label_horizontal_gap', 'label_vertical_gap', 'label_max_width', 'icon_size', 'icon_gap'].includes(name))
		return { editor: 'length', options: { units: ['px'] } };
	if (name === 'font_size') return { editor: 'length', options: { units: ['px', 'rem', 'em'] } };
	if (sizeNames.includes(name)) return { editor: 'length', options: { keywords: ['auto', 'elastic'], units: ['px', 'rem', 'em', '%'] } };
	if (name === 'line_height') return { editor: 'json' };
	return { editor: 'text' };
}

function visible(name: string): ((ctx: PropertyContext) => boolean) | undefined {
	const horizontal = (ctx: PropertyContext) => ['left', 'right'].includes(String(ctx.get('label_placement') ?? 'top'));
	if (name === 'label_horizontal_gap' || name === 'label_vertical_align') return (ctx) => !!localized(ctx.get('label') as Text | undefined) && horizontal(ctx);
	if (name.startsWith('label_')) return (ctx) => !!localized(ctx.get('label') as Text | undefined);
	if (name.startsWith('icon_')) return (ctx) => !!ctx.get('icon');
	return undefined;
}

// No icon is stored as "" (an explicit value, so it can override a class's icon);
// the pulldown names it with a choice of its own.
const NO_ICON = 'none';

function property(name: string): PropertyDef {
	const weight = name === 'font_weight' || name === 'label_font_weight';
	return {
		name,
		label: () => textAttributeLabel(name),
		...editorOf(name),
		...(weight ? { read: weightText, write: weightValue } : {}),
		...(name === 'icon' ? { read: (v: Json | undefined) => (v === '' ? NO_ICON : v), write: (v: Json | undefined) => (v === NO_ICON ? '' : v) } : {}),
		visible: visible(name),
		validate: (value) => (validTextAttribute(name, value) ? null : sizeNames.includes(name) ? m.wpi_text_invalid_size() : m.wpi_text_invalid()),
		warn: (value, ctx) => {
			if (name === 'data_binding' && ctx.get('text') != null) return m.wpi_text_source_conflict();
			const limit = flowLimit(name, value, ctx);
			if (limit) return name === 'width' ? m.wpi_text_flow_width_limit({ limit }) : m.wpi_text_flow_height_limit({ limit });
			return exceedsMaximum(name, value, ctx) ? m.wpi_text_exceeds_maximum() : null;
		}
	};
}

export const text: EditorDef = {
	groups: [
		elementPlacement,
		...textGroups.map(([id, names]) => ({
			id,
			label: () => textGroupLabel(id),
			help: () => textGroupHelp(id),
			properties: names.map(property)
		}))
	]
};
