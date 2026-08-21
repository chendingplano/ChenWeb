<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import DocumentMetricsAnalysis from '$lib/components/home3/document-metrics-analysis.svelte';
	import { getMetricOntologyAnalysis } from '$lib/services/metricOntologyAnalysisService';
	import { theme } from '$lib/stores/theme.svelte';
	import {
		ArrowDownRight,
		ArrowUpRight,
		Check,
		ChevronDown,
		CircleHelp,
		Database,
		Download,
		ExternalLink,
		Filter,
		Info,
		Layers3,
		Moon,
		MoreHorizontal,
		RefreshCw,
		Search,
		Sun,
		TriangleAlert
	} from '@lucide/svelte';

	type Status = 'complete' | 'findings' | 'historical' | 'blocked' | 'incomplete' | 'failed';

	let {
		embedded = false,
		hostDarkMode = undefined,
		knowledgeStoreId = null
	}: { embedded?: boolean; hostDarkMode?: boolean; knowledgeStoreId?: number | null } = $props();
	let darkMode = $derived(hostDarkMode ?? theme.isDark);
	let loading = $state(false);
	let loadError = $state('');
	let errorFreePercent = $state('93.5%');
	let coverageMeta = $state({
		writer: 'semantic writer / current',
		scope: 'authorized corpus',
		read_model: 'metric ontology analysis v1',
		rules: 'diagnostic rules v1.4',
		conformance: 'passed',
		projection: 'current',
		resolution: 'mixed',
		denominator: 1284
	});
	let activeScope = $state('All authorized documents');
	let filterOpen = $state(false);
	let selectedMetric = $state<string | null>(null);
	let query = $state('');

	let kpis = $state([
		{ label: 'Occurrences', sub: 'record-born rows', value: '1,284', delta: '+8.4%', direction: 'up', tone: 'bronze', href: 'Document Metrics' },
		{ label: 'Current instances', sub: 'distinct assertions', value: '982', delta: '+3.1%', direction: 'up', tone: 'ink', href: 'Document Metrics' },
		{ label: 'Ontology metrics', sub: 'governed definitions', value: '146', delta: '+12', direction: 'up', tone: 'olive', href: 'Ontology Metrics' },
		{ label: 'Metric classes', sub: 'instantiated classes', value: '38', delta: '0', direction: 'flat', tone: 'slate', href: 'Ontology Metrics' },
		{ label: 'With errors', sub: 'detected error facts', value: '84', delta: '-5.2%', direction: 'down', tone: 'red', href: 'Document Metrics' },
		{ label: 'No detected errors', sub: 'not complete or warning-free', value: '1,200', delta: '+9.6%', direction: 'up', tone: 'blue', href: 'Document Metrics' }
	]);

	let coverage = $state([
		{ label: 'Complete', value: 684, percent: 53, color: 'var(--green)' },
		{ label: 'Completed with findings', value: 216, percent: 17, color: 'var(--bronze)' },
		{ label: 'Historical / not processed', value: 168, percent: 13, color: 'var(--slate)' },
		{ label: 'Blocked claim', value: 83, percent: 6, color: 'var(--red)' },
		{ label: 'Incomplete graph', value: 74, percent: 6, color: 'var(--violet)' },
		{ label: 'Execution failed', value: 42, percent: 3, color: 'var(--orange)' },
		{ label: 'Unknown coverage', value: 17, percent: 2, color: 'var(--muted)' }
	]);

	let errors = $state([
		{ label: 'Unresolved unit', value: 31, percent: 88, severity: 'Warning' },
		{ label: 'Missing assertion edge', value: 22, percent: 63, severity: 'Error' },
		{ label: 'Range type fallback', value: 16, percent: 46, severity: 'Warning' },
		{ label: 'Processor timeout', value: 9, percent: 26, severity: 'Failed' },
		{ label: 'Unknown governed term', value: 6, percent: 17, severity: 'Warning' }
	]);

	let mappings = $state([
		{ term: 'temperature', module: 'physical-quantity', occurrences: 208, instances: 146, state: 'Current' },
		{ term: 'pressure', module: 'physical-quantity', occurrences: 167, instances: 121, state: 'Current' },
		{ term: 'flow rate', module: 'process-measure', occurrences: 126, instances: 98, state: 'Current' },
		{ term: 'service life', module: 'reliability', occurrences: 82, instances: 61, state: 'Warning' }
	]);

	const statusMeta: Record<Status, { label: string; className: string }> = {
		complete: { label: 'Complete', className: 'positive' },
		findings: { label: 'Findings · 2', className: 'warning' },
		historical: { label: 'Historical', className: 'neutral' },
		blocked: { label: 'Blocked claim', className: 'blocked' },
		incomplete: { label: 'Incomplete graph', className: 'warning' },
		failed: { label: 'Execution failed', className: 'error' }
	};

	let recentRows: { id: string; document: string; metric: string; value: string; definition: string; status: Status; updated: string }[] = $state([
		{ id: 'occ-00841', document: '泵站维护规程 · §4.2', metric: '入口压力', value: '0.42 MPa', definition: 'pressure', status: 'findings', updated: '12 min ago' },
		{ id: 'occ-00840', document: '冷却系统验收 · §2.1', metric: '出口温度', value: '72 °C', definition: 'temperature', status: 'complete', updated: '18 min ago' },
		{ id: 'occ-00839', document: '流量计校准记录 · §7', metric: '额定流量', value: '1,200 L/min', definition: 'flow rate', status: 'historical', updated: '31 min ago' },
		{ id: 'occ-00838', document: '风机技术条件 · §5.3', metric: '设计寿命', value: '20,000 h', definition: 'service life', status: 'blocked', updated: '44 min ago' }
	]);

	function number(value: number) {
		return new Intl.NumberFormat('en-US').format(value);
	}

	function normalizeStatus(value: string): Status {
		if (value === 'findings' || value === 'historical' || value === 'blocked' || value === 'incomplete' || value === 'failed') return value;
		return 'complete';
	}

	async function loadAnalysis() {
		loading = true;
		loadError = '';
		try {
			const response = await getMetricOntologyAnalysis(knowledgeStoreId);
			const errorPresence = response.error_presence ?? [];
			const coverageStates = response.coverage_states ?? [];
			const errorTypes = response.errors_by_type ?? [];
			const mappingRows = response.mappings ?? [];
			const occurrenceRows = response.recent_occurrences ?? [];
			coverageMeta = response.coverage;
			errorFreePercent = `${errorPresence.find((item) => item.label === 'Without detected errors')?.percent ?? 0}%`;
			kpis = [
				{ label: 'Occurrences', sub: 'record-born rows', value: number(response.kpi.occurrences), delta: '', direction: 'flat', tone: 'bronze', href: 'Document Metrics' },
				{ label: 'Current instances', sub: 'distinct assertions', value: number(response.kpi.current_instances), delta: '', direction: 'flat', tone: 'ink', href: 'Document Metrics' },
				{ label: 'Ontology metrics', sub: 'governed definitions', value: number(response.kpi.ontology_metrics), delta: '', direction: 'flat', tone: 'olive', href: 'Ontology Metrics' },
				{ label: 'Metric classes', sub: 'instantiated classes', value: number(response.kpi.metric_classes), delta: '', direction: 'flat', tone: 'slate', href: 'Ontology Metrics' },
				{ label: 'With errors', sub: 'detected error facts', value: number(response.kpi.with_errors), delta: '', direction: 'flat', tone: 'red', href: 'Document Metrics' },
				{ label: 'No detected errors', sub: 'not complete or warning-free', value: number(response.kpi.without_errors), delta: '', direction: 'flat', tone: 'blue', href: 'Document Metrics' }
			];
			const colors = ['var(--green)', 'var(--bronze)', 'var(--slate)', 'var(--red)', 'var(--violet)', 'var(--orange)', 'var(--muted)'];
			coverage = coverageStates.map((item, index) => ({ ...item, color: colors[index % colors.length] }));
			errors = errorTypes;
			mappings = mappingRows;
			recentRows = occurrenceRows.map((row) => ({ ...row, status: normalizeStatus(row.status) }));
		} catch (error) {
			loadError = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
		}
	}

	onMount(loadAnalysis);

	let filteredRows = $derived(
		recentRows.filter((row) => `${row.document} ${row.metric} ${row.definition}`.toLowerCase().includes(query.toLowerCase()))
	);

	function selectMetric(metric: string) {
		selectedMetric = selectedMetric === metric ? null : metric;
	}

	function resetFilters() {
		activeScope = 'All authorized documents';
		query = '';
		selectedMetric = null;
	}
