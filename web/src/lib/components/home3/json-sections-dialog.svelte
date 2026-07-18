<script lang="ts">
	import XIcon from '@lucide/svelte/icons/x';
	import type { JsonDialogSection } from './doc-review-json-dialog.js';

	let {
		title,
		sections,
		dark = true,
		onclose
	}: { title: string; sections: JsonDialogSection[]; dark?: boolean; onclose: () => void } = $props();

	let cardBg = $derived(dark ? '#1F2333' : '#FFFFFF');
	let surface2 = $derived(dark ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(dark ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(dark ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(dark ? '#94A3B8' : '#6B7280');
	let textMuted = $derived(dark ? '#64748B' : '#9CA3AF');
	let overlay = $derived(dark ? '#0D1117E6' : '#00000066');
	let scrollThumb = $derived(dark ? '#2A3140' : '#D7CFB8');

	let modalElement = $state<HTMLDivElement | null>(null);
	let closeButton = $state<HTMLButtonElement | null>(null);
	let returnFocusElement: HTMLElement | null = null;

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault();
			onclose();
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
		aria-label={title}
		onkeydown={handleKeydown}
		style="background:{cardBg};border:1px solid {borderColor};width:min(900px,100%);max-height:80vh"
	>
		<div class="flex justify-between px-5 py-4" style="border-bottom:1px solid {borderColor}">
			<span style="font-size:14px;font-weight:600;color:{textPrimary};font-family:monospace">{title}</span>
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
			{#if !sections.length}
				<div class="text-center" style="color:{textMuted};padding:2rem">No data.</div>
			{:else}
				<div class="space-y-4 text-xs" style="line-height:1.6;user-select:text">
					{#each sections as section, index (index)}
						<div class="rounded-lg p-4" style="background:{surface2};border:1px solid {borderColor}">
							<div class="grid gap-x-3 gap-y-2" style="grid-template-columns:minmax(90px,max-content) minmax(0,1fr)">
								{#each section.rows as row, i (i)}
									<div
										style="color:{textMuted};font-family:monospace;word-break:break-word;{row.indent ? 'padding-left:1rem;' : ''}"
									>{row.label}</div>
									<div style="color:{textSecondary};word-break:break-word;white-space:pre-wrap">{row.value}</div>
								{/each}
							</div>
						</div>
					{/each}
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
</style>
