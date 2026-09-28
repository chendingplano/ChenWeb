const hiddenSuccessfulOperations = new Set([
	'doc_processing',
	'blocking',
	'static_analyzer',
	'static_analzyer',
	'extract_metadata',
	'chunked'
]);

/** @param {Array<{ operation?: string; proc_status?: string; 'proc-status'?: string }> | undefined} entries */
export function formatInputStatus(entries) {
	const groups = new Map();
	for (const entry of entries ?? []) {
		const operation = entry?.operation?.trim();
		if (!operation) continue;
		const status = (entry.proc_status ?? entry['proc-status'] ?? '').trim().toLowerCase();
		if (status === 'success' && hiddenSuccessfulOperations.has(operation.toLowerCase())) continue;
		const label = status === 'fail' || status === 'failed'
			? 'Failed'
			: status === 'success' ? 'Success' : status ? status[0].toUpperCase() + status.slice(1) : 'Unknown';
		if (!groups.has(label)) groups.set(label, []);
		groups.get(label).push(operation);
	}

	if (groups.size === 0) return '-';
	if (groups.size === 1 && groups.has('Success')) return groups.get('Success').join(', ');
	return ['Failed', ...[...groups.keys()].filter((label) => label !== 'Failed' && label !== 'Success'), 'Success']
		.filter((label) => groups.has(label))
		.map((label) => `${label}: ${groups.get(label).join(', ')}`)
		.join(', ');
}
