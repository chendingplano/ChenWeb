import type { MeshEntity, MeshProperty, MeshRecord, MeshValue } from './meshService';

export function valuesOf(entity: MeshEntity, ...predicates: string[]): MeshValue[] {
	return (entity.properties ?? [])
		.filter((p) => predicates.includes(p.predicate))
		.flatMap((p) => p.values);
}

export function buildRecordDetails(record: MeshRecord): MeshProperty[] {
	const entity = record.entity;
	const fields = new Map((entity.properties ?? []).map((p) => [p.predicate, p.values]));
	fields.set('heading', [{ text: entity.label || entity.id }]);
	fields.set('identifier', [{ text: entity.id }]);
	fields.set('rdfIdentifier', [
		{ text: `https://id.nlm.nih.gov/mesh/${encodeURIComponent(entity.id)}` }
	]);
	const preferred = record.concepts.find((c) => c.preferred);
	if (!fields.has('scopeNote') && preferred)
		fields.set('scopeNote', valuesOf(preferred.entity, 'scopeNote'));
	for (const predicate of ['registryNumber', 'relatedRegistryNumber']) {
		const all = [
			...(fields.get(predicate) ?? []),
			...record.concepts.flatMap((c) => valuesOf(c.entity, predicate))
		];
		fields.set(predicate, [...new Map(all.map((v) => [v.text ?? v.id, v])).values()]);
	}
	const entries = new Set(
		record.concepts
			.flatMap((c) => c.terms)
			.flatMap((t) => [t.label, ...valuesOf(t, 'altLabel').map((v) => v.text ?? '')])
			.filter((label) => label && label.toLowerCase() !== entity.label.toLowerCase())
	);
	fields.set(
		'entryTerms',
		[...entries].sort((a, b) => a.localeCompare(b)).map((text) => ({ text }))
	);
	const order = [
		'heading',
		'treeNumber',
		'identifier',
		'rdfIdentifier',
		'annotation',
		'scopeNote',
		'entryTerms',
		'registryNumber',
		'relatedRegistryNumber',
		'previousIndexing',
		'publicMeSHNote',
		'onlineNote',
		'historyNote',
		'dateEstablished',
		'dateRevised',
		'dateCreated'
	];
	const separate = new Set([
		'concept',
		'preferredConcept',
		'term',
		'preferredTerm',
		'allowableQualifier'
	]);
	return [
		...order,
		...[...fields.keys()].filter((key) => !order.includes(key) && !separate.has(key))
	]
		.filter((key) => fields.get(key)?.length)
		.map((predicate) => ({
			predicate,
			values: fields
				.get(predicate)!
				.map((v) =>
					predicate.startsWith('date') && v.text && /^\d{4}-\d{2}-\d{2}$/.test(v.text)
						? { ...v, text: v.text.replaceAll('-', '/') }
						: v
				)
		}));
}
