// Classifies a kb.metrics row into what a domain expert would call it
// (ADR 2026100603 DR3). Many rows extracted as "metrics" are requirements with
// nothing to measure; customer-facing views label them by this kind. Computed
// at read time from existing fields; nothing is stored.

export type StatementKind =
	| 'requirement_with_criterion'
	| 'requirement_value_open'
	| 'inspection_requirement'
	| 'delegated_requirement'
	| 'test_parameter'
	| 'metric_definition'
	| 'definition'
	| 'metric_value'
	| 'observation'
	| 'unclassified';

export type StatementGroup = 'requirement' | 'metric' | 'test' | 'definition' | 'unclassified';

export type MetricStatementFields = {
	value_class?: string | null;
	value_range_type?: string | null;
	formula_or_definition?: string | null;
	reasoning_tags?: unknown;
};

const NUMERIC_RANGE_TYPES = new Set(['lower_bound', 'upper_bound', 'exact', 'range']);

const GROUP_OF: Record<StatementKind, StatementGroup> = {
	requirement_with_criterion: 'requirement',
	requirement_value_open: 'requirement',
	inspection_requirement: 'requirement',
	delegated_requirement: 'requirement',
	test_parameter: 'test',
	metric_definition: 'definition',
	definition: 'definition',
	metric_value: 'metric',
	observation: 'metric',
	unclassified: 'unclassified'
};

function norm(v: unknown): string {
	return typeof v === 'string' ? v.trim().toLowerCase() : '';
}

function parseTags(raw: unknown): string[] {
	let value = raw;
	if (typeof value === 'string') {
		try {
			value = JSON.parse(value);
		} catch {
			return [];
		}
	}
	if (!Array.isArray(value)) return [];
	return value.filter((t): t is string => typeof t === 'string').map((t) => t.trim().toLowerCase());
}

export function classifyMetricStatement(row: MetricStatementFields): {
	kind: StatementKind;
	group: StatementGroup;
} {
	const kind = statementKind(row);
	return { kind, group: GROUP_OF[kind] };
}

function statementKind(row: MetricStatementFields): StatementKind {
	const valueClass = norm(row.value_class);
	const rangeType = norm(row.value_range_type);
	const numeric = NUMERIC_RANGE_TYPES.has(rangeType);
	const tags = parseTags(row.reasoning_tags);

	if (tags.includes('test_condition')) return 'test_parameter';
	if (valueClass === 'definition') {
		return (row.formula_or_definition ?? '').trim() !== '' ? 'metric_definition' : 'definition';
	}
	const cites = tags.some((t) => t === 'external_reference' || t.startsWith('cited_doc:'));
	if (valueClass === 'reference' || cites) {
		return numeric ? 'metric_value' : 'delegated_requirement';
	}
	if (valueClass === 'requirement' || valueClass === 'target') {
		if (numeric) return 'requirement_with_criterion';
		if (rangeType === 'limit_absent') return 'requirement_value_open';
		return 'inspection_requirement';
	}
	if (valueClass === 'observation' || valueClass === 'design_capability') {
		return numeric ? 'metric_value' : 'observation';
	}
	return 'unclassified';
}
