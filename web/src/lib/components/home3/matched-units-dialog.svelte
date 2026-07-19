<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import { buildMatchedUnitRows, matchedUnitFocusTarget, matchedUnitLabel } from './doc-review-json-dialog.js';

	let {
		title,
		units,
		initialSelected,
		dark = true,
		onclose,
		onFocusUnit
	}: {
		title: string;
		units: unknown[];
		initialSelected?: number;
		dark?: boolean;
		onclose: () => void;
		onFocusUnit?: (recordId: number, lineNumbers: number[]) => void;
	} = $props();

	let cardBg = $derived(dark ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(dark ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(dark ? '#2D3348' : '#E4E6EB');
	let accent = $derived(dark ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(dark ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');
	let overlay = $derived(dark ? '#0D1117E6' : '#00000066');
	let scrollThumb = $derived(dark ? '#2A3140' : '#D7CFB8');

	let selected = $state<number | null>(initialSelected ?? null);
	let selectedRows = $derived(selected != null ? buildMatchedUnitRows(units[selected]) : []);
	let dialogTitle = $derived(selected != null ? `Matched — ${matchedUnitLabel(units[selected], selected)}` : title);

	function selectUnit(i: number) {
		selected = i;
		const target = matchedUnitFocusTarget(units[i]);
		console.info('[matched-units-dialog] unit selected', {
			index: i,
			label: matchedUnitLabel(units[i], i),
			focusTarget: target
		});
		if (target) onFocusUnit?.(target.recordId, target.lineNumbers);
	}

	let modalElement = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let returnFocusElement: HTMLElement | null = null;

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault();
			if (selected != null) selected = null;
			else onclose();
			return;
		}
		if (event.key !== 'Tab' || !modalElement) return;
		const focusable = Array.from(
			modalElement.querySelectorAll<HTMLElement>(
				'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
			)
		);
		if (!focusable.length) {
			event.preventDefault();
			modalElement.focus();
			return;
		}
		const first = focusable[0];
		const last = focusable[focusable.length - 1];
		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
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
	onclick={(event) => {
		if (event.target === event.currentTarget) onclose();
	}}
>
	<div
		bind:this={modalElement}
		tabindex="-1"
		class="rounded-xl flex flex-col"
		role="dialog"
		aria-modal="true"
		aria-label={dialogTitle}
		onkeydown={handleKeydown}
		style="background:{cardBg};border:1px solid {borderColor};width:min(900px,100%);max-height:80vh"
	>
		<div class="flex justify-between items-center px-5 py-4" style="border-bottom:1px solid {borderColor}">
			<div class="flex items-center gap-2">
				{#if selected != null}
					<button
						type="button"
						onclick={() => (selected = null)}
						class="rounded p-1 cursor-pointer"
						style="background:{surface2};color:{textMuted};border:1px solid {borderColor}"
						aria-label="Back to list"
					>
						<ChevronLeftIcon class="w-3.5 h-3.5" />
					</button>
				{/if}
				<span style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace">{dialogTitle}</span>
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
			{#if !units.length}
				<div class="text-center" style="color:{textMuted};padding:2rem">No data.</div>
			{:else if selected == null}
				<ul class="mu-list">
					{#each units as unit, i (i)}
						<li>
							<button
								type="button"
								class="mu-item"
								style="border:1px solid {borderColor};color:{textSecondary}"
								onclick={() => selectUnit(i)}
							>
								<span style="color:{accent};font-family:monospace">{matchedUnitLabel(unit, i)}</span>
							</button>
						</li>
					{/each}
				</ul>
			{:else}
				<div class="rounded-lg p-4 text-xs" style="background:{surface2};border:1px solid {borderColor};line-height:1.6;user-select:text">
					<div class="grid gap-x-3 gap-y-2" style="grid-template-columns:minmax(90px,max-content) minmax(0,1fr)">
						{#each selectedRows as row, i (i)}
							<div style="color:{textMuted};font-family:monospace;word-break:break-word">{row.label}</div>
							{#if row.sourceContext}
								<div class="sc-table" style="border:1px solid {borderColor};">
									<div class="sc-head" style="color:{textMuted};border-bottom:1px solid {borderColor};">
										<span>Line</span>
										<span>Content</span>
									</div>
									{#each row.sourceContext as span, j (j)}
										<div class="sc-row" style="color:{textSecondary};{j > 0 ? `border-top:1px solid ${borderColor};` : ''}">
											<span style="color:{textMuted};font-family:monospace">{span.lineNumber}</span>
											<span style="white-space:pre-wrap;word-break:break-word">{span.content}</span>
										</div>
									{/each}
								</div>
							{:else}
								<div style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">{row.value}</div>
							{/if}
						{/each}
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
	.mu-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.mu-item {
		display: block;
		width: 100%;
		text-align: left;
		background: transparent;
		border-radius: 6px;
		padding: 0.5rem 0.75rem;
		margin: 0.3rem 0;
		cursor: pointer;
		font-size: 0.8rem;
	}
	.sc-table {
		border-radius: 6px;
		overflow: hidden;
	}
	.sc-head,
	.sc-row {
		display: grid;
		grid-template-columns: minmax(40px, max-content) minmax(0, 1fr);
		gap: 0.6rem;
		padding: 0.35rem 0.55rem;
	}
	.sc-head {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
</style>
