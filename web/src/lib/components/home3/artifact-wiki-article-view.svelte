<script lang="ts">
	import { metricWikiCopyForLang } from './metric-wiki-i18n';

	type ArticleMap = Record<string, unknown>;

	let {
		article = null,
		artifactType,
		artifactId,
		darkMode = true,
		lang,
		showInfobox = false,
		loading = false,
		error = '',
		generated = false,
		retry = undefined
	}: {
		article?: ArticleMap | null;
		artifactType: string;
		artifactId: string;
		darkMode?: boolean;
		lang?: string;
		showInfobox?: boolean;
		loading?: boolean;
		error?: string;
		generated?: boolean;
		retry?: (() => void) | undefined;
	} = $props();

	let pageBg = $derived(darkMode ? 'oklch(16% 0.013 250)' : 'oklch(96% 0.012 84)');
	let surface = $derived(darkMode ? 'oklch(20% 0.018 250)' : 'oklch(99% 0.006 84)');
	let border = $derived(darkMode ? 'oklch(30% 0.02 250)' : 'oklch(88% 0.012 84)');
	let ink = $derived(darkMode ? 'oklch(92% 0.01 250)' : 'oklch(28% 0.02 250)');
	let inkSoft = $derived(darkMode ? 'oklch(72% 0.02 250)' : 'oklch(46% 0.02 250)');
	let accent = $derived(darkMode ? 'oklch(74% 0.16 190)' : 'oklch(48% 0.12 190)');
	let accentTint = $derived(darkMode ? 'oklch(28% 0.05 190)' : 'oklch(95% 0.03 190)');
	let copy = $derived(metricWikiCopyForLang(lang));

	function str(val: unknown): string {
		return typeof val === 'string' ? val.trim() : '';
	}

	function num(val: unknown): number | null {
		return typeof val === 'number' && Number.isFinite(val) ? val : null;
	}

	function arr(val: unknown): string[] {
		if (!Array.isArray(val)) return [];
		return val
			.map((item) => (typeof item === 'string' ? item.trim() : ''))
			.filter((item) => item.length > 0);
	}

	function obj(val: unknown): ArticleMap | null {
		return val && typeof val === 'object' && !Array.isArray(val) ? (val as ArticleMap) : null;
	}

	let isMetricArticle = $derived(Boolean(article && (str(article.metric_id) || obj(article.in_this_corpus))));
	let title = $derived(str(article?.title) || artifactId);
	let lead = $derived(str(article?.lead));
	let definition = $derived(str(article?.definition));
	let background = $derived(str(article?.background));
	let howUsed = $derived(str(article?.how_used));
	let choosingValues = $derived(str(article?.choosing_values));
	let relatedItems = $derived(arr(article?.related_metrics));
	let metricId = $derived(str(article?.metric_id));
	let sourceDocument = $derived(obj(obj(article?.in_this_corpus)?.source_document));
	let sourceTitle = $derived(str(sourceDocument?.title));
	let sourceFile = $derived(str(sourceDocument?.file_name));
	let sourceExcerpt = $derived(str(obj(article?.in_this_corpus)?.source_excerpt));
	let chunkSummary = $derived(str(obj(article?.in_this_corpus)?.chunk_summary));
	let infobox = $derived(obj(article?.infobox));
	let artifactLabel = $derived(
		artifactType === 'metric' ? copy.eyebrow : `${artifactType.replaceAll('_', ' ')} wiki`
	);
	let buildingSub = $derived(
		artifactType === 'metric'
			? copy.buildingSub
			: lang === 'zh-cn'
				? '正在整理知识库中关于该制品的信息。'
				: 'Compiling what the knowledge base knows about this artifact.'
	);

	type Section = { heading: string; body: string; general?: boolean };
	let proseSections = $derived.by<Section[]>(() => {
		const out: Section[] = [];
		if (definition) out.push({ heading: copy.definition, body: definition });
		if (background) out.push({ heading: copy.background, body: background, general: true });
		if (howUsed) out.push({ heading: copy.howUsed, body: howUsed, general: true });
		if (choosingValues) out.push({ heading: copy.choosingValues, body: choosingValues, general: true });
		return out;
	});

	type InfoRow = { label: string; value: string };
	let infoRows = $derived.by<InfoRow[]>(() => {
		const rows: InfoRow[] = [];
		if (!infobox) return rows;
		const add = (label: string, value: unknown) => {
			const s = typeof value === 'number' ? value.toFixed(2) : str(value);
			if (s) rows.push({ label, value: s });
		};
		add(copy.value, infobox.value);
		add(copy.unit, infobox.unit);
		add(copy.rangeType, infobox.range_type);
		add(copy.thresholdOrTarget, infobox.threshold_or_target);
		add(copy.measurementFrequency, infobox.measurement_frequency);
		add(copy.subject, infobox.subject);
		const confidence = num(infobox.confidence);
		if (confidence !== null) add(copy.confidence, confidence);
		return rows;
	});
