<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import type { Snippet } from 'svelte';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import AlertTriangleIcon from '@lucide/svelte/icons/alert-triangle';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import {
		closeAttentionItem, listOpenAttentionItems, type AttentionItem
	} from './resolve-ambiguous-objects-client.js';

	type Props = {
		darkMode?: boolean;
		/** kb.attention_items types to list (see server/api/attention). */
		itemType: string | string[];
		title: string;
		description: string;
		noneText: string;
		foundText: (count: number) => string;
		dismissLabel: string;
		dismissHint: string;
		/** The left-hand content of one row. */
		row: Snippet<[AttentionItem]>;
	};

	let {
		darkMode = true, itemType, title, description, noneText, foundText, dismissLabel, dismissHint, row
	}: Props = $props();

	let cardBg = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let colorOk = $derived(darkMode ? '#34D399' : '#059669');
	let colorInfo = $derived(darkMode ? '#818CF8' : '#6366F1');
	let colorError = $derived(darkMode ? '#F87171' : '#DC2626');

	let status = $state<'idle' | 'checking' | 'ok' | 'open' | 'error'>('idle');
	let errorMsg = $state('');
	let items = $state<AttentionItem[]>([]);
	let busyId = $state<number | null>(null);
	let rowError = $state('');

	async function runCheck() {
		status = 'checking';
		errorMsg = '';
		rowError = '';
		try {
			const types = Array.isArray(itemType) ? itemType : [itemType];
			const results = await Promise.all(types.map((type) => listOpenAttentionItems(type)));
			items = results.flatMap((result) => result.items)
				.sort((a, b) => Date.parse(b.last_seen) - Date.parse(a.last_seen));
			status = items.length === 0 ? 'ok' : 'open';
		} catch (err) {
			errorMsg = err instanceof Error ? err.message : String(err);
			status = 'error';
		}
	}

	// The value is acceptable as written: the item closes and returns only if a later run
	// produces the same unresolved value again.
	async function dismiss(item: AttentionItem) {
		busyId = item.id;
		rowError = '';
		try {
			await closeAttentionItem(item.id, 'dismiss', '');
			items = items.filter((i) => i.id !== item.id);
			if (items.length === 0) status = 'ok';
		} catch (err) {
			rowError = err instanceof Error ? err.message : String(err);
		} finally {
			busyId = null;
		}
	}
</script>

<div class="rounded-xl mb-4" style="background:{cardBg}; border:1px solid {borderColor}; overflow:hidden;">
	<div class="flex items-start justify-between gap-4 p-5">
		<div class="flex-1 min-w-0">
			<div class="flex items-center gap-2 mb-1">
				{#if status === 'ok'}
					<ShieldCheckIcon style="width:16px; height:16px; color:{colorOk}; flex-shrink:0;" />
				{:else if status === 'open'}
					<AlertTriangleIcon style="width:16px; height:16px; color:{colorInfo}; flex-shrink:0;" />
				{:else if status === 'error'}
					<AlertTriangleIcon style="width:16px; height:16px; color:{colorError}; flex-shrink:0;" />
				{:else}
					<div style="width:16px; height:16px; flex-shrink:0;"></div>
				{/if}
				<span style="font-size:14px; font-weight:600; color:{textPrimary};">{title}</span>
			</div>
			<p style="font-size:12px; color:{textMuted}; line-height:1.6; margin-left:24px;">{description}</p>
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
				<span style="color:{colorOk};">{noneText}</span>
			{:else if status === 'open'}
				<span style="color:{colorInfo};">{foundText(items.length)}</span>
				{#if rowError}<span style="color:{colorError}; margin-left:10px;">{rowError}</span>{/if}
			{:else}
				<span style="color:{colorError};">{m.db_consistency_error({ errorMsg })}</span>
			{/if}
		</div>
	{/if}

	{#if status === 'open'}
		{#each items as item (item.id)}
			<div class="flex items-start justify-between gap-4" style="padding:12px 20px; border-top:1px solid {borderColor};">
				<div class="min-w-0">{@render row(item)}</div>
				<button
					onclick={() => dismiss(item)}
					disabled={busyId === item.id}
					title={dismissHint}
					style="flex-shrink:0; padding:6px 14px; border-radius:7px; border:1px solid {borderColor}; cursor:pointer; font-size:13px; background:transparent; color:{textPrimary}; opacity:{busyId === item.id ? 0.5 : 1};"
				>{dismissLabel}</button>
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
