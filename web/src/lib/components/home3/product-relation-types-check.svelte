<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import {
		approveProductRelationType, deleteProductRelationType, listProductRelationTypes,
		validateRelationTypeInput, DEFAULT_RELATION_WEIGHT, type ProductRelationType
	} from './product-relation-types-client.js';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorOk = $derived(darkMode ? '#34D399' : '#059669');
	let colorWarn = $derived(darkMode ? '#FBBF24' : '#D97706');
	let colorError = $derived(darkMode ? '#F87171' : '#DC2626');

	type Draft = { en: string; zh: string; weight: string; busy: boolean; error: string };

	let status = $state<'idle' | 'checking' | 'ok' | 'stale' | 'error'>('idle');
	let errorMsg = $state('');
	let notice = $state('');
	let proposed = $state<ProductRelationType[]>([]);
	let drafts = $state<Record<string, Draft>>({});

	async function runCheck() {
		status = 'checking';
		errorMsg = '';
		notice = '';
		try {
			const result = await listProductRelationTypes();
			proposed = result.rows.filter((row) => row.status === 'proposed');
			drafts = Object.fromEntries(
				proposed.map((row) => [
					row.code,
					{ en: '', zh: '', weight: String(DEFAULT_RELATION_WEIGHT), busy: false, error: '' }
				])
			);
			status = proposed.length === 0 ? 'ok' : 'stale';
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : String(err);
			status = 'error';
		}
	}

	function settle(code: string) {
		proposed = proposed.filter((row) => row.code !== code);
		if (proposed.length === 0) status = 'ok';
	}

	async function approve(row: ProductRelationType) {
		const draft = drafts[row.code];
		const input = { description_en: draft.en, description_zh: draft.zh, weight: Number(draft.weight) };
		const problem = validateRelationTypeInput(input);
		if (problem) {
			draft.error = problem === 'descriptions' ? m.prt_validation_descriptions() : m.prt_validation_weight();
			return;
		}
		draft.busy = true;
		draft.error = '';
		try {
			await approveProductRelationType(row.code, input);
			notice = m.prt_approved_notice({ code: row.code });
			settle(row.code);
		} catch (err) {
			draft.error = err instanceof Error ? err.message : String(err);
			draft.busy = false;
		}
	}

	async function remove(row: ProductRelationType) {
		if (row.product_count > 0 || !confirm(m.prt_delete_confirm({ code: row.code }))) return;
		const draft = drafts[row.code];
		draft.busy = true;
		draft.error = '';
		try {
			await deleteProductRelationType(row.code);
			notice = m.prt_deleted_notice({ code: row.code });
			settle(row.code);
		} catch (err) {
			draft.error = err instanceof Error ? err.message : String(err);
			draft.busy = false;
		}
	}

	let field = $derived(
		`width:100%; box-sizing:border-box; padding:7px 10px; border-radius:6px; font-size:12px; font-family:inherit; border:1px solid ${borderColor}; background:${surface2}; color:${textPrimary};`
	);
</script>

