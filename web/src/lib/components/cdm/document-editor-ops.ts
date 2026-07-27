// Pure helpers for DocumentEditor.svelte's error handling (task group 7).
// Split out from the component so the one non-obvious piece of logic here —
// attributing a validation violation or block-slug conflict to the block
// that caused it — is unit-testable without mounting Svelte or TipTap.

/** One violation/conflict message, attributed to a block id where possible. */
export interface BlockAttribution {
	blockId: string | null;
	message: string;
}

const FIRST_QUOTED = /"([^"]+)"/;

/**
 * Extracts the block id from a server message. Every model.Validate
 * violation (server/api/cdm/model/validate.go) and store.ConflictError
 * message (server/api/cdm/store/store.go) puts the offending block's id in
 * the first quoted substring — e.g. `block "abc" has no type` or
 * `cdm: block id "abc" already exists in this document` — because there is
 * no structured field carrying it separately. A block that fails validation
 * for having no id at all (`block at position 2 in the document has an
 * empty id`) has nothing to quote, so this correctly returns null for that
 * case rather than misattributing it.
 */
export function extractBlockId(message: string): string | null {
	const match = FIRST_QUOTED.exec(message);
	return match ? match[1] : null;
}

export function attributeToBlocks(messages: string[]): BlockAttribution[] {
	return messages.map((message) => ({ blockId: extractBlockId(message), message }));
}
