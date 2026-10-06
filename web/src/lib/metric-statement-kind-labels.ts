// Localized labels for metric statement kinds (ADR 2026100603 DR5). Kept apart
// from metric-statement-kind.ts so the classifier stays free of Paraglide.
import { m } from '$lib/paraglide/messages.js';
import type { StatementGroup, StatementKind } from '$lib/metric-statement-kind';

export const STATEMENT_GROUP_LABEL: Record<StatementGroup, () => string> = {
	requirement: m.metric_statement_group_requirement,
	metric: m.metric_statement_group_metric,
	test: m.metric_statement_group_test,
	definition: m.metric_statement_group_definition,
	unclassified: m.metric_statement_group_unclassified
};

export const STATEMENT_KIND_LABEL: Record<StatementKind, () => string> = {
	requirement_with_criterion: m.metric_statement_kind_requirement_with_criterion,
	requirement_value_open: m.metric_statement_kind_requirement_value_open,
	inspection_requirement: m.metric_statement_kind_inspection_requirement,
	delegated_requirement: m.metric_statement_kind_delegated_requirement,
	test_parameter: m.metric_statement_kind_test_parameter,
	metric_definition: m.metric_statement_kind_metric_definition,
	definition: m.metric_statement_kind_definition,
	metric_value: m.metric_statement_kind_metric_value,
	observation: m.metric_statement_kind_observation,
	unclassified: m.metric_statement_kind_unclassified
};
