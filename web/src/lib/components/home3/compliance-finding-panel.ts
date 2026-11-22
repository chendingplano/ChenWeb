// What the Finding panel shows of a compliance activity finding, read from the fields the reviewer
// stores with it (metadata.artifact_fields, version 3 on). Older findings lack these fields and
// show their description as stored.

export type ComplianceActivity = { label: string; kind: string; lineStart: number; lineEnd: number };

export type ComplianceSearchCounts = {
	searched: number; // entries returned, summed over every search of the finding
	selected: number; // candidates sent to the relevance check
	relevant: number; // candidates the relevance check kept
	irrelevant: number; // candidates it removed
};

const ACTIVITY_LINE = /^Activity \S+ \[[^\],]+, lines \d+-\d+\]: /;

function asRecord(value: unknown): Record<string, unknown> | null {
	if (typeof value === 'string') {
		try {
			value = JSON.parse(value);
		} catch {
			return null;
		}
	}
	return value !== null && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

function artifactFields(metadata: unknown): Record<string, unknown> | null {
	return asRecord(asRecord(metadata)?.artifact_fields);
}

export function complianceActivities(metadata: unknown): ComplianceActivity[] {
	const list = artifactFields(metadata)?.activities;
	if (!Array.isArray(list)) return [];
	return list.flatMap((item) => {
		const a = asRecord(item);
		if (!a || typeof a.label !== 'string') return [];
		return [{ label: a.label, kind: String(a.kind ?? ''), lineStart: Number(a.line_start), lineEnd: Number(a.line_end) }];
	});
}

export function complianceSearchCounts(metadata: unknown): ComplianceSearchCounts | null {
	const fields = artifactFields(metadata);
	const traces = fields?.retrieval_trace;
	const relevance = fields?.relevance;
	if (!Array.isArray(traces) || !Array.isArray(relevance)) return null;
	let searched = 0;
	for (const t of traces) {
		const results = asRecord(t)?.results;
		if (Array.isArray(results)) searched += results.length;
	}
	const relevant = relevance.filter((d) => asRecord(d)?.kept === true).length;
	return { searched, selected: relevance.length, relevant, irrelevant: relevance.length - relevant };
}

// The description without its "Activity a1 [text, lines 49-52]: ..." lines, which the panel shows
// in its own Activity row.
export function withoutActivityLines(description: string): string {
	return description
		.split('\n')
		.filter((line) => !ACTIVITY_LINE.test(line))
		.join('\n')
		.trim();
}
