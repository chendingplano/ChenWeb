<!-- Options available for a property value editor. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';

	let { id, tip, name }: { id: string; tip: string; name: string } = $props();
	let open = $state(false);
	let showTip = $state(false);
</script>

<span class="help-tip" onfocusout={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) { open = false; showTip = false; } }} onkeydown={(e) => { if (e.key === 'Escape') { open = false; showTip = false; } }}>
	<button type="button" class="icon" aria-label={m.wpi_string_options()} title={m.wpi_string_options()} aria-haspopup="menu" aria-expanded={open} onclick={() => { open = !open; showTip = false; }}>⋮</button>
	{#if open}
		<span class="menu" role="menu" aria-label={name}>
			<button type="button" role="menuitem" aria-describedby={showTip ? id : undefined} onclick={() => (showTip = !showTip)}>{m.wpi_create_element_help_tip()}</button>
			{#if showTip}<span {id} role="tooltip" class="tip">{tip}</span>{/if}
		</span>
	{/if}
</span>

<style>
	.help-tip { position: absolute; top: 0; right: 0; z-index: 5; }
	.icon { display: flex; align-items: center; justify-content: center; box-sizing: border-box; width: 10px; height: 16px; padding: 0; border: 1px solid var(--cv-accent, #6366f1); border-radius: 2px; background: var(--cv-panel, white); color: var(--cv-accent, #6366f1); font-size: 12px; line-height: 1; cursor: pointer; }
	.icon:focus-visible, .menu button:focus-visible { outline: 2px solid var(--cv-accent, #6366f1); outline-offset: 1px; }
	.menu { display: grid; position: absolute; right: 0; top: 19px; width: max-content; max-width: 240px; padding: 4px; border-radius: 4px; border: 1px solid var(--cv-border, #d1d5db); background: var(--cv-surface, white); color: var(--cv-text, inherit); box-shadow: 0 4px 12px rgb(0 0 0 / 0.18); }
	.menu button { border: 0; padding: 4px 8px; text-align: left; background: transparent; color: inherit; cursor: pointer; font-size: 12px; }
	.menu button:hover { background: var(--cv-hover, #f3f4f6); }
	.tip { max-width: 224px; padding: 6px 8px; font-size: 11px; line-height: 1.4; white-space: pre-line; }
</style>
