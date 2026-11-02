<!-- A property's help tip: an info icon at the upper right of its value
     editor; hovering over it or focusing it shows the tip. -->
<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';

	let { id, tip, name }: { id: string; tip: string; name: string } = $props();
	let open = $state(false);
</script>

<span class="help-tip">
	<button
		type="button"
		class="icon"
		aria-label={m.wpi_prop_help({ name })}
		aria-describedby={id}
		onmouseenter={() => (open = true)}
		onmouseleave={() => (open = false)}
		onfocus={() => (open = true)}
		onblur={() => (open = false)}
		onclick={() => (open = !open)}
		onkeydown={(e) => {
			if (e.key === 'Escape') open = false;
		}}>i</button
	>
	<span {id} role="tooltip" class="tip" class:open>{tip}</span>
</span>

<style>
	.help-tip {
		position: absolute;
		top: -7px;
		right: -5px;
		z-index: 2;
	}
	.icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 14px;
		height: 14px;
		padding: 0;
		border-radius: 50%;
		border: 1px solid var(--cv-accent, #6366f1);
		background: var(--cv-panel, white);
		color: var(--cv-accent, #6366f1);
		font: italic 700 9px/1 Georgia, serif;
		cursor: help;
	}
	.icon:focus-visible {
		outline: 2px solid var(--cv-accent, #6366f1);
		outline-offset: 1px;
	}
	.tip {
		display: none;
		position: absolute;
		right: 0;
		top: 18px;
		width: max-content;
		max-width: 240px;
		padding: 6px 8px;
		border-radius: 4px;
		border: 1px solid var(--cv-border, #d1d5db);
		background: var(--cv-surface, white);
		color: var(--cv-text, inherit);
		font-size: 11px;
		line-height: 1.4;
		box-shadow: 0 4px 12px rgb(0 0 0 / 0.18);
		white-space: pre-line;
	}
	.tip.open {
		display: block;
	}
</style>
