<script lang="ts">
	// Renders a record as name-value pairs. See name-value.ts for the data model and
	// KnowledgeStore/doc-repo/modules/module-name-value-visualizer.md for usage.
	import { normalizeFields, type NameValueInput } from './name-value';

	let {
		fields,
		labelWidth = '13rem',
		emptyText,
		ariaLabel
	}: {
		fields: NameValueInput[];
		// Upper bound of the name column; it shrinks to 9rem before wrapping.
		labelWidth?: string;
		// When set, fields without values stay in the list, dimmed, showing this text.
		emptyText?: string;
		ariaLabel?: string;
	} = $props();

	const visible = $derived(normalizeFields(fields, emptyText != null));
</script>

<div class="name-value-box">
	<dl class="name-value select-text" style={`--nv-label-width:${labelWidth}`} aria-label={ariaLabel}>
		{#each visible as field (field.key)}
			{@const empty = field.values.length === 0}
			<dt class:empty>{field.label}</dt>
			<dd class:empty>
				{#if empty}
					<span>{emptyText}</span>
				{:else}
					{#each field.values as item, index (index)}
						<div class:mono={item.mono}>
							{#if item.href}
								<a href={item.href} target="_blank" rel="noreferrer">{item.text}</a>
							{:else if item.onClick}
								<button class="text-link" type="button" onclick={item.onClick}>{item.text}</button>
							{:else}
								<span lang={item.lang || undefined}>{item.text}</span>
							{/if}
						</div>
					{/each}
				{/if}
			</dd>
		{/each}
	</dl>
</div>

<style>
	/* The compact layout follows the list's own width, so it also applies in side panels. */
	.name-value-box {
		container-type: inline-size;
	}
	.name-value {
		display: grid;
		grid-template-columns: minmax(9rem, var(--nv-label-width)) minmax(0, 1fr);
		gap: 0.35rem 1.25rem;
		margin: 0;
		font-size: 0.875rem;
		line-height: 1.6;
		overflow-wrap: anywhere;
	}
	dt {
		text-align: right;
		font-weight: 650;
	}
	dd {
		text-align: left;
		margin: 0;
		min-width: 0;
		white-space: pre-wrap;
	}
	.empty {
		opacity: 0.55;
	}
	.mono {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.8125rem;
	}
	a,
	.text-link {
		color: var(--accent, currentColor);
		text-decoration: none;
	}
	a:hover,
	.text-link:hover {
		text-decoration: underline;
	}
	.text-link {
		padding: 0;
		border: 0;
		background: transparent;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	a:focus-visible,
	.text-link:focus-visible {
		outline: 2px solid var(--accent, currentColor);
		outline-offset: 3px;
	}
	@container (max-width: 600px) {
		.name-value {
			grid-template-columns: minmax(6rem, 35%) minmax(0, 1fr);
			gap: 0.4rem 0.75rem;
		}
	}
</style>
