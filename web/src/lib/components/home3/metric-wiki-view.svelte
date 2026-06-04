<script lang="ts">
	import { getMetricWiki, type MetricWikiPage } from '$lib/services/metricWikiService';

	let {
		metricId,
		darkMode = true,
		lang
	}: { metricId: string; darkMode?: boolean; lang?: string } = $props();

	let page = $state<MetricWikiPage | null>(null);
	let loading = $state(true);
	let generated = $state(false);
	let error = $state('');

	// Page-scoped "encyclopedic paper" theme, dark/light aware.
	let pageBg = $derived(darkMode ? 'oklch(16% 0.013 250)' : 'oklch(96% 0.012 84)');
	let surface = $derived(darkMode ? 'oklch(20% 0.018 250)' : 'oklch(99% 0.006 84)');
	let border = $derived(darkMode ? 'oklch(30% 0.02 250)' : 'oklch(88% 0.012 84)');
	let ink = $derived(darkMode ? 'oklch(92% 0.01 250)' : 'oklch(28% 0.02 250)');
	let inkSoft = $derived(darkMode ? 'oklch(72% 0.02 250)' : 'oklch(46% 0.02 250)');
	let accent = $derived(darkMode ? 'oklch(74% 0.16 190)' : 'oklch(48% 0.12 190)');
	let accentTint = $derived(darkMode ? 'oklch(28% 0.05 190)' : 'oklch(95% 0.03 190)');

	async function load() {
		loading = true;
		error = '';
		try {
			const resp = await getMetricWiki(metricId, lang);
			page = resp.page;
			generated = resp.generated;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	// Load on mount and whenever the metric (or language) changes. The $effect
	// fires once after mount and again on any key change.
	let lastKey = $state('');
	$effect(() => {
		const key = `${metricId}::${lang ?? ''}`;
		if (key !== lastKey) {
			lastKey = key;
			load();
		}
	});

	type Section = { heading: string; body: string; general?: boolean };
	let proseSections = $derived.by<Section[]>(() => {
		if (!page) return [];
		const out: Section[] = [];
		if (page.definition) out.push({ heading: 'Definition', body: page.definition });
		if (page.background) out.push({ heading: 'Background', body: page.background, general: true });
		if (page.how_used) out.push({ heading: 'How it is used', body: page.how_used, general: true });
		if (page.choosing_values)
			out.push({ heading: 'Choosing values', body: page.choosing_values, general: true });
		return out;
	});

	type InfoRow = { label: string; value: string };
	let infoRows = $derived.by<InfoRow[]>(() => {
		if (!page) return [];
		const ib = page.infobox ?? {};
		const rows: InfoRow[] = [];
		const add = (label: string, value?: string | number) => {
			const v = value === undefined || value === null ? '' : String(value).trim();
			if (v) rows.push({ label, value: v });
		};
		add('Value', ib.value);
		add('Unit', ib.unit);
		add('Range type', ib.range_type);
		add('Threshold / target', ib.threshold_or_target);
		add('Measurement frequency', ib.measurement_frequency);
		add('Subject', ib.subject);
		if (typeof ib.confidence === 'number') add('Confidence', ib.confidence.toFixed(2));
		return rows;
	});
</script>

<div
	class="metric-wiki"
	style="--bg:{pageBg}; --surface:{surface}; --border:{border}; --ink:{ink}; --ink-soft:{inkSoft}; --accent:{accent}; --accent-tint:{accentTint};"
>
	{#if loading}
		<div class="state">
			<div class="spinner" aria-hidden="true"></div>
			<p class="state-title">Building this page…</p>
			<p class="state-sub">Compiling what the knowledge base knows about this metric.</p>
		</div>
	{:else if error}
		<div class="state">
			<p class="state-title error">Could not load this page</p>
			<p class="state-sub">{error}</p>
			<button class="retry" onclick={load}>Try again</button>
		</div>
	{:else if page}
		<article class="article">
			<header class="head">
				<p class="eyebrow">SemOS metric</p>
				<h1>{page.title}</h1>
				{#if page.in_this_corpus?.source_document?.title}
					<p class="source">from {page.in_this_corpus.source_document.title}</p>
				{/if}
			</header>

			<div class="layout">
				<div class="main">
					{#if page.lead}
						<p class="lead">{page.lead}</p>
					{/if}

					{#each proseSections as section}
						<section class="block">
							<h2>
								{section.heading}
								{#if section.general}
									<span class="tag" title="General background, not specific to this corpus"
										>general background</span
									>
								{/if}
							</h2>
							<p>{section.body}</p>
						</section>
					{/each}

					{#if page.in_this_corpus?.source_excerpt || page.in_this_corpus?.chunk_summary}
						<section class="block">
							<h2>In this corpus <span class="tag grounded">grounded</span></h2>
							{#if page.in_this_corpus.source_excerpt}
								<blockquote>{page.in_this_corpus.source_excerpt}</blockquote>
							{/if}
							{#if page.in_this_corpus.chunk_summary}
								<p>{page.in_this_corpus.chunk_summary}</p>
							{/if}
							{#if page.in_this_corpus.source_document?.file_name}
								<p class="cite">Source: {page.in_this_corpus.source_document.file_name}</p>
							{/if}
						</section>
					{/if}

					{#if page.related_metrics && page.related_metrics.length}
						<section class="block">
							<h2>Related metrics</h2>
							<ul class="related">
								{#each page.related_metrics as rel}
									<li>{rel}</li>
								{/each}
							</ul>
						</section>
					{/if}
				</div>

				<aside class="infobox">
					<div class="infobox-title">{page.title}</div>
					{#if infoRows.length}
						<dl>
							{#each infoRows as row}
								<dt>{row.label}</dt>
								<dd>{row.value}</dd>
							{/each}
						</dl>
					{:else}
						<p class="infobox-empty">No structured values recorded.</p>
					{/if}
				</aside>
			</div>

			<footer class="foot">
				<span>metric_id {page.metric_id}</span>
				{#if page.generated?.model}<span>generated by {page.generated.model}</span>{/if}
				{#if generated}<span>freshly generated</span>{/if}
			</footer>
		</article>
	{/if}
</div>

<style>
	.metric-wiki {
		height: 100%;
		overflow-y: auto;
		background: var(--bg);
		color: var(--ink);
		font-family:
			system-ui,
			-apple-system,
			sans-serif;
	}
	.article {
		max-width: 980px;
		margin: 0 auto;
		padding: 2rem 1.75rem 4rem;
	}
	.head {
		border-bottom: 1px solid var(--border);
		padding-bottom: 1rem;
		margin-bottom: 1.5rem;
	}
	.eyebrow {
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-size: 0.72rem;
		color: var(--accent);
		margin: 0 0 0.35rem;
	}
	.head h1 {
		font-family: Georgia, 'Times New Roman', serif;
		font-size: 2rem;
		line-height: 1.15;
		margin: 0;
		color: var(--ink);
	}
	.source {
		color: var(--ink-soft);
		font-size: 0.9rem;
		margin: 0.4rem 0 0;
	}
	.layout {
		display: grid;
		grid-template-columns: 1fr 300px;
		gap: 2rem;
		align-items: start;
	}
	.lead {
		font-size: 1.1rem;
		line-height: 1.6;
		color: var(--ink);
		margin: 0 0 1.5rem;
	}
	.block {
		margin: 0 0 1.5rem;
	}
	.block h2 {
		font-family: Georgia, 'Times New Roman', serif;
		font-size: 1.25rem;
		margin: 0 0 0.5rem;
		padding-bottom: 0.3rem;
		border-bottom: 1px solid var(--border);
		color: var(--ink);
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.block p {
		line-height: 1.65;
		color: var(--ink);
		margin: 0;
	}
	.tag {
		font-family: system-ui, sans-serif;
		font-size: 0.62rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding: 0.15rem 0.45rem;
		border-radius: 999px;
		background: var(--accent-tint);
		color: var(--accent);
		font-weight: 600;
	}
	.tag.grounded {
		opacity: 0.85;
	}
	blockquote {
		margin: 0 0 0.75rem;
		padding: 0.6rem 0.9rem;
		border-left: 3px solid var(--accent);
		background: var(--surface);
		color: var(--ink-soft);
		font-style: italic;
		line-height: 1.55;
	}
	.cite {
		font-size: 0.8rem;
		color: var(--ink-soft);
		margin-top: 0.5rem !important;
	}
	.related {
		margin: 0;
		padding-left: 1.1rem;
		line-height: 1.7;
		color: var(--ink);
	}
	.infobox {
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 1rem 1.1rem;
		position: sticky;
		top: 1rem;
	}
	.infobox-title {
		font-family: Georgia, serif;
		font-weight: 700;
		font-size: 1rem;
		text-align: center;
		padding-bottom: 0.6rem;
		margin-bottom: 0.6rem;
		border-bottom: 2px solid var(--accent);
		color: var(--ink);
	}
	.infobox dl {
		margin: 0;
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.55rem;
	}
	.infobox dt {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--ink-soft);
	}
	.infobox dd {
		margin: 0.1rem 0 0;
		font-size: 0.95rem;
		color: var(--ink);
		word-break: break-word;
	}
	.infobox-empty {
		font-size: 0.85rem;
		color: var(--ink-soft);
		margin: 0;
	}
	.foot {
		margin-top: 2rem;
		padding-top: 1rem;
		border-top: 1px solid var(--border);
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		font-size: 0.75rem;
		color: var(--ink-soft);
	}
	.state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		min-height: 60vh;
		text-align: center;
		padding: 2rem;
	}
	.state-title {
		font-size: 1.15rem;
		font-weight: 600;
		color: var(--ink);
		margin: 0;
	}
	.state-title.error {
		color: oklch(62% 0.18 25);
	}
	.state-sub {
		color: var(--ink-soft);
		margin: 0;
		max-width: 30rem;
	}
	.retry {
		margin-top: 0.5rem;
		padding: 0.45rem 1.1rem;
		border-radius: 8px;
		border: 1px solid var(--accent);
		background: transparent;
		color: var(--accent);
		cursor: pointer;
		font-weight: 600;
	}
	.retry:hover {
		background: var(--accent-tint);
	}
	.spinner {
		width: 34px;
		height: 34px;
		border-radius: 50%;
		border: 3px solid var(--border);
		border-top-color: var(--accent);
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (max-width: 760px) {
		.layout {
			grid-template-columns: 1fr;
		}
		.infobox {
			position: static;
			order: -1;
		}
	}
</style>
