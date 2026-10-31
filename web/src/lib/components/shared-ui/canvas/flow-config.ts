import type { Json, JsonObject } from '$lib/wpi/types';

export const flowDefaults = {
	column_gap: '0.75rem',
	row_gap: '0.75rem',
	horizontal_alignment: 'start',
	vertical_alignment: 'top',
	rows_alignment: 'top',
	min_row_height: '30px',
	max_row_height: '200px',
	min_elem_width: '20px',
	max_elem_width: '300px',
	max_num_elems: 50,
	row_separator: 'none',
	elem_separator: 'none'
} as const;
export type FlowField = keyof typeof flowDefaults;
export const flowFields = Object.keys(flowDefaults) as FlowField[];
export const flowOptions: Partial<Record<FlowField, readonly string[]>> = {
	horizontal_alignment: ['start', 'center', 'end', 'space-between'],
	vertical_alignment: ['top', 'center', 'bottom', 'stretch'],
	rows_alignment: ['top', 'center', 'bottom', 'space-between'],
	row_separator: [
		'none',
		'thin_solid_line',
		'norm_solid_line',
		'bold_solid_line',
		'thin_dash_line',
		'norm_dash_line',
		'bold_dash_line'
	],
	elem_separator: [
		'none',
		'thin_solid_line',
		'norm_solid_line',
		'bold_solid_line',
		'thin_dash_line',
		'norm_dash_line',
		'bold_dash_line'
	]
};

export function flowValue(key: FlowField, own: JsonObject | undefined, copy: JsonObject): Json {
	return (own ?? copy)[key] ?? flowDefaults[key];
}
export function validFlowValue(key: FlowField, value: string): boolean {
	if (value === '') return true;
	if (key === 'max_num_elems') return /^\d+$/.test(value) && Number.isSafeInteger(Number(value));
	const options = flowOptions[key];
	return options ? options.includes(value) : /^\d+(?:\.\d+)?(?:px|rem)?$/.test(value);
}
export function setFlowValue(config: JsonObject, key: FlowField, value: string): JsonObject {
	const next = { ...config };
	if (value === '') delete next[key];
	else next[key] = key === 'max_num_elems' ? Number(value) : (!flowOptions[key] && /^\d+(?:\.\d+)?$/.test(value) ? `${value}px` : value);
	return next;
}
