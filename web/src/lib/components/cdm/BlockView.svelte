<script lang="ts">
	// Read-only rendering of one CDM block, dispatching on block.type (spec §2
	// block vocabulary). Recurses into children (callout) and items (list) via
	// self-import, matching InlineView's inline recursion.
	//
	// This is the editor's own lightweight in-browser preview, not the
	// authoritative rendering: the Typst-rendered SVG a preview action fetches
	// (design D4) is what reflects the active template and is what actually
	// publishes. This view exists so an author can see block structure and
	// content while editing, without triggering a server render on every
	// keystroke.
	import type { Block } from './types.js';
	import InlineView from './InlineView.svelte';
	import Self from './BlockView.svelte';

	let { block }: { block: Block } = $props();

	let headingTag = $derived(`h${Math.min(6, Math.max(1, block.level ?? 2))}` as const);
</script>

{#if block.type === 'paragraph'}
	<p><InlineView inline={block.content ?? []} /></p>
{:else if block.type === 'heading'}
	<svelte:element this={headingTag}><InlineView inline={block.content ?? []} /></svelte:element>
{:else if block.type === 'quote'}
	<blockquote class="cdm-quote"><InlineView inline={block.content ?? []} /></blockquote>
{:else if block.type === 'list'}
	<svelte:element this={block.ordered ? 'ol' : 'ul'}>
		{#each block.items ?? [] as item, i (i)}
			<li>
				{#each item as child (child.id)}
					<Self block={child} />
				{/each}
			</li>
		{/each}
	</svelte:element>
{:else if block.type === 'table'}
	<table class="cdm-table">
		{#if block.caption?.length}
			<caption><InlineView inline={block.caption} /></caption>
		{/if}
		<thead>
			<tr>
				{#each block.columns ?? [] as col (col.key)}
					<th style={col.align ? `text-align: ${col.align}` : undefined}>{col.title}</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each block.rows ?? [] as row, i (i)}
				<tr>
					{#each block.columns ?? [] as col (col.key)}
						<td style={col.align ? `text-align: ${col.align}` : undefined}
							><InlineView inline={row.cells[col.key] ?? []} /></td
						>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
{:else if block.type === 'equation'}
	<div class="cdm-equation" class:cdm-equation--inline={block.math && !block.math.display}>
		<code>{block.math?.original?.source ?? '(no source)'}</code>
		{#if block.math}
			<span class="cdm-equation-meta"
				>{block.math.original?.format ?? '?'} · {block.math.parse_status}</span
			>
		{/if}
	</div>
{:else if block.type === 'code'}
	<div class="cdm-code">
		{#if block.lang}<span class="cdm-code-lang">{block.lang}</span>{/if}
		<pre><code>{block.text ?? ''}</code></pre>
	</div>
{:else if block.type === 'image'}
	<figure class="cdm-figure">
		<img src={block.src} alt={block.alt ?? ''} />
		{#if block.caption?.length}
			<figcaption><InlineView inline={block.caption} /></figcaption>
		{/if}
	</figure>
{:else if block.type === 'callout'}
	<div class="cdm-callout" data-role={block.role ?? 'note'}>
		{#if block.title}<div class="cdm-callout-title">{block.title}</div>{/if}
		{#each block.children ?? [] as child (child.id)}
			<Self block={child} />
		{/each}
	</div>
{:else}
	<div class="cdm-unsupported">Unsupported block type "{block.type}" (id: {block.id})</div>
{/if}

<style>
	.cdm-quote {
		border-left: 3px solid rgba(127, 127, 127, 0.35);
		margin: 0;
		padding: 2px 0 2px 12px;
		font-style: italic;
	}
	/*
	 * The app's global stylesheet (Tailwind preflight) resets list-style to
	 * none and heading font-size/weight to inherit, which is correct for a
	 * design system building its own components but leaves this read-only
	 * preview's <ul>/<ol>/<h1-6> visually indistinguishable from plain text.
	 * Confirmed by screenshot during task 4.2 verification. Restoring
	 * defaults here, scoped to this component, rather than fighting preflight
	 * globally.
	 */
	ul,
	ol {
		margin: 0;
		padding-left: 1.5em;
	}
	ul {
		list-style: disc;
	}
	ol {
		list-style: decimal;
	}
	h1,
	h2,
	h3,
	h4,
	h5,
	h6 {
		font-weight: 700;
		margin: 0;
	}
	h1 {
		font-size: 1.6em;
	}
	h2 {
		font-size: 1.4em;
	}
	h3 {
		font-size: 1.2em;
	}
	h4 {
		font-size: 1.1em;
	}
	h5,
	h6 {
		font-size: 1em;
	}
	.cdm-table {
		border-collapse: collapse;
		width: 100%;
	}
	.cdm-table th,
	.cdm-table td {
		border: 1px solid rgba(127, 127, 127, 0.3);
		padding: 4px 8px;
	}
	.cdm-equation {
		font-family: ui-monospace, monospace;
		background: rgba(127, 127, 127, 0.08);
		border-radius: 6px;
		padding: 8px 12px;
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
	}
	.cdm-equation--inline {
		display: inline-flex;
	}
	.cdm-equation-meta {
		font-size: 0.75em;
		color: var(--cdm-muted, #6b7280);
		white-space: nowrap;
	}
	.cdm-code {
		position: relative;
	}
	.cdm-code-lang {
		position: absolute;
		top: 4px;
		right: 8px;
		font-size: 0.7em;
		text-transform: uppercase;
		color: var(--cdm-muted, #6b7280);
	}
	.cdm-code pre {
		background: rgba(127, 127, 127, 0.08);
		border-radius: 6px;
		padding: 10px 12px;
		overflow-x: auto;
		margin: 0;
	}
	.cdm-figure {
		margin: 0;
	}
	.cdm-figure img {
		max-width: 100%;
	}
	.cdm-figure figcaption {
		font-size: 0.85em;
		color: var(--cdm-muted, #6b7280);
		margin-top: 4px;
	}
	.cdm-callout {
		border: 1px solid rgba(127, 127, 127, 0.3);
		border-radius: 8px;
		padding: 10px 14px;
	}
	.cdm-callout[data-role='warning'],
	.cdm-callout[data-role='caution'] {
		background: rgba(217, 119, 6, 0.08);
		border-color: rgba(217, 119, 6, 0.4);
	}
	.cdm-callout[data-role='important'] {
		background: rgba(220, 38, 38, 0.08);
		border-color: rgba(220, 38, 38, 0.4);
	}
	.cdm-callout[data-role='tip'] {
		background: rgba(5, 150, 105, 0.08);
		border-color: rgba(5, 150, 105, 0.4);
	}
	.cdm-callout-title {
		font-weight: 600;
		margin-bottom: 4px;
	}
	.cdm-unsupported {
		color: #b91c1c;
		font-family: ui-monospace, monospace;
		font-size: 0.85em;
		border: 1px dashed #b91c1c;
		border-radius: 6px;
		padding: 6px 10px;
	}
</style>
