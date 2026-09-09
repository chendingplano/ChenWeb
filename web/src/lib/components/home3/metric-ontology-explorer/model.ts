// Static ontology model for the Metric Ontology Explorer canvas.
//
// The diagram is fixed: a `Metric` centre, nine satellites, and seven
// unfoldable downstream chains. Nothing here is fetched — the record tabs read
// their slice of GET /api/v1/kb/metrics/:metric_id/graph (keyed by chain-node
// `id`) and the source-document pane loads the metric's input.
//
// Chains (exact, per spec metric-ontology-explorer + analysis-node-related-metrics):
//   object  -> Object Mention -> Object Node
//   keyword -> Keyword Concept
//   mdef    -> Ontology Term -> Class Contract Revision
//   processor -> Extract Metrics -> Normalize Assertion -> Associate Semantics -> Project Semantics
//   evidence  -> Assertion Evidence -> Decision Candidate -> Semantic Assertion
//   analysis  -> Metrics of Same Class -> Metrics of Similar Classes  (lazy: GET /kb/metrics/:id/related-metrics)
//   document / product / misc: no chain

export type Glyph =
	| 'metric'
	| 'document'
	| 'object'
	| 'keyword'
	| 'evidence'
	| 'mdef'
	| 'processor'
	| 'product'
	| 'analysis'
	| 'misc'
	| 'entity'
	| 'stage';

export type EntryCopy = {
	definition: string;
	connection: string;
	processing: string;
	sources: string[]; // short quotes; wired to PDF highlighting in a later change
};

export type SatelliteNode = {
	id: string;
	label: string;
	kind: string;
	glyph: Glyph;
	edge: string; // "Metric <edge> <label>", e.g. "found in"
	entry: EntryCopy;
};

export type ChainNode = {
	id: string;
	label: string;
	glyph: 'entity' | 'stage';
	table: string; // kb.* table name, or a short pipeline-stage note
	columns: string[]; // display columns; order matches PROJECT[id] output
	description: string;
	/** Evidence span ids to highlight in the source pane when this tab is active. */
	evidenceSpans?: string[];
	/**
	 * Marks a lazily-fetched node: its rows come from
	 * GET /api/v1/kb/metrics/:metric_id/related-metrics?scope=<related>, not the
	 * one-shot metric_graph payload. content-viewer fetches these on tab open
	 * and caches per (metric_id, scope).
	 */
	related?: 'same_class' | 'similar_class';
};

export const CENTER: SatelliteNode = {
	id: 'metric',
	label: 'Metric',
	kind: 'ontology core',
	glyph: 'metric',
	edge: 'is',
	entry: {
		definition:
			'A metric is the smallest unit of quantified meaning the system keeps. Extraction reads a clause, recognises that it asserts something measurable, and records the value together with the words that carry it. From that point the metric is an object in its own right — it can be enriched, compared, and audited without returning to the page.',
		connection:
			'Every other node on this canvas exists because a metric needs it: a document is where it was found, an object is what it describes, a definition is the mould it must fit, evidence is the proof it is real, a processor is the hand that made it, and analysis and products are what becomes of it afterwards.',
		processing:
			'Metrics move through a fixed sequence — extract, enrich, then a lossless semantic write in Phase D. Each step is cached against a shared prefix and batched per document chunk, so a re-run touches only what changed.',
		sources: []
	}
};

