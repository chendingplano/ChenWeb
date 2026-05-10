export function formatTopicLineSpecs(sourceLineSpecs: string[] | null | undefined): string {
	if (!Array.isArray(sourceLineSpecs) || sourceLineSpecs.length === 0) return '—';
	const cleaned = sourceLineSpecs.map((spec) => spec.trim()).filter(Boolean);
	return cleaned.length > 0 ? cleaned.join(', ') : '—';
}
