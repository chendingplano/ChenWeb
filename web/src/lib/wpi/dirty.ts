// Snapshots for dirty tracking: a canonical JSON string that does not depend
// on key order, so a doc is dirty exactly when its content differs.

function sortKeys(value: unknown): unknown {
	if (Array.isArray(value)) return value.map(sortKeys);
	if (value !== null && typeof value === 'object') {
		const out: Record<string, unknown> = {};
		for (const key of Object.keys(value).sort()) {
			const v = (value as Record<string, unknown>)[key];
			if (v !== undefined) out[key] = sortKeys(v);
		}
		return out;
	}
	return value;
}

export function canonical(value: unknown): string {
	return JSON.stringify(sortKeys(value) ?? null);
}

export function isDirty(doc: unknown, snapshot: string): boolean {
	return canonical(doc) !== snapshot;
}
