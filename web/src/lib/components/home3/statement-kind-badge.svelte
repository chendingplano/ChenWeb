<script lang="ts">
	// Shows what a metric row is to an expert: requirement, metric, test parameter
	// or definition (ADR 2026100603 DR5). Group as text, specific kind as tooltip.
	import {
		classifyMetricStatement,
		type MetricStatementFields
	} from '$lib/metric-statement-kind';
	import { STATEMENT_GROUP_LABEL, STATEMENT_KIND_LABEL } from '$lib/metric-statement-kind-labels';

	let { row }: { row: MetricStatementFields } = $props();

	const statement = $derived(classifyMetricStatement(row));
</script>

<span class="statement-kind {statement.group}" title={STATEMENT_KIND_LABEL[statement.kind]()}>
	{STATEMENT_GROUP_LABEL[statement.group]()}
</span>

<style>
	.statement-kind {
		display: inline-block;
		font-size: 11px;
		line-height: 1.4;
		font-weight: 600;
		border: 1px solid currentColor;
		border-radius: 999px;
		padding: 1px 8px;
		white-space: nowrap;
		user-select: text;
	}
	.requirement {
		color: #b45309;
	}
	.metric {
		color: #2563eb;
	}
	.test {
		color: #7c3aed;
	}
	.definition,
	.unclassified {
		color: #6b7280;
	}
</style>