export const SATELLITES: SatelliteNode[] = [
	{
		id: 'document',
		label: 'Document',
		kind: 'source',
		glyph: 'document',
		edge: 'found in',
		entry: {
			definition:
				'The page a metric came from — a source PDF and the line-based extract taken from it. Documents enter as PDFs, are parsed with a layout-aware reader, and are cut into chunks; each chunk becomes an input record with a stable id.',
			connection:
				'A metric without a document is a claim without a citation. The link is one-directional and permanent: the metric points back to a chunk offset, never the reverse.',
			processing:
				'Ingest → layout parse → chunk → write kb.inputs. Re-ingestion is idempotent; unchanged chunks keep their ids.',
			sources: []
		}
	},
	{
		id: 'object',
		label: 'Object',
		kind: 'entity',
		glyph: 'object',
		edge: 'measures',
		entry: {
			definition:
				'The real thing a metric is about — a wall assembly, a fenestration unit, a process step. Objects are read from the surrounding text as mentions, then resolved to canonical nodes.',
			connection:
				'The metric measures the object. When an object has fewer metrics than its class expects, that gap is what object-anchored diagnostics report.',
			processing:
				'Mention extraction → canonicalisation → kb.artifact_objects, kb.object_nodes. Objects anchor the missing-metric detector.',
			sources: []
		}
	},
	{
		id: 'keyword',
		label: 'Keyword',
		kind: 'lexicon',
		glyph: 'keyword',
		edge: 'named by',
		entry: {
			definition:
				'The vocabulary that lets the system recognise a metric by name — a curated lexicon of verified terms, each resolving to a governed concept tied to QUDT.',
			connection:
				'Keywords do not define a metric; they surface it. The alignment they produce is a stub — a hint that a governed concept applies — pending confirmation.',
			processing:
				'Lexicon match → keyword concept → aligns_to_term stub. The lexicon is versioned; QUDT links are complete.',
			sources: []
		}
	},
	{
		id: 'evidence',
		label: 'Evidence',
		kind: 'provenance',
		glyph: 'evidence',
		edge: 'proven by',
		entry: {
			definition:
				'The exact words on the page that make a metric’s value defensible. Evidence rows link a semantic assertion to a character span and bounding box inside a chunk.',
			connection:
				'Evidence is the metric’s alibi. One metric may rest on several spans; a span with no assertion is just text.',
			processing:
				'assertion_evidence → decision candidate → semantic assertion. Rendered in the source pane as highlights.',
			sources: []
		}
	},
	{
		id: 'mdef',
		label: 'Metric Definition',
		kind: 'governed',
		glyph: 'mdef',
		edge: 'shaped by',
		entry: {
			definition:
				'The governed template a metric instance has to fit: its unit, its value range, its comparison direction. A definition binds to governed ontology terms and carries a class contract synthesised from many instances; each contract change is an immutable revision.',
			connection:
				'The metric conforms to the definition. Where the two disagree, the metric is flagged rather than silently coerced.',
			processing:
				'extract_metric_definitions → ontology term binding → class-contract revision → per-instance conformance.',
			sources: []
		}
	},
	{
		id: 'processor',
		label: 'Processor',
		kind: 'pipeline',
		glyph: 'processor',
		edge: 'made by',
		entry: {
			definition:
				'The pipeline stages that bring a metric into being and enrich it: extract metrics from chunks, normalise raw candidates onto governed assertion kinds, associate them with objects, keywords and evidence, then project the result into the semantic tables.',
			connection:
				'The processor is the hand; the metric is the mark it leaves. Nothing else on this canvas runs code against the document.',
			processing:
				'Extract Metrics → Normalize Assertion → Associate Semantics → Project Semantics. DeepSeek prefix cache, per-chunk batch coordinator.',
			sources: []
		}
	},
	{
		id: 'product',
		label: 'Product',
		kind: 'downstream',
		glyph: 'product',
		edge: 'read by',
		entry: {
			definition:
				'What a curated metric is eventually for — comparisons, dashboards, wiki entries. Products read curated metrics and their contracts.',
			connection:
				'The product consumes the metric. It never writes back — a correction goes through the processor, not the product.',
			processing: 'Read curated metrics + contracts. can_compare gate — planned.',
			sources: []
		}
	},
	{
		id: 'analysis',
		label: 'Analysis',
		kind: 'diagnostic',
		glyph: 'analysis',
		edge: 'judged by',
		entry: {
			definition:
				'The diagnostics that keep the metric population honest — coverage by object class, unmapped definitions, and self-harvested ontology candidates. Findings are catalogued P1 through P8.',
			connection:
				'Analysis evaluates the metric in aggregate. A single metric is never wrong here; a pattern across many is.',
			processing:
				'Coverage + mapping scan → findings. Candidate harvest is additive-only; fingerprints never delete on re-run.',
			sources: []
		}
	},
	{
		id: 'misc',
		label: 'Misc',
		kind: 'deferred',
		glyph: 'misc',
		edge: 'parked in',
		entry: {
			definition:
				'The holding pen — unparsed assertion kinds, ambiguous reconciliations, things not yet placed. Rather than force a classification, the system parks an artefact with a marker and waits for a person.',
			connection:
				'A metric lands here when its assertion kind has no governed term, or when an object reconciliation is ambiguous. It is still a metric; it just has no seat yet.',
			processing:
				'Marker no_governed_assertion_kind_term:unparsed. Object reconciliation ambiguous — WARN. Manual drain only.',
			sources: []
		}
	}
];

