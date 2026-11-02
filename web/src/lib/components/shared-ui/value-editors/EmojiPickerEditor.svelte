<script lang="ts">
	import { Emoji, EmojiSearchPicker, getEmoji } from '@chendingplano/shared';
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import { tick } from 'svelte';
	import type { ValueEditorProps } from './types';
	import './value-editors.css';

	let { id, label, value, check, onChange }: ValueEditorProps = $props();
	const selected = $derived(typeof value === 'string' ? value : '');
	const locale = $derived(getLocale());
	let open = $state(false);
	let error = $state<string | null>(null);
	let name = $state('');
	let trigger: HTMLButtonElement;
	let container: HTMLDivElement;
	$effect(() => {
		const current = selected, language = locale;
		name = '';
		error = null;
		if (!current) return;
		const ctl = new AbortController();
		getEmoji(current, ctl.signal).then((e) => name = language.startsWith('zh') ? e.name_zh : e.name_en).catch(() => {});
		return () => ctl.abort();
	});
	function choose(next: string) {
		const problem = check?.(next) ?? null;
		if (problem) { error = problem; return; }
		error = null; onChange(next); open = false; trigger?.focus();
	}
	function keydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open) { e.stopPropagation(); open = false; error = null; trigger?.focus(); }
	}
	async function stay() { error = null; open = true; await tick(); container.querySelector<HTMLInputElement>('input[type="search"]')?.focus(); }
</script>

<div bind:this={container} class="select-text" onkeydown={keydown} role="group" aria-label={label}>
	<div class="ve-row">
		<button {id} bind:this={trigger} type="button" class="ve-button emoji-trigger" aria-expanded={open} aria-label={label} onclick={() => open = !open}>
			{#if selected}<Emoji value={selected} size={20} decorative /><span>{name || selected}</span>{:else}{m.emoji_picker_choose()}{/if}
		</button>
		{#if selected}<button type="button" class="ve-button" onclick={() => { error = null; onChange(undefined); }}>{m.emoji_picker_clear()}</button>{/if}
	</div>
	{#if open}
		<EmojiSearchPicker value={selected} onSelect={choose} {locale} labels={{placeholder:m.emoji_search_placeholder(), empty:m.emoji_search_empty(), error:m.emoji_search_error(), loading:m.emoji_search_loading(), all:m.emoji_search_all(), category:m.emoji_search_category(), more:m.emoji_search_more(), attribution:m.emoji_search_attribution()}} />
	{/if}
	{#if error}
		<p class="ve-error" role="alert">{error}</p>
		<div class="ve-row"><button type="button" class="ve-button" onclick={() => {error = null; open = false; trigger?.focus();}}>{m.wpi_prop_abandon_edit()}</button><button type="button" class="ve-button" onclick={stay}>{m.wpi_prop_stay_edit()}</button></div>
	{/if}
</div>
<style>.emoji-trigger { display: inline-flex; align-items: center; gap: .35rem; }</style>
