<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import {
		AlertTriangle,
		ArrowLeft,
		BookOpen,
		Check,
		ChevronDown,
		ChevronRight,
		CircleHelp,
		Clock3,
		Columns3,
		Database,
		FileText,
		Filter,
		Info,
		Layers3,
		ListFilter,
		Search,
		SlidersHorizontal,
		X
	} from '@lucide/svelte';

	type RowState = 'complete' | 'findings' | 'historical' | 'blocked' | 'incomplete' | 'failed';
	type MetricRow = {
		id: string;
		document: string;
		section: string;
		metric: string;
		value: string;
		normalized: string;
		definition: string | null;
		definitionMeta: string;
		subject: string;
		assertion: string;
		className: string;
		state: RowState;
		finding: string;
		processed: string;
	};

	let darkMode = $state(false);
	let filterOpen = $state(false);
	let columnsOpen = $state(false);
	let query = $state('');
	let scope = $state(m.document_metrics_analysis_all_authorized_documents());
	let selectedStatus = $state(m.document_metrics_analysis_all_statuses());
	let selectedRow = $state<string | null>(null);

	const rows: MetricRow[] = [
		{ id: 'occ-00841', document: '泵站维护规程', section: '§4.2 · 入口管路', metric: '入口压力', value: '0.42 MPa', normalized: '420 kPa', definition: 'pressure', definitionMeta: 'physical-quantity · current', subject: '冷却泵 P-04', assertion: 'A-19F2', className: 'OperatingMetric', state: 'findings', finding: '2 findings', processed: '12 min ago' },
		{ id: 'occ-00840', document: '冷却系统验收', section: '§2.1 · 性能指标', metric: '出口温度', value: '72 °C', normalized: '345.15 K', definition: 'temperature', definitionMeta: 'physical-quantity · current', subject: '冷却回路', assertion: 'A-19E8', className: 'OperatingMetric', state: 'complete', finding: 'No findings', processed: '18 min ago' },
		{ id: 'occ-00839', document: '流量计校准记录', section: '§7 · 额定点', metric: '额定流量', value: '1,200 L/min', normalized: '0.02 m³/s', definition: 'flow rate', definitionMeta: 'process-measure · current', subject: '流量计 FT-12', assertion: 'Not present', className: 'Not resolved', state: 'historical', finding: 'Historical', processed: '31 min ago' },
		{ id: 'occ-00838', document: '风机技术条件', section: '§5.3 · 可靠性', metric: '设计寿命', value: '20,000 h', normalized: '20,000 h', definition: 'service life', definitionMeta: 'reliability · current', subject: '送风机 F-02', assertion: 'Blocked claim', className: 'Not resolved', state: 'blocked', finding: 'Rejected candidate', processed: '44 min ago' },
		{ id: 'occ-00837', document: '储罐设计规范', section: '§6.4 · 壁厚计算', metric: '最小壁厚', value: '8 mm', normalized: '8 mm', definition: null, definitionMeta: 'No governed term', subject: '储罐 T-07', assertion: 'Not present', className: 'Not present', state: 'incomplete', finding: 'Silent gap', processed: '1 h ago' },
		{ id: 'occ-00836', document: '压缩机运行手册', section: '§3.8 · 保护值', metric: '排气压力上限', value: '1.6 MPa', normalized: '—', definition: 'pressure', definitionMeta: 'physical-quantity · current', subject: '压缩机 C-01', assertion: 'Not present', className: 'Not resolved', state: 'failed', finding: 'Processor timeout', processed: '2 h ago' }
	];

	const visibleColumns = [
		{ id: 'value', label: m.document_metrics_analysis_raw_normalized_value_2(), checked: true },
		{ id: 'definition', label: m.document_metrics_analysis_definition_vocabulary(), checked: true },
		{ id: 'subject', label: m.document_metrics_analysis_subject(), checked: true },
		{ id: 'assertion', label: m.document_metrics_analysis_assertion_class_2(), checked: true },
		{ id: 'processed', label: m.document_metrics_analysis_last_processed(), checked: true },
		{ id: 'identity', label: m.document_metrics_analysis_stable_occurrence_id(), checked: false },
		{ id: 'evidence', label: m.document_metrics_analysis_evidence_span(), checked: false }
	];

	const stateMeta: Record<RowState, { label: string; className: string; icon: typeof Check }> = {
		complete: { label: m.document_metrics_analysis_complete_2(), className: 'positive', icon: Check },
		findings: { label: m.document_metrics_analysis_findings_2(), className: 'warning', icon: AlertTriangle },
		historical: { label: m.document_metrics_analysis_historical_2(), className: 'neutral', icon: Clock3 },
		blocked: { label: m.document_metrics_analysis_blocked_claim(), className: 'blocked', icon: AlertTriangle },
		incomplete: { label: m.document_metrics_analysis_incomplete_graph(), className: 'warning', icon: AlertTriangle },
		failed: { label: m.document_metrics_analysis_execution_failed(), className: 'error', icon: AlertTriangle }
	};

	let filteredRows = $derived(rows.filter((row) => {
		const haystack = `${row.document} ${row.section} ${row.metric} ${row.definition ?? ''} ${row.subject}`.toLowerCase();
		const matchesQuery = haystack.includes(query.toLowerCase());
		const matchesStatus = selectedStatus === m.document_metrics_analysis_all_statuses() || stateMeta[row.state].label.startsWith(selectedStatus);
		return matchesQuery && matchesStatus;
	}));

	function resetFilters() {
		query = '';
		scope = m.document_metrics_analysis_all_authorized_documents();
		selectedStatus = m.document_metrics_analysis_all_statuses();
	}
