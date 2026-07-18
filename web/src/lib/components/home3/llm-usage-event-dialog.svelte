<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import type { LLMUsageEventDetail } from './llm-activities-client.js';
	import { buildJsonTree, type JsonTreeNode } from './doc-review-json-dialog.js';
	import JsonTree from './json-tree.svelte';

	let { event, dark = true, onclose }: { event: LLMUsageEventDetail; dark?: boolean; onclose: () => void } = $props();

	let cardBg = $derived(dark ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(dark ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(dark ? '#2D3348' : '#E4E6EB');
	let accent = $derived(dark ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(dark ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');
	let danger = $derived(dark ? '#F87171' : '#DC2626');
	let overlay = $derived(dark ? '#0D1117E6' : '#00000066');
	let scrollThumb = $derived(dark ? '#2A3140' : '#D7CFB8');

	const fields: { label: string; key: keyof LLMUsageEventDetail }[] = [
		{ label: 'id', key: 'id' },
		{ label: 'account_id', key: 'account_id' },
		{ label: 'account_name', key: 'account_name' },
		{ label: 'profile_id', key: 'profile_id' },
		{ label: 'record_id', key: 'record_id' },
		{ label: 'provider', key: 'provider' },
		{ label: 'model_name', key: 'model_name' },
		{ label: 'prompt_name', key: 'prompt_name' },
		{ label: 'call_reason', key: 'call_reason' },
		{ label: 'call_loc', key: 'call_loc' },
		{ label: 'request_started_at', key: 'request_started_at' },
		{ label: 'input_tokens', key: 'input_tokens' },
		{ label: 'output_tokens', key: 'output_tokens' },
		{ label: 'total_tokens', key: 'total_tokens' },
		{ label: 'prompt_cache_hit_tokens', key: 'prompt_cache_hit_tokens' },
		{ label: 'prompt_cache_miss_tokens', key: 'prompt_cache_miss_tokens' },
		{ label: 'latency_ms', key: 'latency_ms' },
		{ label: 'error_message', key: 'error_message' }
	];

	function displayValue(value: unknown): string {
		return value == null || value === '' ? '—' : String(value);
	}

	// ── Body viewer (View button for input_body_ref / output_body_ref) ────────
	// Mirrors llm-usage-logs-view.svelte's openBody (fetch the archived
	// request/response body), but renders it in the same recursive name-value
	// fashion as metadata_json rather than a raw pretty-printed JSON dump.
	let bodyView = $state<{
		title: string;
		loading: boolean;
		error: string;
		nodes: JsonTreeNode[];
		rawText: string;
	} | null>(null);

	async function openBody(type: 'input' | 'output') {
		const ref = type === 'input' ? event.input_body_ref : event.output_body_ref;
		if (!ref) return;
		bodyView = { title: `${type === 'input' ? 'Input' : 'Output'} Body — ${event.id.slice(0, 12)}…`, loading: true, error: '', nodes: [], rawText: '' };
		try {
			const res = await fetch(`/api/v1/llm/usage-events/${event.id}/body?type=${type}`, { credentials: 'same-origin' });
			const text = await res.text();
			if (!res.ok) {
				const msg = (() => { try { return JSON.parse(text).message; } catch { return text; } })();
				throw new Error(msg || 'Failed to load body');
			}
			try {
				bodyView = { ...bodyView, loading: false, nodes: buildJsonTree(JSON.parse(text)) };
			} catch {
				bodyView = { ...bodyView, loading: false, rawText: text };
			}
		} catch (err) {
			bodyView = { ...bodyView, loading: false, error: err instanceof Error ? err.message : String(err) };
		}
	}

	// prompt_dir is a local filesystem path (environment-specific, not useful
	// to a reviewer), so it's hidden from the metadata_json block here.
	function omitPromptDir(value: unknown): unknown {
		if (value == null || typeof value !== 'object' || Array.isArray(value)) return value;
		const { prompt_dir, ...rest } = value as Record<string, unknown>;
		return rest;
	}

	let metadataNodes = $derived(buildJsonTree(omitPromptDir(event.metadata_json)));

	let modalElement = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let returnFocusElement: HTMLElement | null = null;

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.preventDefault();
			if (bodyView) bodyView = null;
			else onclose();
			return;
		}
		if (e.key !== 'Tab' || !modalElement) return;
		const focusable = Array.from(
			modalElement.querySelectorAll<HTMLElement>(
				'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
			)
		);
		if (!focusable.length) {
			e.preventDefault();
			modalElement.focus();
			return;
		}
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (e.shiftKey && document.activeElement === first) {
			e.preventDefault();
			last.focus();
		} else if (!e.shiftKey && document.activeElement === last) {
			e.preventDefault();
			first.focus();
		}
	}

	$effect(() => {
		returnFocusElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		requestAnimationFrame(() => closeButton?.focus());
		return () => {
			returnFocusElement?.focus();
		};
	});
</script>

<div
	class="fixed inset-0 z-50 flex items-center justify-center p-6"
	style="background:{overlay}"
	role="presentation"
	onclick={(e) => {
		if (e.target === e.currentTarget) onclose();
	}}
>
	<div
		bind:this={modalElement}
		tabindex="-1"
		class="rounded-xl flex flex-col"
		role="dialog"
		aria-modal="true"
		aria-label={bodyView ? bodyView.title : `LLM Usage Event — ${event.id}`}
		onkeydown={handleKeydown}
		style="background:{cardBg};border:1px solid {borderColor};width:min(900px,100%);max-height:80vh"
	>
		<div class="flex justify-between items-center px-5 py-4" style="border-bottom:1px solid {borderColor}">
			<div class="flex items-center gap-2">
				{#if bodyView}
					<button
						type="button"
						onclick={() => (bodyView = null)}
						class="rounded p-1 cursor-pointer"
						style="background:{surface2};color:{textMuted};border:1px solid {borderColor}"
						aria-label="Back"
					>
						<ChevronLeftIcon class="w-3.5 h-3.5" />
					</button>
				{/if}
				<span style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace">
					{bodyView ? bodyView.title : `LLM Usage Event — ${event.id}`}
				</span>
			</div>
			<button
				bind:this={closeButton}
				onclick={onclose}
				class="rounded p-1.5 cursor-pointer"
				style="background:{surface2};color:{textMuted};border:1px solid {borderColor}"
				aria-label="Close"
			>
				<XIcon class="w-4 h-4" />
			</button>
		</div>
		<div class="flex-1 overflow-auto p-5 modal-scroll" style="--modal-scroll-thumb:{scrollThumb};">
			{#if bodyView}
				{#if bodyView.loading}
					<div class="text-center" style="color:{textMuted};padding:2rem">Loading…</div>
				{:else if bodyView.error}
					<div class="rounded-lg p-4" style="background:{danger}15;border:1px solid {danger}40;color:{danger}">{bodyView.error}</div>
				{:else if bodyView.nodes.length}
					<JsonTree nodes={bodyView.nodes} {dark} />
				{:else}
					<pre class="body-pre" style="color:{textSecondary};">{bodyView.rawText}</pre>
				{/if}
			{:else}
				<div class="rounded-lg p-4 text-xs" style="background:{surface2};border:1px solid {borderColor};line-height:1.6;user-select:text">
					<div class="grid gap-x-3 gap-y-2" style="grid-template-columns:minmax(90px,max-content) minmax(0,1fr)">
						{#each fields as field (field.key)}
							<div style="color:{textMuted};font-family:monospace;word-break:break-word">{field.label}</div>
							<div style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">{displayValue(event[field.key])}</div>
						{/each}
						<div style="color:{textMuted};font-family:monospace;word-break:break-word">input_body_ref</div>
						<div>
							{#if event.input_body_ref}
								<button type="button" class="ev-btn" style="border-color:{borderColor};color:{accent}" onclick={() => openBody('input')}>View</button>
							{:else}
								<span style="color:{textMuted}">—</span>
							{/if}
						</div>
						<div style="color:{textMuted};font-family:monospace;word-break:break-word">output_body_ref</div>
						<div>
							{#if event.output_body_ref}
								<button type="button" class="ev-btn" style="border-color:{borderColor};color:{accent}" onclick={() => openBody('output')}>View</button>
							{:else}
								<span style="color:{textMuted}">—</span>
							{/if}
						</div>
						<div style="color:{textMuted};font-family:monospace;word-break:break-word">metadata_json</div>
						<div>
							<JsonTree nodes={metadataNodes} {dark} />
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.modal-scroll {
		scrollbar-width: thin;
		scrollbar-color: var(--modal-scroll-thumb) transparent;
	}
	.modal-scroll::-webkit-scrollbar {
		width: 6px;
	}
	.modal-scroll::-webkit-scrollbar-thumb {
		background: var(--modal-scroll-thumb);
		border-radius: 999px;
	}
	.modal-scroll::-webkit-scrollbar-track {
		background: transparent;
	}
	.ev-btn {
		padding: 0.2rem 0.6rem;
		border: 1px solid;
		border-radius: 6px;
		background: transparent;
		font: inherit;
		font-size: 0.74rem;
		cursor: pointer;
	}
	.body-pre {
		font-family: monospace;
		font-size: 12px;
		line-height: 1.6;
		white-space: pre-wrap;
		word-break: break-word;
		margin: 0;
		user-select: text;
	}
</style>
