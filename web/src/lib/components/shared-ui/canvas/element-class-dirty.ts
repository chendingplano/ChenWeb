import { buildClassRow, type DraftAttribute } from './element-class-form';

// Compare the descriptor sent to the API, ignoring JSON object key order.
export function classRowKey(attributes: DraftAttribute[]): string {
	return JSON.stringify(buildClassRow(attributes), (_key, value) =>
		value !== null && typeof value === 'object' && !Array.isArray(value)
			? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)))
			: value
	);
}
