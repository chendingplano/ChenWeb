<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CheckCircle2Icon from '@lucide/svelte/icons/circle-check';
	import { listBuildProcesses, type BuildProcess } from '$lib/services/buildInfoService';

	let { darkMode = true }: { darkMode: boolean } = $props();
	let processes = $state<BuildProcess[]>([]);
	let loading = $state(true);
	let error = $state('');

	const colors = $derived({
		bg: darkMode ? '#171B26' : '#F2F4F7',
		card: darkMode ? '#1F2333' : '#FFFFFF',
		raised: darkMode ? '#252A3A' : '#F8FAFC',
		text: darkMode ? '#E2E8F0' : '#111827',
		muted: darkMode ? '#94A3B8' : '#64748B',
		border: darkMode ? '#2D3348' : '#DCE2EA',
		accent: darkMode ? '#67E8C2' : '#087F68',
		accentTint: darkMode ? 'rgba(103,232,194,.12)' : 'rgba(8,127,104,.09)',
		warning: darkMode ? '#FBBF24' : '#A16207',
		warningTint: darkMode ? 'rgba(251,191,36,.12)' : 'rgba(161,98,7,.08)'
	});

	const running = $derived(processes.filter((process) => process.status === 'running'));
	const buildNumbers = $derived([...new Set(running.map((process) => process.build_number).filter(Boolean))]);
	const hasMismatch = $derived(buildNumbers.length > 1);
	const allReported = $derived(processes.length > 0 && processes.every((process) => process.status === 'running' && process.build_number));

	async function load() {
		loading = true;
		error = '';
		try {
			processes = await listBuildProcesses();
		} catch (err) {
			error = err instanceof Error ? err.message : m.build_info_load_error();
		} finally {
			loading = false;
		}
	}

	function formatDate(value?: string): string {
		if (!value) return m.build_info_unspecified();
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return new Intl.DateTimeFormat(getLocale() === 'zh-cn' ? 'zh-CN' : 'en-US', {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(date);
	}

	function statusLabel(status: BuildProcess['status']): string {
		if (status === 'running') return m.build_info_running();
		if (status === 'stopped') return m.build_info_stopped();
		return m.build_info_unreported_status();
	}

	onMount(load);
</script>

<div class="build-page select-text" style={`--build-bg:${colors.bg};--build-card:${colors.card};--build-raised:${colors.raised};--build-text:${colors.text};--build-muted:${colors.muted};--build-border:${colors.border};--build-accent:${colors.accent};--build-accent-tint:${colors.accentTint};--build-warning:${colors.warning};--build-warning-tint:${colors.warningTint}`}>
	<header class="page-head">
		<div class="title-lockup">
			<div class="title-icon" aria-hidden="true"><ActivityIcon size={19} /></div>
			<div>
				<p class="eyebrow">{m.nav_sysadmin_system()}</p>
				<h1>{m.build_info_title()}</h1>
				<p class="lede">{m.build_info_description()}</p>
			</div>
		</div>
		<button type="button" class="refresh" onclick={load} disabled={loading}>
			<RefreshCwIcon size={15} class={loading ? 'spinning' : ''} />
			{m.build_info_refresh()}
		</button>
	</header>

	{#if loading}
		<div class="state-panel" role="status">{m.build_info_loading()}</div>
	{:else if error}
		<div class="state-panel error" role="alert">{error}</div>
	{:else}
		{#if hasMismatch}
			<div class="consistency warning" role="status">
				<CircleAlertIcon size={18} />
				<div><strong>{m.build_info_mismatch()}</strong><span>{buildNumbers.join(' · ')}</span></div>
			</div>
		{:else if allReported}
			<div class="consistency good" role="status">
				<CheckCircle2Icon size={18} />
				<div><strong>{m.build_info_consistent()}</strong><span>{buildNumbers[0]}</span></div>
			</div>
		{:else}
			<div class="consistency warning" role="status">
				<CircleAlertIcon size={18} />
				<div><strong>{m.build_info_incomplete()}</strong><span>{m.build_info_incomplete_detail()}</span></div>
			</div>
		{/if}

		<section class="activity-panel">
			<div class="section-head">
				<div>
					<p class="eyebrow">{m.build_info_live_processes()}</p>
					<h2>{running.length} / {processes.length}</h2>
				</div>
			</div>
			<div class="table-scroll">
				<table>
					<thead>
						<tr>
							<th>{m.build_info_executable()}</th>
							<th>{m.build_info_status()}</th>
							<th>{m.build_info_process_id()}</th>
							<th>{m.build_info_build_number()}</th>
							<th>{m.build_info_build_name()}</th>
							<th>{m.build_info_target_machine()}</th>
							<th>{m.build_info_os_info()}</th>
							<th>{m.build_info_started_at()}</th>
							<th>{m.build_info_description_label()}</th>
						</tr>
					</thead>
					<tbody>
						{#each processes as process (process.executable)}
							<tr>
								<td><span class="executable">{process.executable}</span></td>
								<td><span class:live={process.status === 'running'} class:offline={process.status !== 'running'} class="status-pill">{statusLabel(process.status)}</span></td>
								<td class="mono">{process.pid ?? m.build_info_unspecified()}</td>
								<td class="build-number">{process.build_number ?? m.build_info_unspecified()}</td>
								<td>{process.build_name ?? m.build_info_unspecified()}</td>
								<td>{process.target_machine ?? m.build_info_unspecified()}</td>
								<td>{process.os_info ?? m.build_info_unspecified()}</td>
								<td class="date-cell">{formatDate(process.started_at)}</td>
								<td class="description-cell">{process.description || m.build_info_unspecified()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}
</div>

<style>
	.build-page{min-height:100%;padding:clamp(18px,3vw,34px);background:var(--build-bg);color:var(--build-text);font-family:'Inter Tight',system-ui,sans-serif;user-select:text;-webkit-user-select:text}
	.page-head{display:flex;align-items:center;justify-content:space-between;gap:20px;margin:0 auto 24px;max-width:1440px}
	.title-lockup{display:flex;align-items:center;gap:14px;min-width:0}
	.title-icon{display:grid;place-items:center;width:44px;height:44px;flex:none;border:1px solid color-mix(in srgb,var(--build-accent) 32%,transparent);border-radius:13px;background:var(--build-accent-tint);color:var(--build-accent)}
	.eyebrow{margin:0 0 5px;color:var(--build-accent);font-size:10px;font-weight:750;letter-spacing:.14em;text-transform:uppercase}
	h1{margin:0;font-size:clamp(22px,2.5vw,30px);font-weight:680;letter-spacing:-.035em}
	.lede{margin:5px 0 0;color:var(--build-muted);font-size:13px}
	.refresh{display:flex;align-items:center;gap:8px;padding:9px 13px;border:1px solid var(--build-border);border-radius:9px;background:var(--build-card);color:var(--build-text);font-size:12px;font-family:inherit;font-weight:600;cursor:pointer}
	.refresh:disabled{opacity:.65;cursor:default}
	:global(.spinning){animation:spin 1s linear infinite}
	.activity-panel,.state-panel{max-width:1440px;margin:0 auto 18px;border:1px solid var(--build-border);border-radius:14px;background:var(--build-card)}
	.activity-panel{overflow:hidden}
	.consistency{display:flex;align-items:center;gap:12px;max-width:1440px;margin:0 auto 18px;padding:15px 18px;border:1px solid var(--build-border);border-radius:12px}
	.consistency div{display:flex;min-width:0;flex-direction:column;gap:4px}
	.consistency strong{font-size:13px}.consistency span{font:11px 'SFMono-Regular',Consolas,monospace;overflow-wrap:anywhere}
	.consistency.good{background:var(--build-accent-tint);border-color:color-mix(in srgb,var(--build-accent) 30%,var(--build-border));color:var(--build-accent)}
	.consistency.warning{background:var(--build-warning-tint);border-color:color-mix(in srgb,var(--build-warning) 35%,var(--build-border));color:var(--build-warning)}
	.section-head{display:flex;align-items:center;justify-content:space-between;padding:17px 20px;border-bottom:1px solid var(--build-border)}
	.section-head h2{margin:0;font:600 19px 'SFMono-Regular',Consolas,monospace}
	.table-scroll{overflow-x:auto}
	table{width:100%;border-collapse:collapse;text-align:left;white-space:nowrap}
	th{padding:11px 14px;background:var(--build-raised);color:var(--build-muted);font-size:9px;font-weight:750;letter-spacing:.1em;text-transform:uppercase}
	td{padding:12px 14px;border-top:1px solid var(--build-border);font-size:12px;vertical-align:top}
	tbody tr:hover{background:var(--build-raised)}
	.mono,.build-number,.executable,.date-cell{font-family:'SFMono-Regular',Consolas,monospace;font-size:11px}
	.build-number{color:var(--build-accent)}
	.executable{padding:3px 6px;border:1px solid var(--build-border);border-radius:5px;background:var(--build-raised)}
	.status-pill{display:inline-flex;padding:4px 8px;border:1px solid transparent;border-radius:999px;font-size:10px;font-weight:700;letter-spacing:.04em;text-transform:uppercase}
	.status-pill.live{border-color:color-mix(in srgb,var(--build-accent) 26%,transparent);background:var(--build-accent-tint);color:var(--build-accent)}
	.status-pill.offline{border-color:var(--build-border);background:var(--build-raised);color:var(--build-muted)}
	.description-cell{min-width:180px;max-width:320px;white-space:normal;overflow-wrap:anywhere}
	.state-panel{padding:32px 22px;color:var(--build-muted);text-align:center;font-size:13px}
	.state-panel.error{border-color:color-mix(in srgb,#f87171 45%,var(--build-border));color:darkred}
	@media(max-width:800px){.page-head{align-items:flex-start}}
	@media(max-width:520px){.page-head{flex-direction:column}}
	@keyframes spin{to{transform:rotate(360deg)}}
</style>
