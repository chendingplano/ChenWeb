// Text R2 values are JSON data shared with the server's validator.
import values from '../../../../server/api/wpi/text-values.json';
import { textIn } from './entries';
import { parseRef } from './refs';
import type { ElementInstance, Json } from './types';

export const textIcons = values.icons;
export const textDefaults: Record<string, Json> = {
	help_tip: '',
	icon: '',
	label_placement: 'top',
	label_horizontal_gap: '8px',
	label_vertical_gap: '4px',
	label_horizontal_align: 'start',
	label_vertical_align: 'top',
	label_max_width: '160px',
	label_font_weight: 500,
	icon_placement: 'start',
	icon_size: '16px',
	icon_gap: '8px',
	icon_color: 'inherit',
	font_family: 'inherit',
	font_size: '0.875rem',
	font_weight: 'inherit',
	font_style: 'normal',
	line_height: 'normal',
	color: 'inherit',
	background_color: 'transparent',
	text_align: 'start',
	white_space: 'normal',
	padding: '0.25rem 0px',
	width: 'auto',
	height: 'auto',
	min_width: '0px'
};
export const textGroups: [string, string[]][] = [
	['content', ['text', 'data_binding', 'label', 'help_tip']],
	[
		'caption_layout',
		[
			'label_placement',
			'label_horizontal_gap',
			'label_vertical_gap',
			'label_horizontal_align',
			'label_vertical_align',
			'label_max_width',
			'label_font_weight'
		]
	],
	['icon', ['icon', 'icon_placement', 'icon_size', 'icon_gap', 'icon_color']],
	['typography', ['font_family', 'font_size', 'font_weight', 'font_style', 'line_height']],
	['appearance', ['color', 'background_color', 'text_align', 'white_space', 'padding']],
	['size', ['width', 'height', 'min_width', 'max_width']]
];
export const textChoices: Record<string, string[]> = {
	icon: textIcons,
	label_placement: ['top', 'left', 'right', 'bottom'],
	label_horizontal_align: ['start', 'center', 'end'],
	label_vertical_align: ['top', 'center', 'bottom'],
	icon_placement: ['start', 'end'],
	font_style: ['normal', 'italic', 'oblique'],
	text_align: ['start', 'center', 'end', 'justify'],
	white_space: ['normal', 'pre-wrap'],
	font_weight: [
		'inherit',
		'normal',
		'bold',
		'100',
		'200',
		'300',
		'400',
		'500',
		'600',
		'700',
		'800',
		'900'
	],
	label_font_weight: [
		'inherit',
		'normal',
		'bold',
		'100',
		'200',
		'300',
		'400',
		'500',
		'600',
		'700',
		'800',
		'900'
	]
};

export function displayedText(d: ElementInstance, bound: Json | undefined, locale: string): string {
	if (d.data_binding !== undefined) return bound == null ? '' : String(bound);
	return textIn(d.text, locale);
}

// Both authoring and backend enforce these grammars; never apply arbitrary CSS.
export function validTextAttribute(name: string, value: Json): boolean {
	if (name === 'text' || name === 'help_tip' || name === 'label') {
		if (typeof value === 'string') return name !== 'label' || value.length > 0;
		return (
			value !== null &&
			typeof value === 'object' &&
			!Array.isArray(value) &&
			typeof value.en === 'string' &&
			value.en.length > 0 &&
			Object.entries(value).every(
				([k, v]) => /^[a-z]{2,3}(-[a-z0-9]{2,8})*$/.test(k) && typeof v === 'string' && v.length > 0
			)
		);
	}
	if (name === 'data_binding') {
		try {
			return typeof value === 'string' && parseRef(value)?.root === 'input_doc';
		} catch {
			return false;
		}
	}
	if (name === 'font_weight' || name === 'label_font_weight')
		return typeof value === 'number'
			? Number.isInteger(value) && value >= 100 && value <= 900 && value % 100 === 0
			: typeof value === 'string' && ['inherit', 'normal', 'bold'].includes(value);
	if (name === 'line_height')
		return value === 'normal' || (typeof value === 'number' && Number.isFinite(value) && value > 0);
	if (name in textChoices) return typeof value === 'string' && textChoices[name].includes(value);
	if (['color', 'icon_color', 'background_color'].includes(name)) {
		if (typeof value !== 'string') return false;
		if (value === 'inherit' && name !== 'background_color') return true;
		if (
			values.colors.includes(value.toLowerCase()) ||
			/^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i.test(value)
		)
			return true;
		const m =
			/^(rgb|rgba)\(\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*(,\s*([0-9]+(\.[0-9]+)?)\s*)?\)$/.exec(
				value
			);
		return (
			!!m &&
			(m[1] === 'rgba') === (m[6] !== undefined) &&
			m.slice(2, 5).every((v) => Number(v) <= 255) &&
			(m[6] === undefined || Number(m[6]) <= 1)
		);
	}
	if (name === 'font_family') {
		if (value === 'inherit') return true;
		if (typeof value !== 'string' || !value.trim() || /[;(){}<>\\\r\n]/.test(value)) return false;
		const family =
			/^("[^"\\\r\n]+"|'[^'\\\r\n]+'|[\p{L}\p{N}_ -]+)(\s*,\s*("[^"\\\r\n]+"|'[^'\\\r\n]+'|[\p{L}\p{N}_ -]+))*$/u;
		return (
			family.test(value) &&
			value
				.split(',')
				.every(
					(p) =>
						!['inherit', 'initial', 'unset', 'revert', 'revert-layer'].includes(
							p.trim().toLowerCase()
						)
				)
		);
	}
	if (name === 'padding')
		return (
			typeof value === 'string' &&
			value.trim().split(/\s+/).length <= 4 &&
			value
				.trim()
				.split(/\s+/)
				.every((p) => /^[0-9]+(\.[0-9]+)?(px|rem)$/.test(p))
		);
	if (
		[
			'font_size',
			'label_horizontal_gap',
			'label_vertical_gap',
			'label_max_width',
			'icon_size',
			'icon_gap'
		].includes(name)
	) {
		return (
			typeof value === 'string' &&
			(name === 'font_size' ? /^[0-9]+(\.[0-9]+)?(px|rem|em)$/ : /^[0-9]+(\.[0-9]+)?px$/).test(
				value
			) &&
			(!['font_size', 'icon_size', 'label_max_width'].includes(name) || parseFloat(value) > 0)
		);
	}
	if (['width', 'height', 'min_width', 'max_width'].includes(name))
		return (
			typeof value === 'string' && /^(auto|elastic|[0-9]+(\.[0-9]+)?(px|rem|em|%))$/.test(value)
		);
	return true; // Unsupported class attributes remain visible warnings, not new capabilities.
}