<div class="rounded-xl mb-4" style="background:{cardBg}; border:1px solid {borderColor}; overflow:hidden;">
	<div class="flex items-start justify-between gap-4 p-5">
		<div class="flex-1 min-w-0">
			<div class="flex items-center gap-2 mb-1">
				{#if status === 'ok'}
					<ShieldCheckIcon style="width:16px; height:16px; color:{colorOk}; flex-shrink:0;" />
				{:else if status === 'stale'}
					<AlertTriangleIcon style="width:16px; height:16px; color:{colorWarn}; flex-shrink:0;" />
				{:else if status === 'error'}
					<AlertTriangleIcon style="width:16px; height:16px; color:{colorError}; flex-shrink:0;" />
				{:else}
					<div style="width:16px; height:16px; flex-shrink:0;"></div>
				{/if}
				<span style="font-size:14px; font-weight:600; color:{textPrimary};">{m.prt_check_title()}</span>
			</div>
			<p style="font-size:12px; color:{textMuted}; line-height:1.6; margin-left:24px;">{m.prt_check_description()}</p>
		</div>
		<button
			onclick={runCheck}
			disabled={status === 'checking'}
			style="display:flex; align-items:center; gap:6px; padding:6px 14px; border-radius:7px; border:none; cursor:pointer; font-size:13px; font-weight:500; background:{surface2}; color:{textPrimary}; opacity:{status === 'checking' ? 0.5 : 1};"
		>
			<RefreshCwIcon style="width:13px; height:13px;{status === 'checking' ? ' animation:spin 1s linear infinite;' : ''}" />
			{status === 'checking' ? m.db_consistency_checking() : m.db_consistency_check()}
		</button>
	</div>

	{#if status !== 'idle'}
		<div style="padding:10px 20px; border-top:1px solid {borderColor}; font-size:13px;">
			{#if status === 'checking'}
				<span style="color:{textMuted};">{m.db_consistency_scanning()}</span>
			{:else if status === 'ok'}
				<span style="color:{colorOk};">{notice || m.prt_all_reviewed()}</span>
			{:else if status === 'stale'}
				<span style="color:{colorWarn};">{m.prt_proposed_found({ count: proposed.length })}</span>
				{#if notice}<span style="color:{colorOk}; margin-left:10px;">{notice}</span>{/if}
			{:else}
				<span style="color:{colorError};">{m.db_consistency_error({ errorMsg })}</span>
			{/if}
		</div>
	{/if}

	{#if status === 'stale'}
		{#each proposed as row (row.code)}
			{@const draft = drafts[row.code]}
			<div style="padding:16px 20px; border-top:1px solid {borderColor};">
				<div class="flex items-baseline justify-between gap-3 mb-2">
					<span style="font-size:14px; font-weight:600; color:{textPrimary};"><code>{row.code}</code></span>
					<span style="font-size:11px; color:{textSecondary};">
						{m.prt_seen_count()}: {row.seen_count} · {m.prt_product_count()}: {row.product_count} ·
						{m.prt_first_record()}: {row.first_record_id ?? m.prt_not_recorded()} ·
						{m.prt_first_product()}: {row.first_product_ref || m.prt_not_recorded()}
					</span>
				</div>
				<div class="grid gap-3" style="grid-template-columns:1fr 1fr 110px;">
					<label style="font-size:11px; color:{textSecondary};">{m.prt_description_en()}
						<textarea rows="2" style={field} bind:value={draft.en}></textarea>
					</label>
					<label style="font-size:11px; color:{textSecondary};">{m.prt_description_zh()}
						<textarea rows="2" style={field} bind:value={draft.zh}></textarea>
					</label>
					<label style="font-size:11px; color:{textSecondary};">{m.prt_weight()}
						<input type="number" min="0" max="1" step="0.05" style={field} bind:value={draft.weight} />
					</label>
				</div>
				<p style="font-size:11px; color:{textMuted}; margin:6px 0 10px;">{m.prt_weight_help()}</p>
				{#if draft.error}<p style="font-size:12px; color:{colorError}; margin-bottom:8px;">{draft.error}</p>{/if}
				<div class="flex items-center gap-2">
					<button
						onclick={() => approve(row)}
						disabled={draft.busy}
						style="padding:6px 14px; border-radius:7px; border:none; cursor:pointer; font-size:13px; font-weight:500; background:{accent}; color:white; opacity:{draft.busy ? 0.6 : 1};"
					>{draft.busy ? m.prt_approving() : m.prt_approve()}</button>
					<button
						onclick={() => remove(row)}
						disabled={draft.busy || row.product_count > 0}
						title={row.product_count > 0 ? m.prt_delete_in_use() : undefined}
						style="padding:6px 14px; border-radius:7px; border:1px solid {borderColor}; cursor:pointer; font-size:13px; background:transparent; color:{colorError}; opacity:{draft.busy || row.product_count > 0 ? 0.5 : 1};"
					>{m.prt_delete()}</button>
				</div>
			</div>
		{/each}
	{/if}
</div>

<style>
	@keyframes spin {
		from { transform: rotate(0deg); }
		to   { transform: rotate(360deg); }
	}
</style>
