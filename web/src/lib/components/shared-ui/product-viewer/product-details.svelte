<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';

	let { product, darkMode = true }: { product: any | null; darkMode?: boolean } = $props();

	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textMuted = $derived(darkMode ? '#64748B' : '#9CA3AF');
</script>

{#if product}
	<div class="rounded-xl p-4" style="background:{darkMode ? '#252A3A' : '#ECEEF2'}; border:1px solid {borderColor};">
		<h2 class="break-words text-sm font-semibold" style="color:{textPrimary};">{product.product_name || product.product_rel_id || m.products_product()}</h2>
		<dl class="mt-4 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
			<dt style="color:{textMuted};">{m.products_product_type()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.product_type || '—'}</dd>
			<dt style="color:{textMuted};">{m.kb_extraction_confidence()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{Math.round((Number(product.confidence) || 0) * 100)}%</dd>
			<dt style="color:{textMuted};">{m.kb_extraction_object_id()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.product_rel_id || '—'}</dd>
			<dt style="color:{textMuted};">{m.kb_extraction_lines()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{#each product.evidence_lines ?? [] as line, i}{i ? ', ' : ''}{typeof line === 'object' ? line.line_number ?? line.line : line}{/each}{#if !product.evidence_lines?.length}—{/if}</dd>
			{#if product.create_time}<dt style="color:{textMuted};">{m.kb_extraction_create_time()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{new Date(product.create_time).toLocaleString()}</dd>{/if}
			{#if product.relation_summary}<dt style="color:{textMuted};">{m.products_relation_summary()}</dt><dd class="m-0 break-words whitespace-pre-wrap" style="color:{textPrimary};">{product.relation_summary}</dd>{/if}
			{#if product.relation_summary_en}<dt style="color:{textMuted};">{m.products_relation_summary_english()}</dt><dd class="m-0 break-words whitespace-pre-wrap" style="color:{textPrimary};">{product.relation_summary_en}</dd>{/if}
			<dt style="color:{textMuted};">{m.products_canonical_name()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.canonical_name || '—'}</dd>
			{#if product.canonical_name_en}<dt style="color:{textMuted};">{m.products_canonical_name_english()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.canonical_name_en}</dd>{/if}
			<dt style="color:{textMuted};">{m.products_requirement_text()}</dt><dd class="m-0 break-words whitespace-pre-wrap" style="color:{textPrimary};">{product.requirement_text || '—'}</dd>
			{#if product.requirement_text_en}<dt style="color:{textMuted};">{m.products_requirement_text_english()}</dt><dd class="m-0 break-words whitespace-pre-wrap" style="color:{textPrimary};">{product.requirement_text_en}</dd>{/if}
			<dt style="color:{textMuted};">{m.products_evidence_quote()}</dt><dd class="m-0 break-words whitespace-pre-wrap" style="color:{textPrimary};">{product.evidence_quote || '—'}</dd>
			{#if product.obligation_level}<dt style="color:{textMuted};">{m.products_obligation_level()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.obligation_level}</dd>{/if}
			<dt style="color:{textMuted};">{m.products_confidence_reason()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.confidence_reason || '—'}</dd>
			{#if product.conditions?.length}<dt style="color:{textMuted};">{m.products_conditions()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.conditions.join(', ')}</dd>{/if}
			{#if product.parameters?.length}<dt style="color:{textMuted};">{m.products_parameters()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.parameters.join(', ')}</dd>{/if}
			{#if product.responsible_actor}<dt style="color:{textMuted};">{m.products_responsible_actor()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.responsible_actor}</dd>{/if}
			{#if product.exceptions?.length}<dt style="color:{textMuted};">{m.products_exceptions()}</dt><dd class="m-0 break-words" style="color:{textPrimary};">{product.exceptions.join(', ')}</dd>{/if}
		</dl>
	</div>
{/if}
