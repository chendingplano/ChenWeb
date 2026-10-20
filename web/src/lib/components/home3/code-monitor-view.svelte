<script lang="ts">
	import { onMount } from 'svelte';
	import { Chart } from 'svelte-echarts';
	import type { EChartsOption } from 'echarts';
	import { init, use } from 'echarts/core';
	import { BarChart } from 'echarts/charts';
	import { AriaComponent, GridComponent, TooltipComponent } from 'echarts/components';
	import { CanvasRenderer } from 'echarts/renderers';
	import { m } from '$lib/paraglide/messages.js';
	import { createLatestCodeMonitorLoader, type CodeMonitorProject, type CodeMonitorRange } from './code-monitor-client';

	use([BarChart, AriaComponent, GridComponent, TooltipComponent, CanvasRenderer]);

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
	let projects = $state<CodeMonitorProject[]>([]);
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
			projects = result.response.projects;
		} else {
			error = m.code_monitor_load_error();
		}
		loading = false;
	}

	function chartOption(project: CodeMonitorProject, metric: 'total_lines' | 'frontend_lines_delta' | 'backend_lines_delta', name: string): EChartsOption {
		const rows = [...project.rows].sort((a, b) => a.collection_date.localeCompare(b.collection_date));
		const dates = rows.map((row) => row.collection_date);
		const values = rows.map((row) => row[metric]);
		return {
			backgroundColor: 'transparent',
			aria: { enabled: true, label: { description: m.code_monitor_chart_description({ metric: name, project: project.project_path }) } },
			textStyle: { color: colors.muted },
			tooltip: { trigger: 'axis' },
			grid: { left: 48, right: 16, top: 36, bottom: 36 },
			xAxis: { type: 'category', data: dates, axisLabel: { color: colors.muted } },
			yAxis: { type: 'value', position: 'left', axisLabel: { color: colors.muted }, splitLine: { lineStyle: { color: colors.border } } },
			series: [
				{
					name,
					type: 'bar',
					data: values,
					itemStyle: {
						color: metric === 'total_lines'
							? colors.accent
							: metric === 'frontend_lines_delta'
								? darkMode ? '#34D399' : '#059669'
								: darkMode ? '#FBBF24' : '#D97706',
						opacity: 0.8
					}
				}
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
	{:else if projects.length === 0}
		<div class="state" style={`color:${colors.muted}`}>{m.code_monitor_empty()}</div>
	{:else}
		<div class="projects">
			{#each projects as project (project.project_path)}
				<section class="project" style={`background:${colors.card};border-color:${colors.border}`}>
					<h2>{project.project_path}</h2>
					<div class="charts">
						<article>
							<h3>{m.code_monitor_total_lines()}</h3>
							<Chart {init} options={chartOption(project, 'total_lines', m.code_monitor_total_lines())} style="height:260px;width:100%" />
						</article>
						<article>
							<h3>{m.code_monitor_frontend_delta()}</h3>
							<Chart {init} options={chartOption(project, 'frontend_lines_delta', m.code_monitor_frontend_delta())} style="height:260px;width:100%" />
						</article>
						<article>
							<h3>{m.code_monitor_backend_delta()}</h3>
							<Chart {init} options={chartOption(project, 'backend_lines_delta', m.code_monitor_backend_delta())} style="height:260px;width:100%" />
						</article>
					</div>
				</section>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page { min-height: 100%; padding: 24px; display: flex; flex-direction: column; gap: 18px; }
	.header, .project { border: 1px solid; border-radius: 12px; padding: 20px; }
	.header { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 18px; }
	h1, h2, p { margin: 0; }
	h1 { font-size: 1.35rem; font-weight: 650; }
	.header p { margin-top: 5px; font-size: .9rem; color: #94A3B8; }
	.ranges { display: flex; flex-wrap: wrap; gap: 7px; }
	button { border: 1px solid #64748B; border-radius: 7px; padding: 7px 10px; background: transparent; color: inherit; cursor: pointer; }
	button.active { border-color: #818CF8; background: rgba(129,140,248,.18); }
	.state { padding: 32px; text-align: center; }
	.state.error { color: #F87171; }
	.state.error button { margin-left: 8px; }
	.projects { display: flex; flex-direction: column; gap: 18px; }
	h2 { margin-bottom: 18px; font-size: 1.05rem; overflow-wrap: anywhere; }
	.charts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
	article { min-width: 0; }
	h3 { margin: 0 0 8px; font-size: .95rem; font-weight: 600; }
	@media (max-width: 760px) { .charts { grid-template-columns: 1fr; } }
	@media (max-width: 640px) { .page { padding: 14px; } .header { align-items: flex-start; } }
</style>