export const CHAINS: Record<string, ChainNode[]> = {
	object: [
		{
			id: 'object__mention',
			label: 'Object Mention',
			glyph: 'entity',
			table: 'kb.artifact_objects',
			columns: ['id', 'object', 'object id', 'reconcile'],
			description:
				'The metric’s own object mention(s) — the text this metric was read as being about, before canonicalisation.'
		},
		{
			id: 'object__node',
			label: 'Object Node',
			glyph: 'entity',
			table: 'kb.object_nodes',
			columns: ['id', 'canonical name', 'type', 'object id', 'reconcile'],
			description:
				'The canonical object the metric’s mention resolves to; carries the class and the alias set.'
		}
	],
	keyword: [
		{
			id: 'keyword__concept',
			label: 'Keyword Concept',
			glyph: 'entity',
			table: 'kb.keyword_concepts',
			columns: ['concept id', 'label', 'status', 'scope', 'gloss'],
			description:
				'The governed concept this metric aligns to; one concept groups many synonyms and links to QUDT.'
		}
	],
	mdef: [
		{
			id: 'mdef__term',
			label: 'Ontology Term',
			glyph: 'entity',
			table: 'kb.ontology_terms',
			columns: ['term id', 'kind', 'module', 'status', 'definition'],
			description:
				'The governed term(s) this metric binds to — its definition term plus the unit / quantity-kind / assertion-kind on its assertion.'
		},
		{
			id: 'mdef__contract',
			label: 'Class Contract Revision',
			glyph: 'entity',
			table: 'kb.ontology_class_contract_revisions',
			columns: ['id', 'class', 'rev', 'state', 'effective from'],
			description:
				'The current immutable contract revision of the class this metric’s claim resolved to.'
		}
	],
	processor: [
		{
			id: 'proc__extract',
			label: 'Extract Metrics',
			glyph: 'stage',
			table: 'kb.metrics · extraction',
			columns: ['model', 'explicit?', 'confidence', 'location', 'extracted'],
			description: 'What extraction produced for this metric row.'
		},
		{
			id: 'proc__normalize',
			label: 'Normalize Assertion',
			glyph: 'stage',
			table: 'semantic:stage_normalize',
			columns: ['stage', 'disposition', 'status', 'category', 'findings'],
			description:
				'The normalize-stage processing outcome recorded for this metric, with its finding count.'
		},
		{
			id: 'proc__associate',
			label: 'Associate Semantics',
			glyph: 'stage',
			table: 'semantic:stage_class_resolution + associate',
			columns: ['stage', 'disposition', 'status', 'category', 'findings'],
			description:
				'The class-resolution and associate stage outcomes recorded for this metric.'
		},
		{
			id: 'proc__project',
			label: 'Project Semantics',
			glyph: 'stage',
			table: 'kb.semantic_assertions · projection',
			columns: ['assertion', 'status', 'value state', 'conformance', 'contract rev'],
			description:
				'The projection result for this metric: the resulting assertion’s status and conformance.'
		}
	],
	analysis: [
		{
			id: 'analysis__same_class',
			label: 'Metrics of Same Class',
			glyph: 'stage',
			table: 'kb.metrics · same governed class',
			columns: ['metric id', 'name', 'value', 'unit', 'document'],
			description:
				'Every other metric whose resulting assertion resolved to the same governed class as this one — its direct peer group under one contract. Click a row to recentre the explorer on that metric.',
			related: 'same_class'
		},
		{
			id: 'analysis__similar_class',
			label: 'Metrics of Similar Classes',
			glyph: 'stage',
			table: 'class-contract hybrid search',
			columns: ['metric id', 'name', 'class', 'matched via', 'score'],
			description:
				'Metrics drawn from class contracts hybrid-matched as similar to this metric’s class contract — the wider comparison frontier, ranked by contract similarity. Click a row to recentre the explorer on that metric.',
			related: 'similar_class'
		}
	],
	evidence: [
		{
			id: 'ev__ae',
			label: 'Assertion Evidence',
			glyph: 'entity',
			table: 'kb.assertion_evidence',
			columns: ['id', 'assertion', 'record', 'object', 'quote'],
			description:
				'The evidence rows behind this metric’s assertion — each a verbatim span inside a chunk.',
			evidenceSpans: ['e1', 'e2', 'e5']
		},
		{
			id: 'ev__dc',
			label: 'Decision Candidate',
			glyph: 'entity',
			table: 'kb.semantic_decision_candidates',
			columns: ['id', 'kind', 'identity', 'status', 'assertion'],
			description:
				'The semantic decision candidate(s) proposed for this metric and their status.'
		},
		{
			id: 'ev__sa',
			label: 'Semantic Assertion',
			glyph: 'entity',
			table: 'kb.semantic_assertions',
			columns: ['id', 'subject', 'predicate', 'object', 'kind', 'conf'],
			description:
				'The projected subject–predicate–object statement(s) this metric produced.'
		}
	]
};

export const BY_ID: Record<string, SatelliteNode> = Object.fromEntries(
	[CENTER, ...SATELLITES].map((n) => [n.id, n])
);

export const CHAIN_NODE_BY_ID: Record<string, ChainNode & { parentId: string }> =
	Object.fromEntries(
		Object.entries(CHAINS).flatMap(([parentId, nodes]) =>
			nodes.map((n) => [n.id, { ...n, parentId }])
		)
	);

/** Satellites that unfold a chain, in canvas order. */
export const CHAINED_SATELLITES = new Set(Object.keys(CHAINS));

export function chainFor(satelliteId: string): ChainNode[] {
	return CHAINS[satelliteId] ?? [];
}
