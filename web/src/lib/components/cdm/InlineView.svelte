<script lang="ts">
	// Read-only rendering of CDM inline content (spec §2 inline vocabulary).
	// This is the editor's own lightweight preview, not the authoritative
	// rendering -- that is the Typst-rendered SVG a preview action fetches
	// from the server (design D4), which alone reflects the active template.
	import type { Inline } from './types.js';
	import Self from './InlineView.svelte';

	let { inline }: { inline: Inline[] } = $props();
</script>

{#each inline as node (node)}
	{#if node.type === 'text'}{node.text ?? ''}
	{:else if node.type === 'line_break'}<br />
	{:else if node.type === 'strong'}<strong><Self inline={node.content ?? []} /></strong>
	{:else if node.type === 'emphasis'}<em><Self inline={node.content ?? []} /></em>
	{:else if node.type === 'code'}<code class="cdm-inline-code">{node.text ?? ''}</code>
	{:else if node.type === 'link'}<a href={node.url} target="_blank" rel="noopener noreferrer"
			><Self inline={node.content ?? []} /></a
		>
	{:else if node.type === 'math'}<span class="cdm-inline-math" title="equation"
			>${node.math?.original?.source ?? node.math?.normalized?.op ?? '?'}$</span
		>
	{:else if node.type === 'citation'}<span class="cdm-inline-citation"
			>[{node.citation_key}{node.locator ? `, ${node.locator}` : ''}]</span
		>
	{:else if node.type === 'cross_reference'}<span
			class="cdm-inline-xref"
			title={node.target
				? `→ ${node.target.document_key ? node.target.document_key + ':' : ''}${node.target.block_id}`
				: ''}
			>{#if node.content?.length}<Self inline={node.content} />{:else}{node.target?.block_id ??
					'?'}{/if}</span
		>
	{:else}<span class="cdm-inline-unknown" title={`unsupported inline type "${node.type}"`}
			>[{node.type}]</span
		>{/if}
{/each}

<style>
	/* The app's global stylesheet (Tailwind preflight) strips default <a>
	   color/underline; restored here so a link is visually distinguishable
	   from surrounding text. Found during task 4.2 screenshot verification. */
	a {
		color: #2563eb;
		text-decoration: underline;
	}
	.cdm-inline-code {
		font-family: ui-monospace, monospace;
		background: rgba(127, 127, 127, 0.12);
		border-radius: 3px;
		padding: 0 4px;
	}
	.cdm-inline-math {
		font-family: ui-monospace, monospace;
		font-style: italic;
	}
	.cdm-inline-citation {
		color: var(--cdm-muted, #6b7280);
	}
	.cdm-inline-xref {
		text-decoration: underline dotted;
		cursor: help;
	}
	.cdm-inline-unknown {
		color: #b91c1c;
		font-family: ui-monospace, monospace;
		font-size: 0.85em;
	}
</style>
