const BASE = '/api/v1/kb/metrics/ontology-analysis';

export type MetricOntologyCount = { label: string; value: number; percent: number };
export type MetricOntologyDashboard = {
	status: boolean;
	generated_at: string;
	coverage: {
		writer: string;
		scope: string;
		read_model: string;
		rules: string;
		conformance: string;
		projection: string;
		resolution: string;
		denominator: number;
	};
	kpi: {
		occurrences: number;
		current_instances: number;
		ontology_metrics: number;
		metric_classes: number;
		with_errors: number;
		without_errors: number;
	};
	error_presence: MetricOntologyCount[];
	coverage_states: MetricOntologyCount[];
	mappings: Array<{ term: string; module: string; occurrences: number; instances: number; state: string }>;
	errors_by_type: Array<{ label: string; value: number; percent: number; severity: string }>;
	recent_occurrences: Array<{ id: string; document: string; metric: string; value: string; definition: string; status: string; updated: string }>;
};

export async function getMetricOntologyAnalysis(ksStoreId?: number | null): Promise<MetricOntologyDashboard> {
	const query = new URLSearchParams();
	if (ksStoreId != null) query.set('ks_store_id', String(ksStoreId));
	const response = await fetch(`${BASE}${query.size ? `?${query}` : ''}`, {
		method: 'GET',
		credentials: 'same-origin'
	});
	if (!response.ok) {
		const payload = await response.json().catch(() => null);
		throw new Error(payload?.error_msg || `Failed to load metric ontology analysis (${response.status})`);
	}
	return response.json() as Promise<MetricOntologyDashboard>;
}