</script>

<div
	class="wiki-article"
	style="--bg:{pageBg}; --surface:{surface}; --border:{border}; --ink:{ink}; --ink-soft:{inkSoft}; --accent:{accent}; --accent-tint:{accentTint};"
>
	{#if loading && !article}
		<div class="state">
			<div class="spinner" aria-hidden="true"></div>
			<p class="state-title">{copy.buildingTitle}</p>
			<p class="state-sub">{buildingSub}</p>
			<div class="skeleton" aria-hidden="true">
				<span class="skeleton-line wide"></span>
				<span class="skeleton-line"></span>
				<span class="skeleton-line narrow"></span>
			</div>
		</div>
	{:else if error}
		<div class="state">
			<p class="state-title error">{copy.loadErrorTitle}</p>
			<p class="state-sub">{error}</p>
			{#if retry}
				<button class="retry" onclick={retry}>{copy.retry}</button>
			{/if}
		</div>
	{:else if article}
		<article class="article">
			<header class="head">
				<p class="eyebrow">{artifactLabel}</p>
				<h1>{title}</h1>
				{#if sourceTitle}
					<p class="source">{copy.sourcePrefix} {sourceTitle}</p>
				{/if}
			</header>

			<div class="layout" class:no-infobox={!showInfobox}>
				<div class="main">
					{#if lead}
						<p class="lead">{lead}</p>
					{/if}

					{#each proseSections as section}
						<section class="block">
							<h2>
								{section.heading}
								{#if section.general}
									<span class="tag" title={copy.generalBackgroundTitle}
										>{copy.generalBackground}</span
									>
								{/if}
							</h2>
							<p>{section.body}</p>
						</section>
					{/each}

					{#if sourceExcerpt || chunkSummary}
						<section class="block">
							<h2>{copy.inThisCorpus} <span class="tag grounded">{copy.grounded}</span></h2>
							{#if sourceExcerpt}
								<blockquote>{sourceExcerpt}</blockquote>
							{/if}
							{#if chunkSummary}
								<p>{chunkSummary}</p>
							{/if}
							{#if sourceFile}
								<p class="cite">{copy.sourceLabel} {sourceFile}</p>
							{/if}
						</section>
					{/if}

					{#if relatedItems.length}
						<section class="block">
							<h2>{copy.relatedMetrics}</h2>
							<ul class="related">
								{#each relatedItems as rel}
									<li>{rel}</li>
								{/each}
							</ul>
						</section>
					{/if}
				</div>

				{#if showInfobox}
					<aside class="infobox">
						<div class="infobox-title">{title}</div>
						{#if infoRows.length}
							<dl>
								{#each infoRows as row}
									<dt>{row.label}</dt>
									<dd>{row.value}</dd>
								{/each}
							</dl>
						{:else}
							<p class="infobox-empty">{copy.noStructuredValues}</p>
						{/if}
					</aside>
				{/if}
			</div>

			<footer class="foot">
				{#if isMetricArticle && metricId}<span>{copy.metricIdLabel} {metricId}</span>{/if}
				{#if str(obj(article.generated)?.model)}<span>{copy.generatedByPrefix} {str(obj(article.generated)?.model)}</span>{/if}
				{#if generated}<span>{copy.freshlyGenerated}</span>{/if}
			</footer>
		</article>
	{/if}
</div>

<style>
	.wiki-article {
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
	.layout.no-infobox {
		grid-template-columns: minmax(0, 1fr);
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
	.skeleton {
		width: min(34rem, 100%);
		display: grid;
		gap: 0.6rem;
		margin-top: 0.9rem;
	}
	.skeleton-line {
		height: 0.7rem;
		border-radius: 999px;
		background: linear-gradient(
			90deg,
			color-mix(in oklch, var(--surface) 72%, transparent) 0%,
			color-mix(in oklch, var(--accent-tint) 78%, white 8%) 50%,
			color-mix(in oklch, var(--surface) 72%, transparent) 100%
		);
		background-size: 220% 100%;
		animation: shimmer 1.35s ease-in-out infinite;
	}
	.skeleton-line.wide {
		width: 100%;
	}
	.skeleton-line {
		width: 82%;
	}
	.skeleton-line.narrow {
		width: 64%;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@keyframes shimmer {
		0% {
			background-position: 100% 0;
		}
		100% {
			background-position: -100% 0;
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
