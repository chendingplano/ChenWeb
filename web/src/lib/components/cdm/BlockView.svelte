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
	import { getContext } from 'svelte';
	import type { Block } from './types.js';
	import { CALLOUT_ROLES, EQUATION_FORMATS } from './types.js';
	import InlineView from './InlineView.svelte';
	import InlineEditor from './InlineEditor.svelte';
	import TableEditor from './TableEditor.svelte';
	import Self from './BlockView.svelte';
	import { ALLOCATE_ID_CONTEXT_KEY, type BlockIdHint } from './block-id.js';
	import { addListItem, removeListItem } from './list-ops.js';

	// editable switches paragraph/heading/quote from InlineView (read-only)
	// to InlineEditor (a real ProseMirror instance, task group 5), and turns
	// on the structural editors for table/list/code/equation/image/callout
	// (task group 6). Defaults to false so this component still serves as a
	// plain read-only preview wherever one is wanted; BlockList, the actual
	// editing surface, passes true explicitly.
	//
	// block is $bindable: InlineEditor/TableEditor bind into its nested
	// properties (block.content, row.cells[...], block.caption, ...), and
	// Svelte's own dev-mode ownership check flags exactly this pattern --
	// "component X passed a bind: into property Y of a prop its own parent
	// did not declare bindable" -- when an intermediate component in the
	// chain holds a plain, non-bindable reference. Declaring it bindable
	// here and threading bind:block through BlockList and every recursive
	// Self/TableEditor call below makes the ownership chain match what is
	// actually happening (deep mutation through the same reactive object),
	// rather than relying on it working anyway through Svelte 5's deep
	// $state reactivity without being declared.
	let { block = $bindable(), editable = false }: { block: Block; editable?: boolean } = $props();

	let headingTag = $derived(`h${Math.min(6, Math.max(1, block.level ?? 2))}` as const);

	// Only list items currently mint new blocks from within a nested
	// BlockView; provided by BlockList (see block-id.ts's doc comment on
	// ALLOCATE_ID_CONTEXT_KEY for why this can't just be computed locally).
	const allocateId = getContext<((hint: BlockIdHint) => string) | undefined>(
		ALLOCATE_ID_CONTEXT_KEY
	);

	function doAddListItem() {
		if (!allocateId) return;
		Object.assign(block, addListItem(block, allocateId));
	}
	function doRemoveListItem(index: number) {
		Object.assign(block, removeListItem(block, index));
	}
</script>

