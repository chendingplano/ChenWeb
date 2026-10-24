// Data model for the name-value visualizer: a record shown as a two-column
// list of labelled fields (label right-aligned, values stacked to its right).
// See KnowledgeStore/doc-repo/modules/module-name-value-visualizer.md.

// One value line. Rendering: href → external link, else onClick → in-page link
// button, else plain text.
export type NameValueItem = {
	text: string;
	href?: string;
	onClick?: () => void;
	lang?: string;
	// Render in a monospace face (identifiers, codes).
	mono?: boolean;
};

export type NameValueField = {
	// Stable key, unique within the record; also the {#each} key.
	key: string;
	// Already-localised label shown in the name column.
	label: string;
	values: NameValueItem[];
};

// What callers pass: values may be bare strings or missing.
export type NameValueInput = {
	key: string;
	label: string;
	values: (NameValueItem | string | null | undefined)[];
};

// Wraps bare strings, drops blank values, then drops fields left without values.
export function normalizeFields(fields: NameValueInput[]): NameValueField[] {
	return fields
		.map((field) => ({
			key: field.key,
			label: field.label,
			values: field.values
				.map((v) => (typeof v === 'string' ? { text: v } : v))
				.filter((v): v is NameValueItem => !!v && v.text.trim() !== '')
		}))
		.filter((field) => field.values.length > 0);
}