</script>

<svelte:head>
	<title>Metric Ontology Analysis · ChenWeb</title>
	<meta name="description" content="Read-only diagnostic view for metric ontology coverage and processing outcomes." />
</svelte:head>

{#if page.url.searchParams.get('view') === 'document'}
	<DocumentMetricsAnalysis />
{:else}

	<div class:dark={darkMode} class="page-shell" class:embedded>
	{#if !embedded}
	<header class="topbar">
		<div class="brand-lockup">
			<div class="brand-mark">CH</div>
			<div>
				<p class="eyebrow">ChenWeb / Home3</p>
				<p class="brand-name">Knowledge Store</p>
			</div>
		</div>
		<div class="top-actions">
			<button class="icon-button" aria-label="Refresh read model" title="Refresh read model"><RefreshCw size={16} /></button>
			<button class="icon-button" aria-label="Toggle theme" title="Toggle theme" onclick={() => theme.toggle()}>
				{#if darkMode}<Sun size={16} />{:else}<Moon size={16} />{/if}
			</button>
			<div class="avatar" aria-label="Signed in as C. Ding">CD</div>
		</div>
	</header>
	{/if}

	<main class="content">
		<div class="breadcrumb"><span>Ontology</span><span class="slash">/</span><strong>Metrics</strong><span class="route-badge">Read-only analysis</span></div>
		<div class="title-row">
			<div>
				<p class="section-kicker">Metric corpus / diagnostic view</p>
				<h1>Metric Ontology Analysis</h1>
				<p class="lede">Trace what the corpus knows, what the ontology governs, and where processing still needs attention.</p>
			</div>
			<div class="title-actions">
				<button class="quiet-button" onclick={() => (filterOpen = !filterOpen)}><Database size={15} /> Inspect data</button>
				<button class="primary-button"><Download size={15} /> Export view</button>
			</div>
		</div>

		<nav class="mode-tabs" aria-label="Metric analysis view">
			<a class="active" href="/home3/ontology-metric-analysis">Dashboard <span>01</span></a>
			<a href="/home3/ontology-metric-analysis?view=document">Document Metrics <span>02</span></a>
			<a href="/home3/ontology-metric-analysis?view=ontology">Ontology Metrics <span>03</span></a>
		</nav>

		<section class="coverage-notice" class:expanded={filterOpen}>
			<div class="notice-icon"><Check size={15} /></div>
			<div class="notice-copy"><strong>Coverage is {coverageMeta.conformance}</strong><span>{coverageMeta.writer} · scope: {coverageMeta.scope} · {coverageMeta.read_model} · {coverageMeta.rules}</span></div>
			<button class="notice-detail" onclick={() => (filterOpen = !filterOpen)}>{filterOpen ? 'Hide details' : 'View definition'} <ChevronDown size={14} /></button>
			{#if filterOpen}<div class="notice-expanded"><span>Conformance <b>{coverageMeta.conformance}</b></span><span>Projection <b>{coverageMeta.projection}</b></span><span>Resolution <b>{coverageMeta.resolution}</b></span><span>Denominator <b>{number(coverageMeta.denominator)} occurrences</b></span></div>{/if}
		</section>

		{#if loading}<div class="data-note">Refreshing the composed read model…</div>{/if}
		{#if loadError}<div class="data-note error-note">Live data unavailable: {loadError}. Showing the last safe fixture.</div>{/if}

		<section class="scope-bar">
			<div class="scope-label"><Filter size={15} /><span>Scope</span></div>
			<button class="scope-select" onclick={() => (activeScope = activeScope === 'All authorized documents' ? 'Standards / 2026 intake' : 'All authorized documents')}>{activeScope} <ChevronDown size={14} /></button>
			<button class="scope-chip" onclick={() => (selectedMetric = selectedMetric ? null : 'pressure')}>Class <strong>{selectedMetric ?? 'All classes'}</strong> <ChevronDown size={13} /></button>
			<button class="scope-chip" onclick={() => (selectedMetric = selectedMetric ? null : 'findings')}>Status <strong>{selectedMetric ? '1 active filter' : 'All statuses'}</strong> <ChevronDown size={13} /></button>
			<button class="reset-button" onclick={resetFilters}>Reset filters</button>
		</section>

		<section class="model-layout">
			<div class="model-graph">
				<div class="model-graph-card">
					<div class="model-head">
						<div class="model-head-copy">
							<p class="panel-kicker">The model · metric ontology §5</p>
							<h2>Metric Ontology Model</h2>
							<p class="model-lede">One metric occurrence touches three populations at once. The governed vocabulary on top is global and reused by every document. The identities in the middle are shared by every document that names the same thing. The records at the bottom belong to one document and are rewritten whenever it is processed again.</p>
						</div>
						<ul class="model-legend">
							<li class="lg-a"><span aria-hidden="true"></span><div><strong>Ontology-born</strong><small>curated, imported, or auto-promoted · versioned, never rewritten in place</small></div></li>
							<li class="lg-b"><span aria-hidden="true"></span><div><strong>Corpus-level identity</strong><small>resolved across documents · merged or deprecated, never deleted</small></div></li>
							<li class="lg-c"><span aria-hidden="true"></span><div><strong>Record-born</strong><small>one extraction run over one document · replaced on reprocessing</small></div></li>
						</ul>
					</div>
					<div class="model-canvas">
						<svg viewBox="0 0 1520 740" role="img" aria-labelledby="mm-title mm-desc">
							<title id="mm-title">The Metric Ontology model</title>
							<desc id="mm-desc">Three horizontal lanes. The top lane, ontology-born, holds the governed vocabulary: ontology modules publish module releases of kb.ontology_terms, which group into metric identity (metric_definition, metric class), measurement science (quantity kind, dimension, unit, and the governed value range mapping), and the claim frame (assertion kinds, binding properties, and the six measurement classes); ontology candidates auto-promote into terms and term labels supply readable names. The middle lane, corpus-level identities, holds the keyword concept, the object node, and the canonical claim identity. The bottom lane, record-born, runs the pipeline left to right: document in kb.inputs, extract_metrics produces a metric row in kb.metrics and an object mention in kb.artifact_objects, normalize_assertions produces a decision candidate, associate_semantics produces a semantic assertion with its processing outcome and findings, and assertion evidence traces the stored claim back to the quote and line spans in the source document. The metric row resolves its name to a keyword concept and then to a governed metric_definition term; its subject reconciles to an object node that becomes the assertion's subject; normalization resolves value range type, unit, and quantity kind against the ontology; and the assertion points at an assertion kind, a metric class, and a canonical claim identity.</desc>
							<defs>
								<marker id="mm-arw-a" viewBox="0 0 10 10" refX="8.5" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" class="mk mk-a" /></marker>
								<marker id="mm-arw-b" viewBox="0 0 10 10" refX="8.5" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" class="mk mk-b" /></marker>
								<marker id="mm-arw-c" viewBox="0 0 10 10" refX="8.5" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" class="mk mk-c" /></marker>
							</defs>

							<rect class="lane lane-a" x="16" y="14" width="1488" height="278" rx="12" />
							<rect class="lane lane-b" x="16" y="320" width="1488" height="128" rx="12" />
							<rect class="lane lane-c" x="16" y="464" width="1488" height="260" rx="12" />
							<text class="lane-name ln-a" transform="translate(40 153) rotate(-90)">ONTOLOGY-BORN</text>
							<text class="lane-name ln-b" transform="translate(40 384) rotate(-90)">CORPUS-LEVEL</text>
							<text class="lane-name ln-c" transform="translate(40 594) rotate(-90)">RECORD-BORN</text>
							<text class="lane-caption" x="74" y="42">Governed vocabulary — authored once, reused by every document, versioned rather than rewritten</text>
							<text class="lane-caption" x="74" y="338">Identities resolved across the corpus — no governance status of their own</text>
							<text class="lane-caption" x="74" y="486">Pipeline over one document</text>

							<g class="mnode mnode-a"><rect x="74" y="100" width="208" height="64" rx="8" /><text class="t" x="178" y="126">ontology modules</text><text class="s" x="178" y="146">core · quantity · measurement</text></g>
							<g class="mnode mnode-a"><rect x="74" y="190" width="208" height="64" rx="8" /><text class="t" x="178" y="216">module release</text><text class="s" x="178" y="236">one active per module</text></g>
							<line class="medge medge-a" x1="178" y1="164" x2="178" y2="187" marker-end="url(#mm-arw-a)" />
							<text class="elabel" x="186" y="181">publishes</text>
							<line class="medge medge-a" x1="282" y1="222" x2="299" y2="222" marker-end="url(#mm-arw-a)" />

							<rect class="shell" x="302" y="64" width="913" height="216" rx="10" />
							<text class="shell-title" x="318" y="90">kb.ontology_terms · kb.ontology_term_headers</text>
							<text class="shell-sub" x="318" y="108">governed term — identifier · kind · module · version · status</text>
							<text class="grp" x="467" y="136">METRIC IDENTITY</text>
							<text class="grp" x="760" y="136">MEASUREMENT SCIENCE</text>
							<text class="grp" x="1053" y="136">CLAIM FRAME</text>
							<g class="chip"><rect x="329" y="146" width="276" height="44" rx="7" /><text class="ct" x="467" y="166">metric_definition</text><text class="cs" x="467" y="181">what the metric is, independent of any document</text></g>
							<g class="chip"><rect x="329" y="200" width="276" height="44" rx="7" /><text class="ct" x="467" y="220">class · metric class</text><text class="cs" x="467" y="235">what a stored claim is an instance of</text></g>
							<g class="chip"><rect x="622" y="146" width="276" height="44" rx="7" /><text class="ct" x="760" y="166">quantity_kind → dimension → unit</text><text class="cs" x="760" y="181">what · which units agree · how it is written</text></g>
							<g class="chip chip-edge"><rect x="622" y="200" width="276" height="44" rx="7" /><text class="ct" x="760" y="220">kb.metric_value_range_type_map</text><text class="cs" x="760" y="235">governed mapping, not an ontology term</text></g>
							<g class="chip"><rect x="915" y="146" width="276" height="44" rx="7" /><text class="ct" x="1053" y="166">assertion kind</text><text class="cs" x="1053" y="181">observed · exact · bounds · target · capability</text></g>
							<g class="chip"><rect x="915" y="200" width="276" height="44" rx="7" /><text class="ct" x="1053" y="220">binding properties</text><text class="cs" x="1053" y="235">mea:measured_by · has_unit · has_quantity_kind</text></g>
							<text class="shell-foot" x="758" y="266">measurement classes — feature_of_interest · observable_property · procedure · condition · aggregation_window · metric_assertion</text>

							<g class="mnode mnode-a"><rect x="1255" y="100" width="249" height="64" rx="8" /><text class="t" x="1379" y="126">ontology candidates</text><text class="s" x="1379" y="146">discovered → auto-promoted</text></g>
							<line class="medge medge-a medge-dash" x1="1254" y1="132" x2="1218" y2="132" marker-end="url(#mm-arw-a)" />
							<g class="mnode mnode-a"><rect x="1255" y="190" width="249" height="64" rx="8" /><text class="t" x="1379" y="216">term labels</text><text class="s" x="1379" y="236">kb.ontology_term_labels</text></g>
							<line class="medge medge-a medge-dash" x1="1254" y1="222" x2="1218" y2="222" marker-end="url(#mm-arw-a)" />

							<g class="mnode mnode-b"><rect x="367" y="352" width="200" height="76" rx="8" /><text class="t" x="467" y="379">keyword concept</text><text class="s" x="467" y="398">kb.keyword_concepts</text><text class="s2" x="467" y="415">groups every spelling of one name</text></g>
							<g class="mnode mnode-b"><rect x="870" y="352" width="180" height="76" rx="8" /><text class="t" x="960" y="379">object node</text><text class="s" x="960" y="398">kb.object_nodes</text><text class="s2" x="960" y="415">the reconciled thing measured</text></g>
							<g class="mnode mnode-b"><rect x="1214" y="352" width="220" height="76" rx="8" /><text class="t" x="1324" y="379">canonical claim identity</text><text class="s" x="1324" y="398">kb.semantic_claim_identities</text><text class="s2" x="1324" y="415">equal claims converge, not duplicate</text></g>

							<g class="mnode mnode-c"><rect x="74" y="498" width="200" height="78" rx="8" /><text class="t" x="174" y="525">document</text><text class="s" x="174" y="544">kb.inputs</text><text class="s2" x="174" y="561">the record as the system received it</text></g>
							<g class="mnode mnode-c mnode-center"><rect x="367" y="498" width="200" height="78" rx="8" /><text class="t" x="467" y="525">metric row</text><text class="s" x="467" y="544">kb.metrics</text><text class="s2" x="467" y="561">one metric as one document stated it</text></g>
							<g class="mnode mnode-c"><rect x="660" y="498" width="200" height="78" rx="8" /><text class="t" x="760" y="525">decision candidate</text><text class="s" x="760" y="544">kb.semantic_decision_candidates</text><text class="s2" x="760" y="561">proposed claim awaiting adjudication</text></g>
							<g class="mnode mnode-c"><rect x="953" y="498" width="200" height="78" rx="8" /><text class="t" x="1053" y="525">semantic assertion</text><text class="s" x="1053" y="544">kb.semantic_assertions</text><text class="s2" x="1053" y="561">the stored claim · four state axes</text></g>
							<g class="mnode mnode-c"><rect x="1246" y="498" width="200" height="78" rx="8" /><text class="t" x="1346" y="525">assertion evidence</text><text class="s" x="1346" y="544">kb.assertion_evidence</text><text class="s2" x="1346" y="561">quote · line spans · run</text></g>
							<g class="mnode mnode-c"><rect x="580" y="606" width="220" height="64" rx="8" /><text class="t" x="690" y="632">object mention</text><text class="s" x="690" y="652">kb.artifact_objects · reconcile status</text></g>
							<g class="mnode mnode-c"><rect x="953" y="606" width="280" height="64" rx="8" /><text class="t" x="1093" y="632">processing outcome · findings</text><text class="s" x="1093" y="652">kb.semantic_processing_outcomes · _findings</text></g>

							<line class="medge medge-c" x1="274" y1="537" x2="365" y2="537" marker-end="url(#mm-arw-c)" />
							<line class="medge medge-c" x1="567" y1="537" x2="658" y2="537" marker-end="url(#mm-arw-c)" />
							<line class="medge medge-c" x1="860" y1="537" x2="951" y2="537" marker-end="url(#mm-arw-c)" />
							<line class="medge medge-c" x1="1153" y1="537" x2="1244" y2="537" marker-end="url(#mm-arw-c)" />
							<text class="stage" x="320" y="486">extract_metrics</text>
							<text class="stage" x="613" y="486">normalize_assertions</text>
							<text class="stage" x="906" y="486">associate_semantics</text>
							<text class="stage" x="1199" y="486">attaches evidence</text>

							<path class="medge medge-c" d="M467 576 L467 592 L690 592 L690 604" marker-end="url(#mm-arw-c)" />
							<text class="elabel" x="500" y="587">subject</text>
							<path class="medge medge-b" d="M800 638 L900 638 L900 431" marker-end="url(#mm-arw-b)" />
							<text class="elabel elabel-mid" x="850" y="630">reconciled to</text>
							<line class="medge medge-b" x1="467" y1="498" x2="467" y2="431" marker-end="url(#mm-arw-b)" />
							<text class="elabel" x="477" y="460">metric_name</text>
							<line class="medge medge-a" x1="467" y1="352" x2="467" y2="282" marker-end="url(#mm-arw-a)" />
							<text class="elabel" x="477" y="314">core:aligns_to_term</text>
							<line class="medge medge-a" x1="760" y1="498" x2="760" y2="282" marker-end="url(#mm-arw-a)" />
							<text class="elabel" x="770" y="314">resolves value range type · unit · quantity kind</text>
							<line class="medge medge-a" x1="1075" y1="498" x2="1075" y2="282" marker-end="url(#mm-arw-a)" />
							<text class="elabel" x="1085" y="314">instance_of · assertion kind · unit · quantity kind</text>
							<line class="medge medge-b" x1="1000" y1="428" x2="1000" y2="496" marker-end="url(#mm-arw-b)" />
							<text class="elabel elabel-end" x="992" y="460">subject_object_id</text>
							<path class="medge medge-b" d="M1130 498 L1130 470 L1324 470 L1324 430" marker-end="url(#mm-arw-b)" />
							<text class="elabel" x="1160" y="458">logical_identity_key</text>
							<line class="medge medge-c medge-dash" x1="1053" y1="576" x2="1053" y2="604" marker-end="url(#mm-arw-c)" />
							<text class="elabel" x="1063" y="598">per-stage outcome</text>
							<path class="medge medge-c medge-dash" d="M1346 576 L1346 690 L172 690 L172 578" marker-end="url(#mm-arw-c)" />
							<text class="elabel elabel-mid" x="760" y="682">assertion evidence traces the stored claim back to the quote and line spans in the source document</text>
						</svg>
					</div>
				</div>
			</div>

			<div class="spoke spoke-terms">
				<div class="kpi-grid">
					{#each [kpis[2], kpis[3]] as kpi, i}
						<button class="kpi kpi-{kpi.tone}" onclick={() => selectMetric(kpi.label)} class:selected={selectedMetric === kpi.label}>
							<span class="kpi-index">0{i + 3}</span><span class="kpi-label">{kpi.label} <CircleHelp size={13} /></span>
							<strong>{kpi.value}</strong>
							<span class="kpi-footer"><span>{kpi.sub}</span><span class:negative={kpi.direction === 'down'} class:flat={kpi.direction === 'flat'}>{#if kpi.direction === 'up'}<ArrowUpRight size={13} />{:else if kpi.direction === 'down'}<ArrowDownRight size={13} />{/if}{kpi.delta}</span></span>
						</button>
					{/each}
				</div>
				<div class="panel mapping-panel">
					<div class="panel-head"><div><p class="panel-kicker">ontology_terms · mapping inventory</p><h2>Governed vocabulary in the corpus</h2></div><div class="segmented"><button class="selected">Global</button><button>In scope</button></div></div>
					<p class="panel-note">Global mapping inventory · document and class filters do not apply here.</p>
					<table class="mapping-table"><caption>Metric definition mapping inventory</caption><thead><tr><th>Definition</th><th>Module</th><th>Occurrences</th><th>Instances</th><th></th></tr></thead><tbody>{#each mappings as row}<tr class:selected-row={selectedMetric === row.term} onclick={() => selectMetric(row.term)}><td><span class="term-mark">◈</span><strong>{row.term}</strong></td><td>{row.module}</td><td>{row.occurrences}</td><td>{row.instances}</td><td><span class:warning-text={row.state === 'Warning'}>{row.state}</span><ExternalLink size={13} /></td></tr>{:else}<tr><td colspan="5" class="empty-row">(no data)</td></tr>{/each}</tbody></table>
				</div>
			</div>

			<div class="spoke spoke-assertions">
				<div class="kpi-grid">
					<button class="kpi kpi-{kpis[1].tone}" onclick={() => selectMetric(kpis[1].label)} class:selected={selectedMetric === kpis[1].label}>
						<span class="kpi-index">02</span><span class="kpi-label">{kpis[1].label} <CircleHelp size={13} /></span>
						<strong>{kpis[1].value}</strong>
						<span class="kpi-footer"><span>{kpis[1].sub}</span><span class:negative={kpis[1].direction === 'down'} class:flat={kpis[1].direction === 'flat'}>{#if kpis[1].direction === 'up'}<ArrowUpRight size={13} />{:else if kpis[1].direction === 'down'}<ArrowDownRight size={13} />{/if}{kpis[1].delta}</span></span>
					</button>
				</div>
				<div class="panel coverage-panel">
					<div class="panel-head"><div><p class="panel-kicker">semantic_assertions · coverage state</p><h2>Where the semantic graph stands</h2></div><button class="text-button">Definition <Info size={14} /></button></div>
					<div class="coverage-bars">{#each coverage as item}<div class="bar-row"><div class="bar-label"><span>{item.label}</span><strong>{item.value}</strong></div><div class="track"><span style={`width:${item.percent}%; background:${item.color}`}></span></div></div>{:else}<div class="empty-state">(no data)</div>{/each}</div>
				</div>
				<div class="panel errors-panel">
					<div class="panel-head"><div><p class="panel-kicker">semantic_assertions · errors by type</p><h2>Canonical facts, ranked by severity</h2></div><button class="text-button">All sources <ChevronDown size={14} /></button></div>
					<div class="error-bars">
						{#if errors.length === 0}
							<div class="empty-state empty-state-panel">(no data)</div>
						{:else}
							{#each errors as error}<div class="error-bar-row"><div class="error-bar-meta"><span>{error.label}</span><span><b>{error.value}</b> affected <em>{error.severity}</em></span></div><div class="error-track"><span style={`width:${error.percent}%`}></span></div></div>{/each}
						{/if}
					</div>
					<div class="source-footer"><span>Contributing sources</span><span><b>27</b> documents</span><span><b>5</b> error types</span><a href="/home3/ontology-metric-analysis?view=document">Open Document Metrics <ArrowUpRight size={14} /></a></div>
				</div>
			</div>

			<div class="spoke spoke-evidence">
				<div class="kpi-grid">
					<button class="kpi kpi-{kpis[0].tone}" onclick={() => selectMetric(kpis[0].label)} class:selected={selectedMetric === kpis[0].label}>
						<span class="kpi-index">01</span><span class="kpi-label">{kpis[0].label} <CircleHelp size={13} /></span>
						<strong>{kpis[0].value}</strong>
						<span class="kpi-footer"><span>{kpis[0].sub}</span><span class:negative={kpis[0].direction === 'down'} class:flat={kpis[0].direction === 'flat'}>{#if kpis[0].direction === 'up'}<ArrowUpRight size={13} />{:else if kpis[0].direction === 'down'}<ArrowDownRight size={13} />{/if}{kpis[0].delta}</span></span>
					</button>
					{#each [kpis[4], kpis[5]] as kpi, i}
						<button class="kpi kpi-{kpi.tone}" onclick={() => selectMetric(kpi.label)} class:selected={selectedMetric === kpi.label}>
							<span class="kpi-index">0{i + 5}</span><span class="kpi-label">{kpi.label} <CircleHelp size={13} /></span>
							<strong>{kpi.value}</strong>
							<span class="kpi-footer"><span>{kpi.sub}</span><span class:negative={kpi.direction === 'down'} class:flat={kpi.direction === 'flat'}>{#if kpi.direction === 'up'}<ArrowUpRight size={13} />{:else if kpi.direction === 'down'}<ArrowDownRight size={13} />{/if}{kpi.delta}</span></span>
						</button>
					{/each}
				</div>
				<div class="panel error-panel">
					<div class="panel-head"><div><p class="panel-kicker">assertion_evidence · error presence</p><h2>Most metrics are clean, not necessarily complete.</h2></div><button class="more-button" aria-label="More error presence options"><MoreHorizontal size={18} /></button></div>
					<div class="error-visual"><div class="donut"><div><strong>{errorFreePercent}</strong><span>no detected errors</span></div></div><div class="error-legend"><div><span class="dot blue"></span><strong>{kpis[5].value}</strong><span>without detected errors</span></div><div><span class="dot red"></span><strong>{kpis[4].value}</strong><span>with error facts</span></div><p><TriangleAlert size={13} /> A clean processing result can still carry warnings or silent gaps.</p></div></div>
					<table class="exact-table"><caption>Exact error presence values</caption><tbody><tr><td>Without detected errors</td><td>{kpis[5].value}</td><td>—</td></tr><tr><td>With error facts</td><td>{kpis[4].value}</td><td>—</td></tr></tbody></table>
				</div>

				<section class="recent-section">
					<div class="section-heading"><div><p class="panel-kicker">Latest occurrences</p><h2>Recent activity worth inspecting {#if filteredRows.length === 0}<span class="empty-inline">(no data)</span>{/if}</h2></div><div class="table-tools"><label class="search-field"><Search size={15} /><input bind:value={query} placeholder="Search metric or document" aria-label="Search metric or document" /></label><button class="quiet-button">Columns <ChevronDown size={14} /></button></div></div>
					{#if filteredRows.length === 0}
						<div class="recent-empty empty-state-panel">(no data)</div>
					{:else}
						<div class="table-wrap"><table class="recent-table"><caption>Recent metric occurrences</caption><thead><tr><th>Occurrence</th><th>Metric / document</th><th>Value</th><th>Governed definition</th><th>State</th><th>Updated</th></tr></thead><tbody>{#each filteredRows as row}<tr><td><span class="mono">{row.id}</span></td><td><strong>{row.metric}</strong><small>{row.document}</small></td><td>{row.value}</td><td><span class="definition-chip">{row.definition}</span></td><td><span class="status-badge {statusMeta[row.status].className}">{statusMeta[row.status].label}</span></td><td>{row.updated}</td></tr>{/each}</tbody></table></div>
					{/if}
				</section>
			</div>
		</section>

		<footer class="page-footer"><span><Layers3 size={14} /> Metric ontology analysis · Page 1 of 3</span><span>Last updated 20 Aug 2026, 14:32 UTC <button class="footer-link">View API shape</button></span></footer>
	</main>
	</div>
{/if}

<style>
	@import url('https://fonts.googleapis.com/css2?family=DM+Mono:wght@400;500&family=Manrope:wght@400;500;600;700;800&display=swap');
	:global(*) { box-sizing: border-box; }
	:global(body) { margin: 0; font-family: 'Manrope', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; }
	:global(button), :global(input) { font: inherit; }
	:global(button), :global(a) { -webkit-tap-highlight-color: transparent; }
	:global(button:focus-visible), :global(a:focus-visible), :global(input:focus-visible) { outline: 3px solid oklch(0.72 0.12 65); outline-offset: 3px; }
	:root { --paper: oklch(0.965 0.014 78); --panel: oklch(0.985 0.009 78); --ink: oklch(0.23 0.025 63); --muted: oklch(0.55 0.025 72); --line: oklch(0.88 0.028 75); --bronze: oklch(0.61 0.09 69); --green: oklch(0.58 0.09 145); --red: oklch(0.58 0.13 26); --blue: oklch(0.56 0.11 245); --violet: oklch(0.59 0.1 302); --orange: oklch(0.68 0.12 52); --slate: oklch(0.57 0.035 245); --muted-bar: oklch(0.78 0.02 72); --primary-bg: var(--ink); --primary-text: var(--paper); }
	.page-shell { --bg: var(--paper); --surface: var(--panel); --text: var(--ink); --subtle: var(--muted); --border: var(--line); --shadow: 0 12px 30px oklch(0.3 0.02 70 / 0.06); min-height: 100vh; background: var(--bg); color: var(--text); transition: background 180ms ease, color 180ms ease; }
	.page-shell.dark { --bg: #111827; --surface: #182334; --text: #f7f5f1; --subtle: #c4cfdf; --border: #304663; --bronze: #ff9b54; --green: #47b699; --red: #ff7d6b; --blue: #78a5e5; --violet: #9c8de2; --orange: #ffb072; --slate: #8ea2bb; --ink: #f7f5f1; --primary-bg: var(--bronze); --primary-text: #111827; --shadow: 0 12px 30px rgba(7, 10, 18, 0.32); --muted-bar: #405773; }
	.page-shell.embedded { height: 100%; min-height: 0; overflow-x: hidden; overflow-y: auto; }
	.page-shell.embedded .content { padding-top: 25px; }
	.data-note { margin: 13px 0; padding: 10px 12px; border: 1px solid var(--border); background: color-mix(in oklch, var(--bronze) 8%, transparent); color: var(--subtle); font: 500 11px 'DM Mono', monospace; }
	.error-note { color: var(--red); border-color: color-mix(in oklch, var(--red) 45%, var(--border)); }
	.topbar { height: 70px; padding: 0 4.5vw; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; background: color-mix(in oklch, var(--surface) 80%, transparent); }
	.brand-lockup, .top-actions, .scope-bar, .breadcrumb, .title-actions, .notice-copy, .source-footer, .page-footer, .table-tools { display: flex; align-items: center; }
	.brand-lockup { gap: 11px; }.brand-mark { width: 30px; height: 30px; border: 1px solid var(--bronze); color: var(--bronze); display: grid; place-items: center; font: 500 11px 'DM Mono', monospace; letter-spacing: -0.08em; }.eyebrow, .section-kicker, .panel-kicker { margin: 0; text-transform: uppercase; letter-spacing: 0.13em; font: 500 10px 'DM Mono', monospace; color: var(--bronze); }.brand-name { margin: 2px 0 0; font-size: 13px; font-weight: 700; }.top-actions { gap: 11px; }.icon-button, .more-button { border: 0; color: var(--subtle); background: transparent; cursor: pointer; padding: 7px; }.icon-button:hover, .more-button:hover { color: var(--text); }.avatar { width: 30px; height: 30px; background: var(--ink); color: var(--paper); display: grid; place-items: center; border-radius: 50%; font: 500 10px 'DM Mono', monospace; }
	.content { max-width: 1840px; margin: 0 auto; padding: 33px clamp(18px, 2.4vw, 46px) 28px; }.breadcrumb { gap: 9px; color: var(--subtle); font-size: 12px; }.breadcrumb strong { color: var(--text); }.slash { color: var(--bronze); }.route-badge { margin-left: 8px; padding: 4px 8px; border: 1px solid var(--border); border-radius: 99px; font: 500 10px 'DM Mono', monospace; color: var(--subtle); }.title-row { margin: 28px 0 26px; display: flex; justify-content: space-between; gap: 24px; align-items: end; }.title-row h1 { margin: 7px 0 8px; font-size: clamp(27px, 3.3vw, 44px); letter-spacing: -0.055em; line-height: 1.02; }.lede { margin: 0; max-width: 650px; color: var(--subtle); font-size: 14px; line-height: 1.65; }.title-actions { gap: 10px; }.quiet-button, .primary-button, .reset-button, .text-button, .scope-select, .scope-chip, .variant-picker button, .segmented button { display: inline-flex; align-items: center; gap: 7px; cursor: pointer; }.quiet-button, .primary-button { padding: 10px 13px; border: 1px solid var(--border); background: transparent; color: var(--text); font-size: 12px; }.quiet-button:hover { background: var(--surface); }.primary-button { border-color: var(--primary-bg); background: var(--primary-bg); color: var(--primary-text); }.mode-tabs { display: flex; gap: 2px; border-bottom: 1px solid var(--border); }.mode-tabs a { padding: 11px 15px 12px; color: var(--subtle); text-decoration: none; font-size: 12px; border-bottom: 2px solid transparent; }.mode-tabs a:hover, .mode-tabs a.active { color: var(--text); border-color: var(--bronze); }.mode-tabs span { margin-left: 8px; color: var(--bronze); font: 500 10px 'DM Mono', monospace; }
	.coverage-notice { position: relative; display: flex; align-items: center; gap: 10px; margin: 17px 0 11px; padding: 10px 12px; border: 1px solid var(--border); background: color-mix(in oklch, var(--surface) 55%, var(--bronze) 4%); font-size: 11px; }.notice-icon { width: 21px; height: 21px; border: 1px solid var(--green); color: var(--green); display: grid; place-items: center; border-radius: 50%; }.notice-copy { gap: 9px; flex-wrap: wrap; }.notice-copy strong { font-weight: 700; }.notice-copy span { color: var(--subtle); }.notice-detail { display: inline-flex; align-items: center; gap: 4px; margin-left: auto; border: 0; background: transparent; color: var(--bronze); font-size: 11px; cursor: pointer; }.notice-expanded { width: 100%; flex-basis: 100%; display: flex; gap: 26px; padding: 10px 0 2px 31px; border-top: 1px solid var(--border); color: var(--subtle); }.notice-expanded b { margin-left: 4px; color: var(--text); font-weight: 600; }.coverage-notice.expanded { flex-wrap: wrap; }
	.variant-picker { display: flex; gap: 7px; align-items: center; margin: 17px 0 12px; color: var(--subtle); font-size: 11px; }.variant-picker > span { margin-right: 5px; font: 500 10px 'DM Mono', monospace; text-transform: uppercase; letter-spacing: 0.08em; }.variant-picker button { padding: 6px 8px; border: 1px solid var(--border); background: transparent; color: var(--subtle); font-size: 11px; }.variant-picker button small { display: none; }.variant-picker button.chosen { color: var(--text); border-color: var(--bronze); background: color-mix(in oklch, var(--bronze) 9%, transparent); }
	.scope-bar { min-height: 50px; gap: 9px; margin: 10px 0 15px; padding: 8px 11px; border-top: 1px solid var(--border); border-bottom: 1px solid var(--border); }.scope-label { display: flex; align-items: center; gap: 7px; margin-right: 5px; font: 500 10px 'DM Mono', monospace; text-transform: uppercase; color: var(--bronze); }.scope-select, .scope-chip { padding: 7px 9px; border: 1px solid var(--border); background: var(--surface); color: var(--text); font-size: 11px; }.scope-chip { color: var(--subtle); }.scope-chip strong { color: var(--text); font-weight: 600; }.reset-button { margin-left: auto; padding: 7px 0; border: 0; background: transparent; color: var(--bronze); font-size: 11px; }
	.kpi-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 9px; margin-bottom: 12px; }.kpi { position: relative; min-height: 132px; padding: 17px 15px 13px; text-align: left; border: 1px solid var(--border); border-top: 2px solid var(--tone, var(--bronze)); background: var(--surface); color: var(--text); box-shadow: var(--shadow); cursor: pointer; }.kpi:hover, .kpi.selected { transform: translateY(-2px); border-color: var(--tone); }.kpi-index { position: absolute; top: 12px; right: 13px; color: var(--subtle); font: 500 10px 'DM Mono', monospace; }.kpi-label { display: inline-flex; align-items: center; gap: 4px; color: var(--subtle); font-size: 11px; }.kpi strong { display: block; margin: 24px 0 17px; font: 700 28px 'DM Mono', monospace; letter-spacing: -0.08em; }.kpi-footer { display: flex; justify-content: space-between; gap: 6px; color: var(--subtle); font-size: 10px; }.kpi-footer span:last-child { display: flex; align-items: center; gap: 2px; color: var(--green); font-family: 'DM Mono', monospace; }.kpi-footer .negative { color: var(--red) !important; }.kpi-footer .flat { color: var(--subtle) !important; }.kpi-bronze { --tone: var(--bronze); }.kpi-ink { --tone: var(--ink); }.kpi-olive { --tone: var(--green); }.kpi-slate { --tone: var(--slate); }.kpi-red { --tone: var(--red); }.kpi-blue { --tone: var(--blue); }
	.model-layout { display: grid; grid-template-columns: 1fr 1fr; grid-template-areas: "graph graph" "terms assertions" "evidence evidence"; gap: 15px; }.spoke-terms { grid-area: terms; }.spoke-terms .mapping-panel { flex: 1; }.model-graph { grid-area: graph; min-width: 0; margin-bottom: 7px; }.model-graph-card { position: relative; padding: 24px 26px 22px; border: 1px solid color-mix(in oklch, var(--bronze) 42%, var(--border)); border-top: 3px solid var(--bronze); background: linear-gradient(180deg, color-mix(in srgb, var(--bronze) 8%, var(--surface)) 0, var(--surface) 230px); box-shadow: 0 18px 46px color-mix(in oklch, var(--bronze) 16%, transparent); }.model-head { display: flex; justify-content: space-between; align-items: start; gap: 34px; flex-wrap: wrap; margin-bottom: 18px; }.model-head-copy { flex: 1 1 460px; min-width: 0; }.model-head h2 { margin: 6px 0 0; font-size: 22px; letter-spacing: -0.035em; line-height: 1.15; }.model-lede { margin: 9px 0 0; max-width: 840px; color: var(--subtle); font-size: 12.5px; line-height: 1.7; }.model-legend { flex: 0 1 430px; display: grid; gap: 9px; margin: 3px 0 0; padding: 0; list-style: none; }.model-legend li { display: grid; grid-template-columns: 10px 1fr; align-items: start; gap: 9px; }.model-legend li > span { width: 10px; height: 10px; margin-top: 3px; border-radius: 2px; background: var(--tone); }.model-legend strong { display: block; font-size: 11.5px; font-weight: 700; }.model-legend small { display: block; margin-top: 2px; color: var(--subtle); font-size: 10px; line-height: 1.45; }.lg-a { --tone: var(--bronze); }.lg-b { --tone: var(--violet); }.lg-c { --tone: var(--blue); }.model-canvas { overflow-x: auto; }.model-canvas svg { display: block; width: 100%; min-width: 1080px; height: auto; }.model-graph .lane { stroke-width: 1; }.model-graph .lane-a { fill: color-mix(in oklch, var(--bronze) 10%, transparent); stroke: color-mix(in oklch, var(--bronze) 30%, transparent); }.model-graph .lane-b { fill: color-mix(in oklch, var(--violet) 11%, transparent); stroke: color-mix(in oklch, var(--violet) 28%, transparent); }.model-graph .lane-c { fill: color-mix(in oklch, var(--blue) 11%, transparent); stroke: color-mix(in oklch, var(--blue) 28%, transparent); }.model-graph .lane-name { font: 700 11px 'Manrope', sans-serif; letter-spacing: 0.16em; text-anchor: middle; }.model-graph .ln-a { fill: var(--bronze); }.model-graph .ln-b { fill: var(--violet); }.model-graph .ln-c { fill: var(--blue); }.model-graph .lane-caption { fill: var(--subtle); font: 500 10px 'Manrope', sans-serif; }.model-graph .mnode rect { fill: var(--surface); stroke: var(--border); stroke-width: 1; }.model-graph .mnode-a rect { stroke: color-mix(in oklch, var(--bronze) 62%, transparent); }.model-graph .mnode-b rect { stroke: color-mix(in oklch, var(--violet) 62%, transparent); }.model-graph .mnode-c rect { stroke: color-mix(in oklch, var(--blue) 62%, transparent); }.model-graph .mnode-center rect { stroke: var(--bronze); stroke-width: 2.2; fill: color-mix(in srgb, var(--bronze) 14%, var(--surface)); }.model-graph .t { fill: var(--text); font: 600 11px 'Manrope', sans-serif; text-anchor: middle; }.model-graph .s { fill: var(--bronze); font: 500 8.5px 'DM Mono', monospace; text-anchor: middle; }.model-graph .s2 { fill: var(--subtle); font: 500 8.5px 'Manrope', sans-serif; text-anchor: middle; }.model-graph .shell { fill: color-mix(in oklch, var(--surface) 72%, transparent); stroke: color-mix(in oklch, var(--bronze) 46%, transparent); stroke-width: 1; stroke-dasharray: 3 3; }.model-graph .shell-title { fill: var(--text); font: 600 11px 'DM Mono', monospace; }.model-graph .shell-sub { fill: var(--subtle); font: 500 8.5px 'DM Mono', monospace; }.model-graph .shell-foot { fill: var(--subtle); font: 500 8.5px 'DM Mono', monospace; text-anchor: middle; }.model-graph .grp { fill: var(--bronze); font: 700 9px 'Manrope', sans-serif; letter-spacing: 0.14em; text-anchor: middle; }.model-graph .chip rect { fill: var(--surface); stroke: color-mix(in oklch, var(--bronze) 50%, transparent); stroke-width: 1; }.model-graph .chip-edge rect { stroke-dasharray: 4 3; }.model-graph .ct { fill: var(--text); font: 500 10px 'DM Mono', monospace; text-anchor: middle; }.model-graph .cs { fill: var(--subtle); font: 500 8.5px 'Manrope', sans-serif; text-anchor: middle; }.model-graph .medge { fill: none; stroke-width: 1.4; }.model-graph .medge-a { stroke: color-mix(in oklch, var(--bronze) 78%, transparent); }.model-graph .medge-b { stroke: color-mix(in oklch, var(--violet) 78%, transparent); }.model-graph .medge-c { stroke: color-mix(in oklch, var(--blue) 78%, transparent); }.model-graph .medge-dash { stroke-dasharray: 5 4; }.model-graph .mk-a { fill: var(--bronze); }.model-graph .mk-b { fill: var(--violet); }.model-graph .mk-c { fill: var(--blue); }.model-graph .elabel { fill: var(--subtle); font: 500 8.5px 'DM Mono', monospace; }.model-graph .elabel-mid { text-anchor: middle; }.model-graph .elabel-end { text-anchor: end; }.model-graph .stage { fill: var(--bronze); font: 500 9px 'DM Mono', monospace; text-anchor: middle; letter-spacing: 0.02em; }.spoke-assertions { grid-area: assertions; }.spoke-evidence { grid-area: evidence; }.spoke { display: flex; flex-direction: column; gap: 12px; }.panel { min-width: 0; padding: 21px 21px 18px; border: 1px solid var(--border); background: var(--surface); box-shadow: var(--shadow); }.panel-head { display: flex; justify-content: space-between; gap: 20px; align-items: start; }.panel-head h2, .section-heading h2 { margin: 6px 0 0; font-size: 16px; letter-spacing: -0.025em; line-height: 1.25; }.text-button { border: 0; background: transparent; color: var(--bronze); font-size: 11px; white-space: nowrap; }.error-visual { display: flex; align-items: center; gap: 28px; padding: 22px 0 20px; }.donut { width: 154px; height: 154px; flex: 0 0 auto; display: grid; place-items: center; border-radius: 50%; background: conic-gradient(var(--blue) 0 93.5%, var(--red) 93.5% 100%); }.donut::before { content: ''; width: 114px; height: 114px; position: absolute; border-radius: 50%; background: var(--surface); }.donut > div { position: relative; text-align: center; }.donut strong { display: block; font: 500 21px 'DM Mono', monospace; }.donut span { display: block; margin-top: 4px; max-width: 72px; color: var(--subtle); font-size: 10px; line-height: 1.4; }.error-legend { flex: 1; }.error-legend > div { display: grid; grid-template-columns: 9px 45px 1fr; align-items: center; gap: 7px; margin: 12px 0; font-size: 11px; }.error-legend strong { font: 500 14px 'DM Mono', monospace; }.error-legend > div span:last-child { color: var(--subtle); }.dot { width: 7px; height: 7px; border-radius: 50%; }.dot.blue { background: var(--blue); }.dot.red { background: var(--red); }.error-legend p { display: flex; gap: 6px; margin: 18px 0 0; color: var(--bronze); font-size: 10px; line-height: 1.4; }.exact-table, .mapping-table, .recent-table { width: 100%; border-collapse: collapse; font-size: 11px; }.exact-table caption, .mapping-table caption, .recent-table caption { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0); }.exact-table td { padding: 9px 0; border-top: 1px solid var(--border); }.exact-table td:nth-child(n+2) { text-align: right; font-family: 'DM Mono', monospace; }.exact-table td:last-child { color: var(--subtle); }.coverage-bars { margin-top: 21px; }.bar-row { margin: 14px 0; }.bar-label, .error-bar-meta { display: flex; justify-content: space-between; gap: 10px; font-size: 11px; }.bar-label strong, .error-bar-meta b { font: 500 11px 'DM Mono', monospace; }.track, .error-track { height: 7px; margin-top: 7px; background: color-mix(in oklch, var(--border) 60%, transparent); }.track span, .error-track span { display: block; height: 100%; }.mapping-panel, .errors-panel { min-height: 320px; }.panel-note { margin: 14px 0 9px; color: var(--subtle); font-size: 10px; }.segmented { display: flex; border: 1px solid var(--border); }.segmented button { padding: 6px 8px; border: 0; border-right: 1px solid var(--border); background: transparent; color: var(--subtle); font-size: 10px; }.segmented button:last-child { border: 0; }.segmented button.selected { color: var(--text); background: color-mix(in oklch, var(--bronze) 10%, transparent); }.mapping-table { margin-top: 14px; }.mapping-table th, .mapping-table td, .recent-table th, .recent-table td { padding: 11px 7px; text-align: left; border-top: 1px solid var(--border); }.mapping-table th, .recent-table th { color: var(--subtle); font: 500 9px 'DM Mono', monospace; text-transform: uppercase; letter-spacing: 0.07em; }.mapping-table td:nth-child(n+3) { font-family: 'DM Mono', monospace; }.mapping-table td:last-child { display: flex; align-items: center; gap: 5px; color: var(--green); font-family: inherit; }.mapping-table tr { cursor: pointer; }.mapping-table tr:hover, .mapping-table tr.selected-row { background: color-mix(in oklch, var(--bronze) 7%, transparent); }.term-mark { margin-right: 4px; color: var(--bronze); }.warning-text { color: var(--orange); }.error-bars { margin: 23px 0 20px; }.error-bar-row { margin: 17px 0; }.error-bar-meta span:last-child { color: var(--subtle); }.error-bar-meta em { margin-left: 6px; color: var(--orange); font-style: normal; font-size: 10px; }.error-track span { background: var(--red); }.source-footer { flex-wrap: wrap; gap: 16px; padding-top: 14px; border-top: 1px solid var(--border); color: var(--subtle); font-size: 10px; }.source-footer a { display: inline-flex; align-items: center; gap: 4px; margin-left: auto; color: var(--bronze); text-decoration: none; }.source-footer b { color: var(--text); font: 500 11px 'DM Mono', monospace; }
	.recent-section { margin-top: 32px; }.section-heading { display: flex; justify-content: space-between; gap: 20px; align-items: end; margin-bottom: 13px; }.table-tools { gap: 8px; }.search-field { display: flex; gap: 7px; align-items: center; padding: 8px 10px; border: 1px solid var(--border); color: var(--subtle); background: var(--surface); }.search-field input { width: 200px; border: 0; outline: 0; background: transparent; color: var(--text); font-size: 11px; }.table-wrap { overflow-x: auto; border: 1px solid var(--border); background: var(--surface); box-shadow: var(--shadow); }.recent-table { min-width: 780px; }.recent-table th, .recent-table td { padding: 13px 15px; }.recent-table td { color: var(--subtle); }.recent-table td strong { display: block; color: var(--text); font-weight: 600; }.recent-table td small { display: block; margin-top: 4px; font-size: 10px; color: var(--subtle); }.mono { color: var(--bronze); font: 500 11px 'DM Mono', monospace; }.definition-chip { color: var(--text); }.status-badge { display: inline-flex; align-items: center; padding: 4px 7px; border: 1px solid currentColor; font: 500 10px 'DM Mono', monospace; white-space: nowrap; }.status-badge.positive { color: var(--green); }.status-badge.warning { color: var(--orange); }.status-badge.neutral { color: var(--slate); }.status-badge.blocked { color: var(--red); }.status-badge.error { color: var(--red); }.empty-row { text-align: center !important; padding: 30px !important; }.page-footer { justify-content: space-between; gap: 20px; margin-top: 21px; color: var(--subtle); font-size: 10px; }.page-footer span { display: flex; gap: 6px; align-items: center; }.footer-link { padding: 0; border: 0; background: none; color: var(--bronze); cursor: pointer; font-size: 10px; }
	.variant-signal .panel, .variant-signal .kpi { border-radius: 8px; }.variant-signal .content { max-width: 1560px; }.variant-signal .kpi strong { font-size: 31px; }.variant-signal .panel-head h2 { font-size: 15px; }.variant-signal .mode-tabs a.active { background: color-mix(in oklch, var(--bronze) 9%, transparent); }.variant-field .content { max-width: 1280px; }.variant-field .panel, .variant-field .kpi, .variant-field .table-wrap { box-shadow: none; border-radius: 0; }.variant-field .kpi-grid { grid-template-columns: repeat(3, 1fr); }.variant-field .kpi { min-height: 116px; }.variant-field .kpi strong { margin: 18px 0 12px; }.variant-field .dashboard-grid { gap: 22px; }.variant-field .panel { padding: 26px 0; border-width: 1px 0; background: transparent; }.variant-field .scope-bar { margin-top: 19px; }.variant-field .recent-table, .variant-field .mapping-table { background: var(--surface); }
	@media (max-width: 1050px) { .kpi-grid { grid-template-columns: repeat(3, 1fr); }.title-row { align-items: start; flex-direction: column; }.title-actions { align-self: stretch; }.model-layout { grid-template-columns: 1fr; grid-template-areas: "graph" "terms" "assertions" "evidence"; } }
	@media (max-width: 700px) { .topbar { padding: 0 18px; }.content { padding: 25px 18px 24px; }.title-row h1 { font-size: 30px; }.title-actions { width: 100%; }.title-actions button { flex: 1; justify-content: center; }.mode-tabs { overflow-x: auto; white-space: nowrap; }.coverage-notice { align-items: start; }.notice-detail { margin-left: auto; }.notice-expanded { padding-left: 0; flex-direction: column; gap: 7px; }.variant-picker { overflow-x: auto; white-space: nowrap; padding-bottom: 4px; }.variant-picker > span { display: none; }.scope-bar { overflow-x: auto; white-space: nowrap; }.scope-label { flex: 0 0 auto; }.scope-select, .scope-chip { flex: 0 0 auto; }.reset-button { margin-left: 0; }.kpi-grid, .variant-field .kpi-grid { grid-template-columns: repeat(2, 1fr); }.kpi { min-height: 118px; padding: 14px 12px; }.kpi strong { font-size: 23px; margin: 21px 0 13px; }.error-visual { align-items: start; flex-direction: column; }.donut { width: 132px; height: 132px; }.donut::before { width: 96px; height: 96px; }.section-heading { align-items: start; flex-direction: column; }.table-tools { width: 100%; }.search-field { flex: 1; }.search-field input { width: 100%; }.page-footer { align-items: start; flex-direction: column; }.page-footer span:last-child { flex-wrap: wrap; } }
	.donut { position: relative; }
	.empty-state { padding: 24px 0; color: var(--subtle); font: 500 10px 'DM Mono', monospace; text-align: center; }.empty-state-panel { min-height: 140px; display: grid; place-items: center; border: 1px dashed var(--border); background: color-mix(in oklch, var(--surface) 84%, var(--bronze) 16%); }.empty-inline { margin-left: 10px; color: var(--subtle); font: 500 10px 'DM Mono', monospace; letter-spacing: 0; }
	@media (prefers-reduced-motion: reduce) { *, *::before, *::after { transition-duration: 0.01ms !important; animation-duration: 0.01ms !important; } }
</style>