{#if block.type === 'paragraph'}
	{#if editable && block.content}
		<InlineEditor bind:content={block.content} as="paragraph" />
	{:else}
		<p><InlineView inline={block.content ?? []} /></p>
	{/if}
{:else if block.type === 'heading'}
	{#if editable && block.content}
		<InlineEditor bind:content={block.content} as="heading" level={block.level ?? 2} />
	{:else}
		<svelte:element this={headingTag}><InlineView inline={block.content ?? []} /></svelte:element>
	{/if}
{:else if block.type === 'quote'}
	{#if editable && block.content}
		<InlineEditor bind:content={block.content} as="quote" />
	{:else}
		<blockquote class="cdm-quote"><InlineView inline={block.content ?? []} /></blockquote>
	{/if}
{:else if block.type === 'list'}
	<svelte:element this={block.ordered ? 'ol' : 'ul'}>
		{#each block.items ?? [] as item, i (i)}
			<li>
				{#each item as child, j (child.id)}
					<Self bind:block={block.items![i][j]} {editable} />
				{/each}
				{#if editable}
					<button type="button" class="cdm-list-remove-item" onclick={() => doRemoveListItem(i)}
						>Remove item</button
					>
				{/if}
			</li>
		{/each}
	</svelte:element>
	{#if editable}
		<label class="cdm-list-ordered-toggle">
			<input
				type="checkbox"
				checked={block.ordered ?? false}
				onchange={(e) => (block.ordered = e.currentTarget.checked)}
			/>
			Ordered
		</label>
		<button type="button" onclick={doAddListItem}>+ Item</button>
	{/if}
{:else if block.type === 'table'}
	{#if editable}
		<TableEditor bind:block />
	{:else}
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
	{/if}
{:else if block.type === 'equation'}
	{#if editable && block.math}
		<div class="cdm-equation cdm-equation-editor">
			<label>
				<input
					type="checkbox"
					checked={block.math.display}
					onchange={(e) => (block.math!.display = e.currentTarget.checked)}
				/>
				Display (block) equation
			</label>
			<select
				value={block.math.original?.format ?? 'typst'}
				onchange={(e) => {
					if (!block.math!.original) block.math!.original = { format: 'typst', source: '' };
					block.math!.original.format = e.currentTarget.value;
				}}
			>
				{#each EQUATION_FORMATS as format (format)}
					<option value={format}>{format}</option>
				{/each}
			</select>
			<textarea
				class="cdm-equation-source"
				value={block.math.original?.source ?? ''}
				oninput={(e) => {
					if (!block.math!.original) block.math!.original = { format: 'typst', source: '' };
					block.math!.original.source = e.currentTarget.value;
				}}
			></textarea>
		</div>
	{:else}
		<div class="cdm-equation" class:cdm-equation--inline={block.math && !block.math.display}>
			<code>{block.math?.original?.source ?? '(no source)'}</code>
			{#if block.math}
				<span class="cdm-equation-meta"
					>{block.math.original?.format ?? '?'} · {block.math.parse_status}</span
				>
			{/if}
		</div>
	{/if}
{:else if block.type === 'code'}
	{#if editable}
		<div class="cdm-code cdm-code-editor">
			<input
				type="text"
				class="cdm-code-lang-input"
				placeholder="language"
				value={block.lang ?? ''}
				oninput={(e) => (block.lang = e.currentTarget.value)}
			/>
			<textarea
				class="cdm-code-textarea"
				value={block.text ?? ''}
				oninput={(e) => (block.text = e.currentTarget.value)}
			></textarea>
		</div>
	{:else}
		<div class="cdm-code">
			{#if block.lang}<span class="cdm-code-lang">{block.lang}</span>{/if}
			<pre><code>{block.text ?? ''}</code></pre>
		</div>
	{/if}
{:else if block.type === 'image'}
	<figure class="cdm-figure">
		{#if editable}
			<input
				type="text"
				placeholder="Image source (path or URL)"
				value={block.src ?? ''}
				oninput={(e) => (block.src = e.currentTarget.value)}
			/>
			<input
				type="text"
				placeholder="Alt text"
				value={block.alt ?? ''}
				oninput={(e) => (block.alt = e.currentTarget.value)}
			/>
		{:else}
			<img src={block.src} alt={block.alt ?? ''} />
		{/if}
		{#if editable}
			{#if block.caption}
				<figcaption><InlineEditor bind:content={block.caption} as="plain" /></figcaption>
			{:else}
				<button type="button" onclick={() => (block.caption = [])}>+ Caption</button>
			{/if}
		{:else if block.caption?.length}
			<figcaption><InlineView inline={block.caption} /></figcaption>
		{/if}
	</figure>
{:else if block.type === 'callout'}
	<div class="cdm-callout" data-role={block.role ?? 'note'}>
		{#if editable}
			<div class="cdm-callout-editor-header">
				<select value={block.role ?? 'note'} onchange={(e) => (block.role = e.currentTarget.value)}>
					{#each CALLOUT_ROLES as role (role)}
						<option value={role}>{role}</option>
					{/each}
				</select>
				<input
					type="text"
					placeholder="Title"
					value={block.title ?? ''}
					oninput={(e) => (block.title = e.currentTarget.value)}
				/>
			</div>
		{:else if block.title}
			<div class="cdm-callout-title">{block.title}</div>
		{/if}
		{#each block.children ?? [] as child, i (child.id)}
			<Self bind:block={block.children![i]} {editable} />
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
	.cdm-list-remove-item {
		font-size: 0.75em;
		opacity: 0.6;
		margin-left: 4px;
	}
	.cdm-list-remove-item:hover {
		opacity: 1;
	}
	.cdm-list-ordered-toggle {
		font-size: 0.8em;
		margin-right: 8px;
	}
	.cdm-equation-editor {
		flex-direction: column;
		align-items: stretch;
		gap: 6px;
	}
	.cdm-equation-source,
	.cdm-code-textarea {
		font-family: ui-monospace, monospace;
		width: 100%;
		min-height: 3em;
		resize: vertical;
	}
	.cdm-code-editor {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.cdm-code-lang-input {
		font-size: 0.8em;
		width: 8em;
	}
	.cdm-callout-editor-header {
		display: flex;
		gap: 6px;
		margin-bottom: 6px;
	}
</style>
