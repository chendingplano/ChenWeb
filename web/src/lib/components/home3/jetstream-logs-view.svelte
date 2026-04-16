<script lang="ts">
	import { onMount } from 'svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';

	let { darkMode = true }: { darkMode: boolean } = $props();

	type Endpoint = 'jsz' | 'varz' | 'connz';
	type EndpointData = Record<string, any>;

	const endpoints: { id: Endpoint; label: string; help: string }[] = [
		{ id: 'jsz', label: 'JetStream', help: 'JetStream health, storage, streams and consumers.' },
		{ id: 'varz', label: 'Server', help: 'NATS server runtime information and process stats.' },
		{ id: 'connz', label: 'Connections', help: 'Current client connection details and auth identity.' }
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
				throw new Error(data.message ?? 'Failed to fetch JetStream monitoring data');
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
				{ label: 'Memory (bytes)', value: payload.memory ?? 'n/a' },
				{ label: 'Stored (bytes)', value: payload.store ?? 'n/a' },
				{ label: 'Streams', value: payload.streams ?? 'n/a' },
				{ label: 'Consumers', value: payload.consumers ?? 'n/a' }
			];
		}
		if (endpoint === 'connz') {
			return [
				{ label: 'Connections', value: payload.num_connections ?? 'n/a' },
				{ label: 'Total', value: payload.total ?? 'n/a' },
				{ label: 'Offset', value: payload.offset ?? 'n/a' },
				{ label: 'Limit', value: payload.limit ?? 'n/a' }
			];
		}
		return [
			{ label: 'Server ID', value: payload.server_id ?? 'n/a' },
			{ label: 'Version', value: payload.version ?? 'n/a' },
			{ label: 'Uptime', value: payload.uptime ?? 'n/a' },
			{ label: 'Connections', value: payload.connections ?? 'n/a' }
		];
	});
</script>

<div class="p-6 space-y-4">
	<div class="rounded-xl p-5" style="background:{cardBg}; border:1px solid {borderColor};">
		<div class="flex flex-wrap items-center gap-3 justify-between">
			<div>
				<h2 style="font-size:18px; font-weight:600; color:{textPrimary};">JetStream</h2>
				<p style="font-size:13px; color:{textSecondary};">Live monitoring data from NATS monitoring endpoints.</p>
			</div>
			<div class="flex items-center gap-2">
				<button
					onclick={load}
					disabled={loading}
					class="inline-flex items-center gap-2 rounded-lg px-3 py-2 cursor-pointer"
					style="background:{surface2}; color:{textPrimary}; border:1px solid {borderColor};"
				>
					<RefreshCwIcon class="w-4 h-4" />
					Refresh
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
				Auto refresh
			</label>
			<label class="inline-flex items-center gap-2">
				Every
				<input type="number" min="1" max="60" bind:value={refreshSec} class="w-16 rounded px-2 py-1"
					style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" />
				s
			</label>
			{#if lastUpdated}
				<span style="color:{textMuted};">Last updated: {lastUpdated}</span>
			{/if}
		</div>
	</div>

	{#if error}
		<div class="rounded-xl p-4 flex items-start gap-2" style="background:{danger}20; border:1px solid {danger}70; color:{danger};">
			<CircleAlertIcon class="w-4 h-4 mt-0.5" />
			<div>
				<div style="font-weight:600;">Unable to load JetStream data</div>
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
				Endpoint reachable
			</div>
			<pre class="mt-3 overflow-auto p-3 rounded-lg" style="max-height:460px; background:{surface2}; border:1px solid {borderColor}; color:{textPrimary}; font-size:12px;">{JSON.stringify(payload, null, 2)}</pre>
		</div>
	{:else}
		<div class="rounded-xl p-6" style="background:{cardBg}; border:1px solid {borderColor}; color:{textSecondary};">
			Loading…
		</div>
	{/if}
</div>
