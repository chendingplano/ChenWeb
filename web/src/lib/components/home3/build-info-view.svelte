<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import { listBuildActivities, type BuildActivity } from '$lib/services/buildInfoService';

	let { darkMode = true }: { darkMode: boolean } = $props();
	let activities = $state<BuildActivity[]>([]);
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
		accentTint: darkMode ? 'rgba(103,232,194,.12)' : 'rgba(8,127,104,.09)'
	});

	const latest = $derived(activities[0] ?? null);

	async function load() {
		loading = true;
		error = '';
		try {
			activities = await listBuildActivities();
		} catch (err) {
			error = err instanceof Error ? err.message : m.build_info_load_error();
		} finally {
			loading = false;
		}
	}

	function formatDate(value: string): string {
		const date = new Date(value);
		if (Number.isNaN(date.getTime())) return value;
		return new Intl.DateTimeFormat(getLocale() === 'zh-cn' ? 'zh-CN' : 'en-US', {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(date);
	}

	onMount(load);
</script>

<div class="build-page select-text" style={`--build-bg:${colors.bg};--build-card:${colors.card};--build-raised:${colors.raised};--build-text:${colors.text};--build-muted:${colors.muted};--build-border:${colors.border};--build-accent:${colors.accent};--build-accent-tint:${colors.accentTint}`}>
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
	{:else if latest}
		<section class="latest-panel" aria-label={m.build_info_latest_build()}>
			<div class="latest-topline">
				<span class="eyebrow">{m.build_info_latest_build()}</span>
				<span class="activity-pill"><span class="pulse"></span>{m.build_info_started()}</span>
			</div>
			<div class="latest-number">{latest.build_number}</div>
			<div class="latest-meta">
				<div><span>{m.build_info_build_name()}</span><strong>{latest.build_name}</strong></div>
				<div><span>{m.build_info_target_machine()}</span><strong>{latest.target_machine}</strong></div>
				<div><span>{m.build_info_os_info()}</span><strong>{latest.os_info}</strong></div>
				<div><span>{m.build_info_executable()}</span><strong>{latest.executable}</strong></div>
			</div>
			{#if latest.description}
				<p class="latest-description"><span>{m.build_info_description_label()}</span>{latest.description}</p>
			{/if}
		</section>

		<section class="activity-panel">
			<div class="section-head">
				<div>
					<p class="eyebrow">{m.build_info_recent_activity()}</p>
					<h2>{activities.length}</h2>
				</div>
			</div>
			<div class="table-scroll">
				<table>
					<thead>
						<tr>
							<th>{m.build_info_build_number()}</th>
							<th>{m.build_info_build_name()}</th>
							<th>{m.build_info_executable()}</th>
							<th>{m.build_info_target_machine()}</th>
							<th>{m.build_info_os_info()}</th>
							<th>{m.build_info_activity()}</th>
							<th>{m.build_info_recorded_at()}</th>
							<th>{m.build_info_description_label()}</th>
						</tr>
					</thead>
					<tbody>
						{#each activities as row (row.id)}
							<tr>
								<td class="build-number">{row.build_number}</td>
								<td>{row.build_name}</td>
								<td><span class="executable">{row.executable}</span></td>
								<td>{row.target_machine}</td>
								<td>{row.os_info}</td>
								<td><span class="activity-pill"><span class="pulse"></span>{m.build_info_started()}</span></td>
								<td class="date-cell">{formatDate(row.created_at)}</td>
								<td class="description-cell">{row.description || m.build_info_unspecified()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{:else}
		<div class="state-panel empty">{m.build_info_no_builds()}</div>
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
	.spinning{animation:spin 1s linear infinite}
	.latest-panel,.activity-panel,.state-panel{max-width:1440px;margin:0 auto 18px;border:1px solid var(--build-border);border-radius:14px;background:var(--build-card)}
	.latest-panel{position:relative;overflow:hidden;padding:22px 24px;background:radial-gradient(ellipse at 98% -30%,var(--build-accent-tint),transparent 46%),var(--build-card)}
	.latest-topline{display:flex;align-items:center;justify-content:space-between;gap:12px}
	.latest-number{margin:10px 0 19px;font:600 clamp(24px,4vw,36px)/1.15 'SFMono-Regular',Consolas,monospace;letter-spacing:.015em;color:var(--build-text);overflow-wrap:anywhere}
	.latest-meta{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;border-top:1px solid var(--build-border);padding-top:16px}
	.latest-meta div{display:flex;min-width:0;flex-direction:column;gap:5px}
	.latest-meta span,.latest-description span{color:var(--build-muted);font-size:10px;font-weight:700;letter-spacing:.09em;text-transform:uppercase}
	.latest-meta strong{font-size:13px;font-weight:600;overflow-wrap:anywhere}
	.latest-description{display:flex;gap:10px;align-items:baseline;margin:15px 0 0;color:var(--build-text);font-size:13px;overflow-wrap:anywhere}
	.activity-pill{display:inline-flex;align-items:center;gap:6px;width:max-content;padding:4px 8px;border:1px solid color-mix(in srgb,var(--build-accent) 26%,transparent);border-radius:999px;background:var(--build-accent-tint);color:var(--build-accent);font-size:10px;font-weight:700;letter-spacing:.04em;text-transform:uppercase;white-space:nowrap}
	.pulse{width:6px;height:6px;border-radius:50%;background:currentColor;box-shadow:0 0 0 3px color-mix(in srgb,currentColor 17%,transparent)}
	.activity-panel{overflow:hidden}
	.section-head{display:flex;align-items:center;justify-content:space-between;padding:17px 20px;border-bottom:1px solid var(--build-border)}
	.section-head h2{margin:0;font:600 19px 'SFMono-Regular',Consolas,monospace}
	.table-scroll{overflow-x:auto}
	table{width:100%;border-collapse:collapse;text-align:left;white-space:nowrap}
	th{padding:11px 14px;background:var(--build-raised);color:var(--build-muted);font-size:9px;font-weight:750;letter-spacing:.1em;text-transform:uppercase}
	td{padding:12px 14px;border-top:1px solid var(--build-border);font-size:12px;vertical-align:top}
	tbody tr:hover{background:var(--build-raised)}
	.build-number,.executable,.date-cell{font-family:'SFMono-Regular',Consolas,monospace;font-size:11px}
	.build-number{color:var(--build-accent)}
	.executable{padding:3px 6px;border:1px solid var(--build-border);border-radius:5px;background:var(--build-raised)}
	.description-cell{min-width:180px;max-width:320px;white-space:normal;overflow-wrap:anywhere}
	.state-panel{padding:32px 22px;color:var(--build-muted);text-align:center;font-size:13px}
	.state-panel.error{border-color:color-mix(in srgb,#f87171 45%,var(--build-border));color:darkred}
	@media(max-width:800px){.latest-meta{grid-template-columns:repeat(2,minmax(0,1fr))}.page-head{align-items:flex-start}.latest-panel{padding:18px}}
	@media(max-width:520px){.page-head{flex-direction:column}.latest-meta{gap:15px 10px}.latest-description{align-items:flex-start;flex-direction:column;gap:5px}}
	@keyframes spin{to{transform:rotate(360deg)}}
</style>
