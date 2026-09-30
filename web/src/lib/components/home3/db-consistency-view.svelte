<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import WrenchIcon from '@lucide/svelte/icons/wrench';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens ---
	let cardBg      = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2    = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorOk       = $derived(darkMode ? '#34D399' : '#059669');
	let colorWarn     = $derived(darkMode ? '#FBBF24' : '#D97706');
	let colorError    = $derived(darkMode ? '#F87171' : '#DC2626');

	type CheckStatus = 'idle' | 'checking' | 'ok' | 'stale' | 'error';

	type Check = {
		id: string;
		label: string;
		description: string;
		status: CheckStatus;
		staleCount: number;
		errorMsg: string;
		fixing: boolean;
		fixedCount: number | null;
	};

	let checks = $state<Check[]>([
		{
			id: 'kb-inputs-status',
			label: m.db_consistency_kb_inputs_status_duplicate_operation(),
			description:
				m.db_consistency_detects_records_where_the_status(),
			status: 'idle',
			staleCount: 0,
			errorMsg: '',
			fixing: false,
			fixedCount: null
		}
	]);

	async function runCheck(check: Check) {
		check.status = 'checking';
		check.errorMsg = '';
		check.fixedCount = null;
		try {
			const res = await fetch('/api/v1/admin/db/kb-inputs-status/check');
			if (!res.ok) {
				const body = await res.json().catch(() => ({}));
				throw new Error(body.error ?? `HTTP ${res.status}`);
			}
			const data: { stale_count: number } = await res.json();
			check.staleCount = data.stale_count;
			check.status = data.stale_count === 0 ? 'ok' : 'stale';
		} catch (err) {
			check.errorMsg = err instanceof Error ? err.message : String(err);
			check.status = 'error';
		}
	}

	async function runFix(check: Check) {
		check.fixing = true;
		check.errorMsg = '';
		try {
			const res = await fetch('/api/v1/admin/db/kb-inputs-status/fix', { method: 'POST' });
			if (!res.ok) {
				const body = await res.json().catch(() => ({}));
				throw new Error(body.error ?? `HTTP ${res.status}`);
			}
			const data: { fixed_count: number } = await res.json();
			check.fixedCount = data.fixed_count;
			check.staleCount = 0;
			check.status = 'ok';
		} catch (err) {
			check.errorMsg = err instanceof Error ? err.message : String(err);
		} finally {
			check.fixing = false;
		}
	}
</script>

<div class="p-6 max-w-3xl">
	<div class="mb-6">
		<h1 style="font-size:20px; font-weight:600; color:{textPrimary}; margin-bottom:4px;">
			{m.db_consistency_database_maintenance_consistency_check()}
		</h1>
		<p style="font-size:13px; color:{textSecondary};">
			{m.db_consistency_run_each_check_to_detect()}
		</p>
	</div>

	{#each checks as check (check.id)}
		<div
			class="rounded-xl mb-4"
			style="background:{cardBg}; border:1px solid {borderColor}; overflow:hidden;"
		>
			<!-- Header row -->
			<div class="flex items-start justify-between gap-4 p-5">
				<div class="flex-1 min-w-0">
					<div class="flex items-center gap-2 mb-1">
						<!-- Status icon -->
						{#if check.status === 'ok'}
							<ShieldCheckIcon style="width:16px; height:16px; color:{colorOk}; flex-shrink:0;" />
						{:else if check.status === 'stale'}
							<AlertTriangleIcon style="width:16px; height:16px; color:{colorWarn}; flex-shrink:0;" />
						{:else if check.status === 'error'}
							<AlertTriangleIcon style="width:16px; height:16px; color:{colorError}; flex-shrink:0;" />
						{:else}
							<div style="width:16px; height:16px; flex-shrink:0;"></div>
						{/if}
						<span style="font-size:14px; font-weight:600; color:{textPrimary};">{check.label}</span>
					</div>
					<p style="font-size:12px; color:{textMuted}; line-height:1.6; margin-left:24px;">
						{check.description}
					</p>
				</div>

				<!-- Action buttons -->
				<div class="flex items-center gap-2 flex-shrink-0">
					<button
						onclick={() => runCheck(check)}
						disabled={check.status === 'checking' || check.fixing}
						style="
							display:flex; align-items:center; gap:6px;
							padding:6px 14px; border-radius:7px; border:none; cursor:pointer;
							font-size:13px; font-weight:500;
							background:{surface2}; color:{textPrimary};
							opacity:{check.status === 'checking' || check.fixing ? 0.5 : 1};
						"
					>
						<RefreshCwIcon
							style="width:13px; height:13px;{check.status === 'checking' ? ' animation:spin 1s linear infinite;' : ''}"
						/>
						{check.status === 'checking' ? m.db_consistency_checking() : m.db_consistency_check()}
					</button>

					{#if check.status === 'stale'}
						<button
							onclick={() => runFix(check)}
							disabled={check.fixing}
							style="
								display:flex; align-items:center; gap:6px;
								padding:6px 14px; border-radius:7px; border:none; cursor:pointer;
								font-size:13px; font-weight:500;
								background:{accent}; color:white;
								opacity:{check.fixing ? 0.6 : 1};
							"
						>
							<WrenchIcon style="width:13px; height:13px;" />
							{check.fixing ? m.db_consistency_fixing() : m.db_consistency_fix()}
						</button>
					{/if}
				</div>
			</div>

			<!-- Result banner -->
			{#if check.status !== 'idle'}
				<div
					style="
						padding:10px 20px;
						border-top:1px solid {borderColor};
						background:{
							check.status === 'ok'    ? (darkMode ? 'rgba(52,211,153,0.06)' : 'rgba(5,150,105,0.05)') :
							check.status === 'stale' ? (darkMode ? 'rgba(251,191,36,0.06)'  : 'rgba(217,119,6,0.05)') :
							check.status === 'error' ? (darkMode ? 'rgba(248,113,113,0.06)' : 'rgba(220,38,38,0.05)') :
							'transparent'
						};
						font-size:13px;
					"
				>
					{#if check.status === 'checking'}
						<span style="color:{textMuted};">{m.db_consistency_scanning()}</span>
					{:else if check.status === 'ok' && check.fixedCount !== null}
						<span style="color:{colorOk};">
							{m.db_consistency_fixed_record_all_entries_are({ fixedCount: check.fixedCount, plural: check.fixedCount === 1 ? '' : 's' })}
						</span>
					{:else if check.status === 'ok'}
						<span style="color:{colorOk};">{m.db_consistency_all_records_are_consistent_no()}</span>
					{:else if check.status === 'stale'}
						<span style="color:{colorWarn};">
							{m.db_consistency_record_duplicate_operation_entries_in({ staleCount: check.staleCount, plural: check.staleCount === 1 ? '' : 's' })} <code>kb.inputs.status</code>{m.db_consistency_click()} <strong>{m.db_consistency_fix()}</strong> {m.db_consistency_to_deduplicate_keeps_last_entry()} <code>kb.input_proc_status</code> {m.db_consistency_automatically()}
						</span>
					{:else if check.status === 'error'}
						<span style="color:{colorError};">{m.db_consistency_error({ errorMsg: check.errorMsg })}</span>
					{/if}
				</div>
			{/if}
		</div>
	{/each}
</div>

<style>
	@keyframes spin {
		from { transform: rotate(0deg); }
		to   { transform: rotate(360deg); }
	}
</style>
