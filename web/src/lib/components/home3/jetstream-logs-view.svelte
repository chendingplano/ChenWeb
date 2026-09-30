<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';

	let { darkMode = true }: { darkMode: boolean } = $props();

	type Endpoint = 'jsz' | 'varz' | 'connz';
	type EndpointData = Record<string, any>;

	const endpoints: { id: Endpoint; label: string; help: string }[] = [
		{ id: 'jsz', label: m.jetstream_logs_jetstream(), help: m.jetstream_logs_jetstream_health_storage_streams_and() },
		{ id: 'varz', label: m.jetstream_logs_server(), help: m.jetstream_logs_nats_server_runtime_information_and() },
		{ id: 'connz', label: m.jetstream_logs_connections(), help: m.jetstream_logs_current_client_connection_details_and() }
	];

	let endpoint: Endpoint = $state('jsz');
	let loading = $state(false);
	let error = $state('');
	let lastUpdated = $state('');
	let payload = $state<EndpointData | null>(null);
	let autoRefresh = $state(true);
	let refreshSec = $state(5);
	let timer: ReturnType<typeof setInterval> | null = null;

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let success = $derived(darkMode ? '#34D399' : '#059669');
	let danger = $derived(darkMode ? '#F87171' : '#DC2626');

	function formatNow(): string {
		return new Date().toLocaleString();
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const res = await fetch(`/api/v1/jetstream/monitor?endpoint=${endpoint}`, {
				credentials: 'same-origin'
			});
			const data = await res.json();
			if (!res.ok || !data.ok) {
				throw new Error(data.message ?? m.jetstream_logs_failed_to_fetch_jetstream_monitoring());
			}
			payload = data.data ?? {};
			lastUpdated = formatNow();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			payload = null;
		} finally {
			loading = false;
		}
	}

	function restartAutoRefresh() {
		if (timer) {
			clearInterval(timer);
			timer = null;
		}
		if (!autoRefresh) return;
		timer = setInterval(() => {
			load();
		}, Math.max(1, refreshSec) * 1000);
	}

	onMount(() => {
		load();
		restartAutoRefresh();
		return () => {
			if (timer) clearInterval(timer);
		};
	});

	$effect(() => {
		endpoint;
		load();
	});

	$effect(() => {
		autoRefresh;
		refreshSec;
		restartAutoRefresh();
	});

	let summary = $derived.by(() => {
		if (!payload) return [] as { label: string; value: string | number }[];
		if (endpoint === 'jsz') {
			return [
				{ label: m.jetstream_logs_memory_bytes(), value: payload.memory ?? 'n/a' },
				{ label: m.jetstream_logs_stored_bytes(), value: payload.store ?? 'n/a' },
				{ label: m.jetstream_logs_streams(), value: payload.streams ?? 'n/a' },
				{ label: m.jetstream_logs_consumers(), value: payload.consumers ?? 'n/a' }
			];
		}
		if (endpoint === 'connz') {
			return [
				{ label: m.jetstream_logs_connections(), value: payload.num_connections ?? 'n/a' },
				{ label: m.jetstream_logs_total(), value: payload.total ?? 'n/a' },
				{ label: m.jetstream_logs_offset(), value: payload.offset ?? 'n/a' },
				{ label: m.jetstream_logs_limit(), value: payload.limit ?? 'n/a' }
			];
		}
		return [
			{ label: m.jetstream_logs_server_id(), value: payload.server_id ?? 'n/a' },
			{ label: m.jetstream_logs_version(), value: payload.version ?? 'n/a' },
			{ label: m.jetstream_logs_uptime(), value: payload.uptime ?? 'n/a' },
			{ label: m.jetstream_logs_connections(), value: payload.connections ?? 'n/a' }
		];
	});
</script>

<div class="p-6 space-y-4">
	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="flex flex-wrap items-center gap-3 justify-between">
			<div>
				<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">{m.jetstream_logs_jetstream()}</h2>
				<p style="font-size:13px; color:{textSecondary};">{m.jetstream_logs_live_monitoring_data_from_nats()}</p>
			</div>
			<div class="flex items-center gap-2">
				<button
					onclick={load}
					disabled={loading}
					class="inline-flex items-center gap-2 rounded-lg px-3 py-2 cursor-pointer"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					<RefreshCwIcon class="w-4 h-4" />
					{m.jetstream_logs_refresh()}
				</button>
			</div>
		</div>

		<div class="mt-4 flex flex-wrap items-center gap-2">
			{#each endpoints as ep}
				<button
					onclick={() => (endpoint = ep.id)}
					class="rounded-lg px-3 py-2 text-sm cursor-pointer"
					style="
						border:1px solid {endpoint === ep.id ? accent : borderColor};
						background:{endpoint === ep.id ? accent + '20' : surface2};
						color:{endpoint === ep.id ? accent : textSecondary};
					"
					title={ep.help}
				>
					{ep.label}
				</button>
			{/each}
		</div>

		<div class="mt-4 flex flex-wrap items-center gap-3 text-sm" style="color:{textSecondary};">
			<label class="inline-flex items-center gap-2">
				<input type="checkbox" bind:checked={autoRefresh} />
				{m.jetstream_logs_auto_refresh()}
			</label>
			<label class="inline-flex items-center gap-2">
				{m.jetstream_logs_every()}
				<input type="number" min="1" max="60" bind:value={refreshSec} class="w-16 rounded px-2 py-1"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" />
				s
			</label>
			{#if lastUpdated}
				<span style="color:{textMuted};">{m.jetstream_logs_last_updated({ lastUpdated })}</span>
			{/if}
		</div>
	</div>

	{#if error}
		<div class="rounded-xl p-4 flex items-start gap-2" style="background:{danger}20; border:1px solid {danger}70; color:{danger};">
			<CircleAlertIcon class="w-4 h-4 mt-0.5" />
			<div>
				<div style="font-weight:600;">{m.jetstream_logs_unable_to_load_jetstream_data()}</div>
				<div style="font-size:13px; opacity:0.95;">{error}</div>
			</div>
		</div>
	{:else if payload}
		<div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
			{#each summary as item}
				<div class="rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
					<div style="font-size:12px; color:{textMuted}; text-transform:uppercase; letter-spacing:0.04em;">{item.label}</div>
					<div style="font-size:18px; font-weight:600; color:{textPrimary}; margin-top:6px; word-break:break-all;">{item.value}</div>
				</div>
			{/each}
		</div>

		<div class="rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
			<div class="flex items-center gap-2" style="color:{success}; font-size:13px; font-weight:600;">
				<CircleCheckBigIcon class="w-4 h-4" />
				{m.jetstream_logs_endpoint_reachable()}
			</div>
			<pre class="mt-3 overflow-auto p-3 rounded-lg" style="max-height:460px; background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px;">{JSON.stringify(payload, null, 2)}</pre>
		</div>
	{:else}
		<div class="rounded-xl p-6" style="background:{cardBg}; border:1px solid {borderColor}; color:{textSecondary};">
			{m.jetstream_logs_loading()}
		</div>
	{/if}
</div>
