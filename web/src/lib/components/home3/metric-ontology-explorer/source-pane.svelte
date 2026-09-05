<script lang="ts">
	import SharedPdfViewer from '$lib/components/home3/shared-pdf-viewer.svelte';
	import { getMetricWiki } from '$lib/services/metricWikiService';
	import type { ExplorerTokens } from './theme';

	let {
		tokens,
		metricId = '',
		evidenceSpans = [],
		evidenceLabel = ''
	}: {
		tokens: ExplorerTokens;
		metricId?: string;
		evidenceSpans?: string[];
		evidenceLabel?: string;
	} = $props();

	type Src = { inputId: number; fileUrl: string; title: string };
	let resolving = $state(false);
	let src = $state<Src | null>(null);
	let err = $state('');
	let page = $state(1);
	let zoom = $state(0.5);

	$effect(() => {
		const id = metricId.trim();
		src = null;
		err = '';
		if (!id) return;
		resolving = true;
		let cancelled = false;
		getMetricWiki(id)
			.then((res) => {
				if (cancelled) return;
				const doc = res?.page?.in_this_corpus?.source_document;
				if (doc?.record_id) {
					src = {
						inputId: doc.record_id,
						fileUrl: `/api/v1/kb/inputs/${doc.record_id}/file`,
						title: doc.title || doc.file_name || `input ${doc.record_id}`
					};
				} else {
					err = 'This metric has no linked source document.';
				}
			})
			.catch((e) => {
				if (!cancelled) err = e instanceof Error ? e.message : String(e);
			})
			.finally(() => {
				if (!cancelled) resolving = false;
			});
		return () => {
			cancelled = true;
		};
	});
</script>

<div
	class="sp"
	style="--panel:{tokens.panelBg}; --border:{tokens.border}; --text:{tokens.textPrimary}; --text-2:{tokens.textSecondary}; --accent:{tokens.accent};"
>
	<div class="bar">
		<span class="lbl">Source document</span>
		{#if src}<span class="doc">{src.title}</span>{/if}
		<span class="grow"></span>
		{#if evidenceSpans.length}
			<span class="ev">{evidenceSpans.length} evidence span{evidenceSpans.length > 1 ? 's' : ''}{evidenceLabel ? ` · ${evidenceLabel}` : ''}</span>
		{/if}
	</div>

	<div class="stage">
		{#if src}
			<SharedPdfViewer inputId={src.inputId} fileUrl={src.fileUrl} bind:page bind:zoom />
		{:else if resolving}
			<div class="empty">Resolving the metric's source document…</div>
		{:else if err}
			<div class="empty">{err}</div>
		{:else}
			<div class="empty">
				<p>No metric selected.</p>
				<p class="hint">
					Open this page with <code>?metric_id=&lt;id&gt;</code> — from the metric list or a search
					result — to load its source document here.
				</p>
			</div>
		{/if}
	</div>

	{#if evidenceSpans.length && src}
		<div class="foot">
			Span-level highlighting lands with the <code>kb.assertion_evidence</code> read endpoint (follow-up change).
		</div>
	{/if}
</div>

<style>
	.sp {
		display: flex;
		flex-direction: column;
		height: 100%;
		min-height: 0;
		background: var(--panel);
		color: var(--text);
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 10px 14px;
		border-bottom: 1px solid var(--border);
		flex: 0 0 auto;
	}
	.bar .lbl {
		font: 600 10px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--text-2);
	}
	.bar .doc {
		font: italic 13px/1 'Fraunces', Georgia, serif;
		color: var(--text-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.bar .grow {
		flex: 1;
	}
	.bar .ev {
		font: 600 10px/1 ui-sans-serif, system-ui, sans-serif;
		letter-spacing: 0.04em;
		color: var(--accent);
	}
	.stage {
		flex: 1 1 auto;
		min-height: 0;
		position: relative;
		display: flex;
	}
	.empty {
		margin: auto;
		max-width: 42ch;
		text-align: center;
		color: var(--text-2);
		font: 400 13px/1.55 ui-sans-serif, system-ui, sans-serif;
		padding: 24px;
	}
	.empty .hint {
		font-size: 12px;
		color: var(--text-2);
		opacity: 0.85;
		margin-top: 8px;
	}
	.foot {
		flex: 0 0 auto;
		padding: 8px 14px;
		border-top: 1px solid var(--border);
		font: 400 10.5px/1.4 ui-sans-serif, system-ui, sans-serif;
		color: var(--text-2);
	}
	code {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.92em;
	}
</style>