</script>

<svelte:head>
	<title>{m.document_metrics_analysis_document_metrics_metric_ontology_analysis()}</title>
	<meta name="description" content="Occurrence-first document metric analysis." />
</svelte:head>

<div class:dark={darkMode} class="analysis-page">
	<header class="topbar">
		<div class="brand-lockup">
			<div class="brand-mark">{m.document_metrics_analysis_ch()}</div>
			<div><p class="eyebrow">{m.document_metrics_analysis_chenweb_home3()}</p><p class="brand-name">{m.document_metrics_analysis_knowledge_store()}</p></div>
		</div>
		<div class="top-actions">
			<span class="read-only"><Database size={13} /> {m.document_metrics_analysis_read_only_analysis()}</span>
			<button class="icon-button" aria-label={m.document_metrics_analysis_toggle_theme()} onclick={() => (darkMode = !darkMode)}>{darkMode ? '☼' : '☾'}</button>
			<div class="avatar">{m.document_metrics_analysis_cd()}</div>
		</div>
	</header>

	<main class="content">
		<div class="breadcrumb"><a href="/home3/ontology-metric-analysis"><ArrowLeft size={13} /> {m.document_metrics_analysis_metric_analysis()}</a><span>/</span><strong>{m.document_metrics_analysis_document_metrics()}</strong></div>
		<div class="title-row">
			<div><p class="section-kicker">{m.document_metrics_analysis_02_occurrence_first_view()}</p><h1>{m.document_metrics_analysis_document_metrics()}</h1><p class="lede">{m.document_metrics_analysis_every_authorized_metric_occurrence_including()}</p></div>
			<div class="title-actions"><button class="quiet-button"><Database size={15} /> {m.document_metrics_analysis_inspect_data()}</button><button class="quiet-button"><Info size={15} /> {m.document_metrics_analysis_read_the_grain()}</button></div>
		</div>

		<nav class="mode-tabs" aria-label={m.document_metrics_analysis_metric_analysis_view()}>
			<a href="/home3/ontology-metric-analysis">{m.document_metrics_analysis_dashboard()} <span>01</span></a>
			<a class="active" href="/home3/ontology-metric-analysis?view=document">{m.document_metrics_analysis_document_metrics()} <span>02</span></a>
			<a href="/home3/ontology-metric-analysis?view=ontology">{m.document_metrics_analysis_ontology_metrics()} <span>03</span></a>
		</nav>

		<section class="coverage-notice">
			<div class="notice-icon"><Check size={15} /></div>
			<div class="notice-copy"><strong>{m.document_metrics_analysis_coverage_is_current()}</strong><span>{m.document_metrics_analysis_writer_v3_8_2_authorized()}</span></div>
			<button class="notice-detail">{m.document_metrics_analysis_view_definition()} <ChevronDown size={14} /></button>
		</section>

		<section class="scope-bar" aria-label={m.document_metrics_analysis_active_scope()}>
			<div class="scope-label"><Filter size={15} /><span>{m.document_metrics_analysis_scope()}</span></div>
			<button class="scope-select" onclick={() => (scope = scope === m.document_metrics_analysis_all_authorized_documents() ? m.document_metrics_analysis_standards_2026_intake() : m.document_metrics_analysis_all_authorized_documents())}>{scope} <ChevronDown size={14} /></button>
			<span class="scope-chip"><BookOpen size={13} /> {m.document_metrics_analysis_document()} <strong>{m.document_metrics_analysis_all()}</strong></span>
			<span class="scope-chip"><Layers3 size={13} /> {m.document_metrics_analysis_definition()} <strong>{m.document_metrics_analysis_all_terms()}</strong></span>
			<span class="filter-count"><ListFilter size={13} /> {m.document_metrics_analysis_0_filters()}</span>
			<button class="reset-button" onclick={resetFilters}>{m.document_metrics_analysis_reset_filters()}</button>
		</section>

		<section class="coverage-strip" aria-label={m.document_metrics_analysis_document_metric_coverage_summary()}>
			<div class="coverage-title"><p class="panel-kicker">{m.document_metrics_analysis_coverage_strip()}</p><strong>1,245</strong><span>{m.document_metrics_analysis_authorized_occurrences()}</span></div>
			<div class="coverage-item"><span class="coverage-dot green"></span><strong>684</strong><span>{m.document_metrics_analysis_complete()}</span></div>
			<div class="coverage-item"><span class="coverage-dot bronze"></span><strong>216</strong><span>{m.document_metrics_analysis_with_findings()}</span></div>
			<div class="coverage-item"><span class="coverage-dot slate"></span><strong>168</strong><span>{m.document_metrics_analysis_historical()}</span></div>
			<div class="coverage-item"><span class="coverage-dot red"></span><strong>177</strong><span>{m.document_metrics_analysis_needs_attention()}</span></div>
			<div class="coverage-tail"><span>{m.document_metrics_analysis_coverage_denominator()}</span><b>{m.document_metrics_analysis_1_245_occurrences()}</b><CircleHelp size={13} /></div>
		</section>

		<section class="table-section">
			<div class="section-heading">
				<div><p class="panel-kicker">{m.document_metrics_analysis_current_occurrence_inventory()}</p><h2>{m.document_metrics_analysis_document_metrics()} <span>{m.document_metrics_analysis_1_245_occurrences_page_1()}</span></h2></div>
			<div class="table-tools"><label class="search-field"><Search size={15} /><input bind:value={query} placeholder={m.document_metrics_analysis_search_document_metric_subject()} aria-label={m.document_metrics_analysis_search_document_metric_subject()} /></label><button class="tool-button" class:active={filterOpen} onclick={() => (filterOpen = !filterOpen)}><SlidersHorizontal size={15} /> {m.document_metrics_analysis_filters()}</button><button class="tool-button" class:active={columnsOpen} onclick={() => (columnsOpen = !columnsOpen)}><Columns3 size={15} /> {m.document_metrics_analysis_columns()}</button></div>
			</div>

			{#if filterOpen}
				<div class="filter-drawer">
					<div class="drawer-head"><div><p class="panel-kicker">{m.document_metrics_analysis_server_recognized_filters()}</p><strong>{m.document_metrics_analysis_refine_the_occurrence_set()}</strong></div><button class="close-button" aria-label={m.document_metrics_analysis_close_filters()} onclick={() => (filterOpen = false)}><X size={16} /></button></div>
					<div class="filter-grid">
						<label><span>{m.document_metrics_analysis_source_document()}</span><select><option>{m.document_metrics_analysis_all_documents()}</option><option>{m.document_metrics_analysis_standards_2026_intake()}</option></select></label>
						<label><span>{m.document_metrics_analysis_governed_term_status()}</span><select><option>{m.document_metrics_analysis_all_definitions()}</option><option>{m.document_metrics_analysis_missing_definition()}</option><option>{m.document_metrics_analysis_current_definition()}</option></select></label>
						<label><span>{m.document_metrics_analysis_coverage_execution()}</span><select bind:value={selectedStatus}><option>{m.document_metrics_analysis_all_statuses()}</option><option>{m.document_metrics_analysis_complete_2()}</option><option>{m.document_metrics_analysis_findings()}</option><option>{m.document_metrics_analysis_historical_2()}</option><option>{m.document_metrics_analysis_blocked_claim()}</option><option>{m.document_metrics_analysis_incomplete_graph()}</option><option>{m.document_metrics_analysis_execution_failed()}</option></select></label>
						<label><span>{m.document_metrics_analysis_failed_check()}</span><select><option>{m.document_metrics_analysis_all_checks()}</option><option>{m.document_metrics_analysis_silent_gap()}</option><option>{m.document_metrics_analysis_missing_assertion_edge()}</option></select></label>
					</div>
					<div class="drawer-foot"><span><Info size={14} /> {m.document_metrics_analysis_filters_apply_to_the_server()}</span><button class="reset-button" onclick={resetFilters}>{m.document_metrics_analysis_clear_filters()}</button></div>
				</div>
			{/if}

			{#if columnsOpen}
				<div class="columns-popover"><div class="drawer-head"><strong>{m.document_metrics_analysis_visible_columns()}</strong><button class="close-button" onclick={() => (columnsOpen = false)}><X size={15} /></button></div>{#each visibleColumns as column}<label><input type="checkbox" checked={column.checked} /> <span>{column.label}</span></label>{/each}</div>
			{/if}

			<div class="table-meta"><span><strong>{filteredRows.length}</strong> {m.document_metrics_analysis_visible_in_this_design_preview()}</span><span>{m.document_metrics_analysis_page_size()} <select><option>50</option><option>100</option><option>200</option></select><button class="meta-link">{m.document_metrics_analysis_save_view()}</button></span></div>
			<div class="table-wrap">
				<table class="metric-table"><caption>{m.document_metrics_analysis_document_metric_occurrences()}</caption><thead><tr><th class="identity-head">{m.document_metrics_analysis_document_metric()}</th><th>{m.document_metrics_analysis_raw_normalized_value()}</th><th>{m.document_metrics_analysis_definition()}</th><th>{m.document_metrics_analysis_subject()}</th><th>{m.document_metrics_analysis_assertion_class()}</th><th>{m.document_metrics_analysis_health()}</th><th>{m.document_metrics_analysis_last_processed()}</th><th class="action-head"></th></tr></thead>
				<tbody>{#each filteredRows as row}{@const StateIcon = stateMeta[row.state].icon}<tr class:selected={selectedRow === row.id} onclick={() => (selectedRow = selectedRow === row.id ? null : row.id)} onkeydown={(event) => event.key === 'Enter' && (selectedRow = row.id)} tabindex="0">
					<td class="identity-cell"><div class="doc-name"><FileText size={15} /><div><strong>{row.metric}</strong><span>{row.document} <i>{row.section}</i></span></div></div><code>{row.id}</code></td>
					<td class="value-cell"><strong>{row.value}</strong><span>{row.normalized}</span></td>
					<td>{#if row.definition}<div class="definition"><strong>{row.definition}</strong><span>{row.definitionMeta}</span></div>{:else}<span class="not-present">{m.document_metrics_analysis_not_present()}</span><small class="reason">{m.document_metrics_analysis_no_governed_term()}</small>{/if}</td>
					<td><strong class="subject">{row.subject}</strong><span class="muted">{m.document_metrics_analysis_record_data()}</span></td>
					<td><div class="assertion"><strong class:blocked-text={row.assertion === 'Blocked claim'}>{row.assertion}</strong><span>{row.className}</span></div></td>
					<td><div class="health"><span class="status-badge {stateMeta[row.state].className}"><StateIcon size={12} />{stateMeta[row.state].label}</span><span class="finding {row.finding === 'No findings' ? 'quiet' : ''}">{row.finding}</span></div></td>
					<td class="processed"><Clock3 size={13} />{row.processed}</td><td class="row-action"><ChevronRight size={17} /></td>
				</tr>{:else}<tr><td colspan="8" class="empty-row"><Search size={22} /><strong>{m.document_metrics_analysis_no_occurrences_match_these_filters()}</strong><span>{m.document_metrics_analysis_clear_filters_or_choose_a()}</span><button class="reset-button" onclick={resetFilters}>{m.document_metrics_analysis_clear_filters()}</button></td></tr>{/each}</tbody></table>
			</div>
			<div class="table-foot"><span><Info size={13} /> {m.document_metrics_analysis_each_row_is_one()} <strong>{m.document_metrics_analysis_kb_metrics()}</strong> {m.document_metrics_analysis_occurrence_a_missing_assertion_remains()}</span><div class="pagination"><button disabled><ChevronRight size={15} style="transform:rotate(180deg)" /></button><span>1 / 25</span><button><ChevronRight size={15} /></button></div></div>
		</section>

		{#if selectedRow}<aside class="selection-note"><div><p class="panel-kicker">{m.document_metrics_analysis_selected_occurrence()}</p><strong>{selectedRow}</strong><span>{m.document_metrics_analysis_detail_route_will_open_with()}</span></div><button onclick={() => (selectedRow = null)}><X size={15} /></button></aside>{/if}
		<footer class="page-footer"><span><Layers3 size={14} /> {m.document_metrics_analysis_metric_ontology_analysis_page_2()}</span><span>{m.document_metrics_analysis_frontend_design_preview_backend_connection()}</span></footer>
	</main>
</div>

<style>
	@import url('https://fonts.googleapis.com/css2?family=DM+Mono:wght@400;500&family=Manrope:wght@400;500;600;700;800&display=swap');
	:global(*) { box-sizing: border-box; }
	:global(body) { margin: 0; font-family: 'Manrope', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; }
	:global(button), :global(input), :global(select) { font: inherit; }
	:global(button:focus-visible), :global(a:focus-visible), :global(input:focus-visible), :global(select:focus-visible) { outline: 3px solid oklch(0.72 0.12 65); outline-offset: 3px; }
	:root { --paper: oklch(0.965 0.014 78); --panel: oklch(0.985 0.009 78); --ink: oklch(0.23 0.025 63); --muted: oklch(0.55 0.025 72); --line: oklch(0.88 0.028 75); --bronze: oklch(0.61 0.09 69); --green: oklch(0.58 0.09 145); --red: oklch(0.58 0.13 26); --slate: oklch(0.57 0.035 245); --orange: oklch(0.68 0.12 52); }
	.analysis-page { --bg: var(--paper); --surface: var(--panel); --text: var(--ink); --subtle: var(--muted); --border: var(--line); --shadow: 0 12px 30px oklch(0.3 0.02 70 / .06); min-height: 100vh; background: var(--bg); color: var(--text); }
	.analysis-page.dark { --bg: #171b26; --surface: #252a3a; --text: #e2e8f0; --subtle: #94a3b8; --border: #2d3348; --shadow: 0 12px 30px rgba(7, 10, 18, 0.34); }
	.topbar { height: 70px; padding: 0 4.5vw; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; background: color-mix(in oklch, var(--surface) 80%, transparent); }.brand-lockup,.top-actions,.breadcrumb,.title-actions,.scope-bar,.coverage-strip,.coverage-item,.table-tools,.table-meta,.table-foot,.page-footer,.drawer-foot,.read-only { display: flex; align-items: center; }.brand-lockup { gap: 11px; }.brand-mark { width: 30px; height: 30px; border: 1px solid var(--bronze); color: var(--bronze); display: grid; place-items: center; font: 500 11px 'DM Mono', monospace; letter-spacing: -.08em; }.eyebrow,.section-kicker,.panel-kicker { margin: 0; text-transform: uppercase; letter-spacing: .13em; font: 500 10px 'DM Mono', monospace; color: var(--bronze); }.brand-name { margin: 2px 0 0; font-size: 13px; font-weight: 700; }.top-actions { gap: 12px; }.read-only { gap: 5px; color: var(--subtle); font: 500 10px 'DM Mono', monospace; }.icon-button,.close-button { border: 0; background: none; color: var(--subtle); cursor: pointer; padding: 7px; }.icon-button { font-size: 17px; }.avatar { width: 30px; height: 30px; border-radius: 50%; display: grid; place-items: center; background: var(--ink); color: var(--paper); font: 500 10px 'DM Mono', monospace; }.content { max-width: 1540px; margin: 0 auto; padding: 33px 4.5vw 28px; }.breadcrumb { gap: 9px; color: var(--subtle); font-size: 12px; }.breadcrumb a { color: var(--subtle); text-decoration: none; display: flex; align-items: center; gap: 5px; }.breadcrumb strong { color: var(--text); }.breadcrumb span { color: var(--bronze); }.title-row { margin: 28px 0 26px; display: flex; justify-content: space-between; align-items: end; gap: 24px; }.title-row h1 { margin: 7px 0 8px; font-size: clamp(28px, 3.5vw, 46px); letter-spacing: -.055em; line-height: 1.02; }.lede { margin: 0; max-width: 680px; color: var(--subtle); font-size: 14px; line-height: 1.65; }.title-actions,.table-tools { gap: 8px; }.quiet-button,.tool-button,.reset-button,.scope-select { display: inline-flex; align-items: center; gap: 7px; border: 1px solid var(--border); background: transparent; color: var(--text); cursor: pointer; font-size: 11px; padding: 9px 12px; }.quiet-button:hover,.tool-button:hover,.tool-button.active { background: color-mix(in oklch, var(--bronze) 8%, transparent); border-color: color-mix(in oklch, var(--bronze) 65%, var(--border)); }.mode-tabs { display: flex; gap: 2px; border-bottom: 1px solid var(--border); }.mode-tabs a { padding: 11px 15px 12px; color: var(--subtle); text-decoration: none; font-size: 12px; border-bottom: 2px solid transparent; }.mode-tabs a.active,.mode-tabs a:hover { color: var(--text); border-color: var(--bronze); }.mode-tabs span { margin-left: 8px; color: var(--bronze); font: 500 10px 'DM Mono', monospace; }.coverage-notice { display: flex; align-items: center; gap: 10px; margin: 17px 0 11px; padding: 10px 12px; border: 1px solid var(--border); background: color-mix(in oklch, var(--surface) 55%, var(--bronze) 4%); font-size: 11px; }.notice-icon { width: 21px; height: 21px; border: 1px solid var(--green); color: var(--green); display: grid; place-items: center; border-radius: 50%; }.notice-copy { display: flex; gap: 9px; flex-wrap: wrap; }.notice-copy span { color: var(--subtle); }.notice-detail { margin-left: auto; border: 0; background: none; color: var(--bronze); font-size: 11px; cursor: pointer; }.scope-bar { gap: 9px; padding: 12px 0; border-bottom: 1px solid var(--border); overflow-x: auto; white-space: nowrap; }.scope-label { display: flex; align-items: center; gap: 6px; color: var(--subtle); font: 500 10px 'DM Mono', monospace; text-transform: uppercase; letter-spacing: .09em; }.scope-select { border: 0; padding-left: 4px; padding-right: 4px; }.scope-chip { gap: 5px; padding: 8px 10px; border: 1px solid var(--border); color: var(--subtle); font-size: 10px; }.scope-chip strong { color: var(--text); font-weight: 600; }.filter-count { display: flex; gap: 5px; align-items: center; color: var(--subtle); font-size: 10px; }.reset-button { border: 0; color: var(--bronze); padding-left: 5px; padding-right: 5px; }.coverage-strip { gap: 24px; margin: 20px 0 27px; padding: 17px 18px; border: 1px solid var(--border); background: var(--surface); box-shadow: var(--shadow); }.coverage-title { min-width: 205px; padding-right: 24px; border-right: 1px solid var(--border); }.coverage-title strong { display: block; margin-top: 6px; font: 500 26px 'DM Mono', monospace; }.coverage-title span { color: var(--subtle); font-size: 10px; }.coverage-item { gap: 7px; color: var(--subtle); font-size: 10px; white-space: nowrap; }.coverage-item strong { color: var(--text); font: 500 16px 'DM Mono', monospace; }.coverage-dot { width: 7px; height: 7px; border-radius: 50%; }.green { background: var(--green); }.bronze { background: var(--bronze); }.slate { background: var(--slate); }.red { background: var(--red); }.coverage-tail { margin-left: auto; padding-left: 24px; border-left: 1px solid var(--border); color: var(--subtle); font-size: 10px; }.coverage-tail b { display: block; color: var(--text); font: 500 10px 'DM Mono', monospace; margin-top: 5px; }.coverage-tail :global(svg) { margin-left: 3px; vertical-align: -2px; }.section-heading { display: flex; align-items: end; justify-content: space-between; gap: 20px; margin-bottom: 13px; }.section-heading h2 { margin: 6px 0 0; font-size: 18px; letter-spacing: -.03em; }.section-heading h2 span { margin-left: 8px; color: var(--subtle); font: 500 10px 'DM Mono', monospace; letter-spacing: 0; }.search-field { display: flex; align-items: center; gap: 7px; padding: 8px 10px; border: 1px solid var(--border); color: var(--subtle); background: var(--surface); }.search-field input { width: 220px; border: 0; outline: 0; background: transparent; color: var(--text); font-size: 11px; }.filter-drawer { margin-bottom: 12px; padding: 16px; border: 1px solid var(--border); background: color-mix(in oklch, var(--surface) 90%, var(--bronze) 5%); }.drawer-head { display: flex; justify-content: space-between; align-items: start; gap: 20px; }.drawer-head strong { display: block; margin-top: 5px; font-size: 14px; }.filter-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin: 16px 0; }.filter-grid label { display: grid; gap: 7px; color: var(--subtle); font-size: 10px; }.filter-grid select { width: 100%; border: 1px solid var(--border); background: var(--surface); color: var(--text); padding: 9px; font-size: 11px; }.drawer-foot { justify-content: space-between; gap: 15px; padding-top: 12px; border-top: 1px solid var(--border); color: var(--subtle); font-size: 10px; }.drawer-foot span { display: flex; align-items: center; gap: 6px; }.columns-popover { position: absolute; z-index: 4; right: 4.5vw; width: 260px; padding: 15px; border: 1px solid var(--border); background: var(--surface); box-shadow: var(--shadow); }.columns-popover label { display: flex; gap: 8px; align-items: center; padding: 9px 0; color: var(--subtle); font-size: 11px; }.columns-popover input { accent-color: var(--bronze); }.table-meta { justify-content: space-between; padding: 10px 0; color: var(--subtle); font-size: 10px; }.table-meta strong { color: var(--text); }.table-meta select { border: 0; color: var(--text); background: transparent; font-size: 10px; }.meta-link { margin-left: 12px; border: 0; background: none; color: var(--bronze); font-size: 10px; cursor: pointer; }.table-wrap { overflow-x: auto; border: 1px solid var(--border); background: var(--surface); box-shadow: var(--shadow); }.metric-table { width: 100%; min-width: 1160px; border-collapse: separate; border-spacing: 0; font-size: 11px; }.metric-table th,.metric-table td { padding: 13px 13px; text-align: left; border-bottom: 1px solid var(--border); vertical-align: middle; }.metric-table th { position: sticky; top: 0; z-index: 2; color: var(--subtle); background: color-mix(in oklch, var(--surface) 92%, var(--bronze) 4%); font: 500 9px 'DM Mono', monospace; text-transform: uppercase; letter-spacing: .07em; white-space: nowrap; }.metric-table tbody tr { cursor: pointer; transition: background 140ms ease; }.metric-table tbody tr:hover,.metric-table tbody tr.selected { background: color-mix(in oklch, var(--bronze) 8%, transparent); }.metric-table tbody tr:last-child td { border-bottom: 0; }.identity-head,.identity-cell { position: sticky; left: 0; z-index: 1; background: var(--surface); }.metric-table th.identity-head { z-index: 3; }.metric-table tbody tr:hover .identity-cell,.metric-table tbody tr.selected .identity-cell { background: color-mix(in oklch, var(--surface) 92%, var(--bronze) 8%); }.doc-name { display: flex; gap: 8px; align-items: start; }.doc-name :global(svg) { flex: 0 0 auto; margin-top: 1px; color: var(--bronze); }.doc-name strong { display: block; color: var(--text); font-weight: 600; }.doc-name span,.definition span,.assertion span { display: block; margin-top: 4px; color: var(--subtle); font-size: 10px; white-space: nowrap; }.doc-name i { margin-left: 4px; color: var(--bronze); font-style: normal; }.identity-cell code { display: block; margin: 8px 0 0 23px; color: var(--bronze); font: 500 9px 'DM Mono', monospace; }.value-cell strong,.subject,.definition strong,.assertion strong { color: var(--text); font-weight: 600; }.value-cell span,.muted { display: block; margin-top: 4px; color: var(--subtle); font: 500 10px 'DM Mono', monospace; }.definition strong { font: 600 11px 'Manrope', sans-serif; }.not-present { display: block; color: var(--text); }.reason { display: block; margin-top: 4px; color: var(--orange); font-size: 10px; }.blocked-text { color: var(--red) !important; }.health { display: grid; gap: 6px; }.status-badge { width: max-content; display: inline-flex; align-items: center; gap: 5px; padding: 4px 6px; border: 1px solid currentColor; font: 500 9px 'DM Mono', monospace; white-space: nowrap; }.status-badge.positive { color: var(--green); }.status-badge.warning { color: var(--orange); }.status-badge.neutral { color: var(--slate); }.status-badge.blocked,.status-badge.error { color: var(--red); }.finding { color: var(--orange); font-size: 10px; }.finding.quiet { color: var(--subtle); }.processed { display: flex; align-items: center; gap: 6px; color: var(--subtle); font-size: 10px; white-space: nowrap; }.row-action { color: var(--subtle); }.empty-row { height: 210px; text-align: center !important; }.empty-row :global(svg),.empty-row strong,.empty-row span { display: block; margin: 0 auto 8px; }.empty-row :global(svg) { color: var(--bronze); }.empty-row strong { color: var(--text); }.empty-row span { color: var(--subtle); }.table-foot { justify-content: space-between; gap: 20px; padding: 13px 0; color: var(--subtle); font-size: 10px; }.table-foot span:first-child { display: flex; align-items: center; gap: 5px; }.table-foot strong { color: var(--text); font-weight: 600; }.pagination { display: flex; align-items: center; gap: 9px; }.pagination button { display: grid; place-items: center; width: 27px; height: 27px; border: 1px solid var(--border); background: transparent; color: var(--text); cursor: pointer; }.pagination button:disabled { opacity: .4; cursor: not-allowed; }.selection-note { display: flex; justify-content: space-between; margin-top: 15px; padding: 14px 16px; border: 1px solid var(--bronze); background: color-mix(in oklch, var(--bronze) 7%, var(--surface)); }.selection-note strong,.selection-note span { display: block; margin-top: 5px; }.selection-note strong { font: 500 11px 'DM Mono', monospace; color: var(--bronze); }.selection-note span { color: var(--subtle); font-size: 10px; }.selection-note button { align-self: start; border: 0; background: none; color: var(--subtle); cursor: pointer; }.page-footer { justify-content: space-between; gap: 20px; margin-top: 21px; color: var(--subtle); font-size: 10px; }.page-footer span { display: flex; gap: 6px; align-items: center; }
	@media (max-width: 1000px) { .title-row { align-items: start; flex-direction: column; }.title-actions { width: 100%; }.coverage-strip { flex-wrap: wrap; gap: 14px; }.coverage-title { min-width: 180px; }.coverage-tail { margin-left: 0; padding-left: 0; border-left: 0; }.filter-grid { grid-template-columns: repeat(2, 1fr); }.section-heading { align-items: start; flex-direction: column; }.table-tools { width: 100%; }.search-field { flex: 1; }.search-field input { width: 100%; }}
	@media (max-width: 620px) { .topbar { padding: 0 18px; }.read-only { display: none; }.content { padding: 25px 18px 24px; }.title-row h1 { font-size: 31px; }.title-actions button { flex: 1; justify-content: center; }.mode-tabs { overflow-x: auto; white-space: nowrap; }.coverage-notice { align-items: start; }.notice-detail { margin-left: auto; }.scope-bar { margin-left: -18px; margin-right: -18px; padding-left: 18px; padding-right: 18px; }.coverage-strip { align-items: start; padding: 14px; }.coverage-title { width: 100%; border-right: 0; border-bottom: 1px solid var(--border); padding: 0 0 12px; }.coverage-item { min-width: calc(50% - 10px); }.filter-grid { grid-template-columns: 1fr; }.drawer-foot { align-items: start; flex-direction: column; }.table-meta { align-items: start; flex-direction: column; gap: 8px; }.page-footer { align-items: start; flex-direction: column; }.columns-popover { right: 18px; }.table-foot { align-items: start; flex-direction: column; }}
	@media (prefers-reduced-motion: reduce) { *,*::before,*::after { transition-duration: .01ms !important; animation-duration: .01ms !important; }}
</style>
