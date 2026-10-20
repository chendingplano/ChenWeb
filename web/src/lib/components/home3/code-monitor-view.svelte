<script lang="ts">
	import { onMount } from 'svelte';
	import { Chart } from 'svelte-echarts';
	import type { EChartsOption } from 'echarts';
	import { init, use } from 'echarts/core';
	import { BarChart, LineChart } from 'echarts/charts';
	import { AriaComponent, GridComponent, TooltipComponent } from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';
	import { m } from '$lib/paraglide/messages.js';
	import { createLatestCodeMonitorLoader, type CodeMonitorPackage, type CodeMonitorRange } from './code-monitor-client';

	use([BarChart, LineChart, AriaComponent, GridComponent, TooltipComponent, CanvasRenderer]);

	let { darkMode = true }: { darkMode?: boolean } = $props();
	const ranges: { value: CodeMonitorRange; label: () => string }[] = [
		{ value: 'today', label: m.code_monitor_today },
		{ value: 'yesterday', label: m.code_monitor_yesterday },
		{ value: 'last_7_days', label: m.code_monitor_last_7_days },
		{ value: 'last_month', label: m.code_monitor_last_month },
		{ value: 'last_6_months', label: m.code_monitor_last_6_months },
		{ value: 'last_12_months', label: m.code_monitor_last_12_months }
	];
	let range = $state<CodeMonitorRange>('last_7_days');
	let packages = $state<CodeMonitorPackage[]>([]);
	let loading = $state(true);
	let error = $state('');
	const loader = createLatestCodeMonitorLoader();
	const colors = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#FFFFFF',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#6B7280',
		border: darkMode ? '#2D3348' : '#E4E6EB',
		accent: darkMode ? '#A5B4FC' : '#4F46E5'
	});

	async function load() {
		loading = true;
		error = '';
		const result = await loader.load(range);
		if (result.status === 'stale') return;
		if (result.status === 'success') {
			packages = result.response.packages;
		} else {
			error = m.code_monitor_load_error();
		}
		loading = false;
	}

	function chartOption(pkg: CodeMonitorPackage, metric: 'total_lines' | 'frontend_files' | 'backend_files', name: string): EChartsOption {
		const rows = [...pkg.rows].sort((a, b) => a.collection_date.localeCompare(b.collection_date));
		const dates = rows.map((row) => row.collection_date);
		const values = rows.map((row) => row[metric]);
		return {
			backgroundColor: 'transparent',
			aria: { enabled: true, label: { description: m.code_monitor_chart_description({ metric: name, package: pkg.package_path }) } },
			textStyle: { color: colors.muted },
			tooltip: { trigger: 'axis' },
			grid: { left: 48, right: 18, top: 24, bottom: 36 },
			xAxis: { type: 'category', data: dates, axisLabel: { color: colors.muted } },
			yAxis: { type: 'value', axisLabel: { color: colors.muted }, splitLine: { lineStyle: { color: colors.border } } },
			series: [
				{ name, type: 'bar', data: values, itemStyle: { color: colors.accent, opacity: 0.62 } },
				{ name, type: 'line', data: values, smooth: false, symbol: 'circle', symbolSize: 5, lineStyle: { color: colors.accent, width: 2 }, itemStyle: { color: colors.accent } }
			]
		};
	}

	onMount(load);
</script>

<div class="page" style={`background:${colors.bg};color:${colors.text};user-select:text`}>
	<header class="header" style={`background:${colors.card};border-color:${colors.border}`}>
		<div>
			<h1>{m.code_monitor_title()}</h1>
			<p>{m.code_monitor_description()}</p>
		</div>
		<div class="ranges" aria-label={m.code_monitor_date_range()}>
			{#each ranges as item (item.value)}
				<button class:active={range === item.value} aria-pressed={range === item.value} onclick={() => { range = item.value; load(); }}>{item.label()}</button>
			{/each}
		</div>
	</header>

	{#if loading}
		<div class="state" style={`color:${colors.muted}`}>{m.code_monitor_loading()}</div>
	{:else if error}
		<div class="state error">{error} <button onclick={load}>{m.code_monitor_retry()}</button></div>
	{:else if packages.length === 0}
		<div class="state" style={`color:${colors.muted}`}>{m.code_monitor_empty()}</div>
	{:else}
		<div class="packages">
			{#each packages as pkg (pkg.package_path)}
				<section class="package" style={`background:${colors.card};border-color:${colors.border}`}>
					<h2>{pkg.package_path}</h2>
					<div class="charts">
						<article><h3>{m.code_monitor_total_lines()}</h3><Chart {init} options={chartOption(pkg, 'total_lines', m.code_monitor_total_lines())} style="height:260px;width:100%" /></article>
						<article><h3>{m.code_monitor_frontend_files()}</h3><Chart {init} options={chartOption(pkg, 'frontend_files', m.code_monitor_frontend_files())} style="height:260px;width:100%" /></article>
						<article><h3>{m.code_monitor_backend_files()}</h3><Chart {init} options={chartOption(pkg, 'backend_files', m.code_monitor_backend_files())} style="height:260px;width:100%" /></article>
					</div>
				</section>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page { min-height: 100%; padding: 24px; display: flex; flex-direction: column; gap: 18px; }
	.header, .package { border: 1px solid; border-radius: 12px; padding: 20px; }
	.header { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 18px; }
	h1, h2, h3, p { margin: 0; }
	h1 { font-size: 1.35rem; font-weight: 650; }
	.header p { margin-top: 5px; font-size: .9rem; color: #94A3B8; }
	.ranges { display: flex; flex-wrap: wrap; gap: 7px; }
	button { border: 1px solid #64748B; border-radius: 7px; padding: 7px 10px; background: transparent; color: inherit; cursor: pointer; }
	button.active { border-color: #818CF8; background: rgba(129,140,248,.18); }
	.state { padding: 32px; text-align: center; }
	.state.error { color: #F87171; }
	.state.error button { margin-left: 8px; }
	.packages { display: flex; flex-direction: column; gap: 18px; }
	h2 { margin-bottom: 18px; font-size: 1.05rem; overflow-wrap: anywhere; }
	.charts { display: grid; grid-template-columns: repeat(auto-fit,minmax(min(100%,320px),1fr)); gap: 16px; }
	article { min-width: 0; }
	h3 { margin: 0 0 8px; font-size: .95rem; font-weight: 600; }
	@media (max-width: 640px) { .page { padding: 14px; } .header { align-items: flex-start; } }
</style>
