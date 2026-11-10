<script lang="ts">
	// Read-only Markdown viewer. See markdown.ts for the safety rules and
	// KnowledgeStore/doc-repo/modules/module-markdown-viewer.md for usage.
	import { renderMarkdown } from './markdown';

	let {
		source,
		gfm = true,
		breaks = false,
		allowImages = false,
		emptyText,
		ariaLabel
	}: {
		source: string;
		gfm?: boolean;
		breaks?: boolean;
		allowImages?: boolean;
		// Shown, dimmed, when source is blank.
		emptyText?: string;
		ariaLabel?: string;
	} = $props();

	const html = $derived(renderMarkdown(source, { gfm, breaks, allowImages }));
	const blank = $derived(!source || source.trim() === '');
</script>

{#if blank && emptyText}
	<p class="md-empty">{emptyText}</p>
{:else}
	<div class="md select-text" aria-label={ariaLabel} role={ariaLabel ? 'document' : undefined}>
		{@html html}
	</div>
{/if}

<style>
	.md {
		font-size: 0.9375rem;
		line-height: 1.65;
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.md-empty {
		opacity: 0.55;
		font-size: 0.875rem;
	}
	.md :global(h1),
	.md :global(h2),
	.md :global(h3),
	.md :global(h4) {
		font-weight: 650;
		line-height: 1.3;
		margin: 1.2em 0 0.5em;
	}
	.md :global(h1) { font-size: 1.5rem; }
	.md :global(h2) { font-size: 1.25rem; }
	.md :global(h3) { font-size: 1.1rem; }
	.md :global(h4) { font-size: 1rem; }
	.md :global(:first-child) { margin-top: 0; }
	.md :global(p),
	.md :global(ul),
	.md :global(ol),
	.md :global(pre),
	.md :global(blockquote),
	.md :global(table) {
		margin: 0.6em 0;
	}
	/* Tailwind's preflight sets list-style: none, so restore markers explicitly. */
	.md :global(ul),
	.md :global(ol) { padding-left: 1.5rem; }
	.md :global(ul) { list-style: disc outside; }
	.md :global(ul ul) { list-style-type: circle; }
	.md :global(ul ul ul) { list-style-type: square; }
	.md :global(ol) { list-style: decimal outside; }
	.md :global(li) { display: list-item; }
	.md :global(li > ul),
	.md :global(li > ol) { margin: 0.25em 0; }
	.md :global(code) {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.85em;
		background: color-mix(in srgb, currentColor 8%, transparent);
		padding: 0.1em 0.3em;
		border-radius: 0.25rem;
	}
	.md :global(pre) {
		overflow-x: auto;
		padding: 0.75rem 1rem;
		border-radius: 0.5rem;
		background: color-mix(in srgb, currentColor 6%, transparent);
	}
	.md :global(pre code) { background: none; padding: 0; }
	.md :global(blockquote) {
		padding-left: 1rem;
		border-left: 3px solid color-mix(in srgb, currentColor 25%, transparent);
		opacity: 0.85;
	}
	.md :global(table) { border-collapse: collapse; display: block; overflow-x: auto; }
	.md :global(th),
	.md :global(td) {
		border: 1px solid color-mix(in srgb, currentColor 20%, transparent);
		padding: 0.3rem 0.6rem;
	}
	.md :global(a) { text-decoration: underline; }
	.md :global(img) { max-width: 100%; height: auto; }
</style>
