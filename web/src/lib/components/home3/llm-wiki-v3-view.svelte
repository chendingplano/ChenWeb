<script lang="ts">
	import { onMount } from 'svelte';
	import { getWikiOverview, type WikiOverviewResponse } from '$lib/services/kbService';
	import SemosKbHero from './semos-kb-hero.svelte';

	// v3 keeps the original "LLM Wiki" (/deep-wiki) foundation: its light paper/ink
	// design, the "Explore the wiki" directory (Panel B), and the recent-activity
	// feeds (Panel C). Panel A is the reusable SemOS KB hero (the artistically wired
	// knowledge-base diagram + search). A dark variant keeps it readable inside the
	// dark knowledge shell.
	let { darkMode = true }: { darkMode?: boolean } = $props();

	let overview = $state<WikiOverviewResponse | null>(null);
	let loadError = $state<string | null>(null);
	let loading = $state(true);

	const KNOWLEDGE = '/home3/knowledge?section=';
	const GRAPH_HREF = '/knowledge-graph';

	// Panel B directory — kept identical in content to the original LLM Wiki.
	const PORTALS: { label: string; blurb: string; href: string }[] = [
		{
			label: 'Documents',
			blurb: 'documents, notes, articles',
			href: KNOWLEDGE + 'kb-input-details'
		},
		{ label: 'Content Segments', blurb: 'content segments, chunks', href: KNOWLEDGE + 'kb-chunks' },
		{ label: 'Topics', blurb: 'browse and search topics', href: KNOWLEDGE + 'kb-topic-tree' },
		{ label: 'Metrics', blurb: 'browse and search metrics', href: KNOWLEDGE + 'kb-metrics' },
		{
			label: 'Parts & Components',
			blurb: 'browse and search parts and components',
			href: KNOWLEDGE + 'kb-products'
		},
		{ label: 'Scenes', blurb: 'browse and search scenes', href: KNOWLEDGE + 'kb-scene-blocks' },
		{
			label: 'Provisions',
			blurb: 'browse and search provisions',
			href: KNOWLEDGE + 'kb-provision-tree'
		},
		{ label: 'Graphs', blurb: 'browse and search entities and relations', href: GRAPH_HREF }
	];

	const nf = new Intl.NumberFormat('en-US');
	const totalDocs = $derived(overview?.counts.documents ?? null);

	async function load() {
		loading = true;
		loadError = null;
		try {
			overview = await getWikiOverview();
		} catch (err) {
			loadError = err instanceof Error ? err.message : 'Unable to reach SemOS.';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function fmt(value: number | null): string {
		return value == null ? '—' : nf.format(value);
	}

	function relTime(iso: string | null): string {
		if (!iso) return '';
		const then = new Date(iso).getTime();
		if (Number.isNaN(then)) return '';
		const secs = Math.round((Date.now() - then) / 1000);
		if (secs < 45) return 'just now';
		const mins = Math.round(secs / 60);
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.round(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		const days = Math.round(hrs / 24);
		if (days < 30) return `${days}d ago`;
		const months = Math.round(days / 30);
		if (months < 12) return `${months}mo ago`;
		return `${Math.round(months / 12)}y ago`;
	}

	function absTime(iso: string | null): string {
		if (!iso) return '';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? '' : d.toLocaleString();
	}
</script>

<div class="wiki" class:dark={darkMode} class:light={!darkMode}>
	<!-- ===================== PANEL A: reusable SemOS KB hero ===================== -->
	<SemosKbHero {darkMode} {overview} {loading} />

	<!-- ===================== PANEL B: directory ===================== -->
	<section class="browse" aria-label="Browse by artifact type">
		<h2 class="panel-title">Explore the wiki</h2>
		<nav class="portals">
			{#each PORTALS as p (p.label)}
				<a class="portal" href={p.href}>
					<span class="portal-label">{p.label}</span>
					<span class="portal-blurb">{p.blurb}</span>
				</a>
			{/each}
		</nav>
		<p class="panel-foot">
			{#if totalDocs != null}
				Artifacts are linked through relations, forming one navigable wiki across {fmt(totalDocs)} documents.
			{:else}
				Artifacts are linked through relations, forming one navigable wiki.
			{/if}
		</p>
	</section>

	<!-- ===================== PANEL C: recent activity ===================== -->
	<section class="panel-c" aria-label="Recent activity">
		{#if loadError}
			<div class="banner error">
				<span>{loadError}</span>
				<button type="button" onclick={load}>Retry</button>
			</div>
		{/if}

		<div class="activity">
			<div class="feed">
				<header class="feed-head">
					<h3>Recent Adds</h3>
					<span class="feed-sub">newest documents</span>
				</header>
				{#if loading}
					{@render skeleton()}
				{:else if (overview?.recent_adds.length ?? 0) === 0}
					{@render emptyFeed('No documents have been added yet.')}
				{:else}
					<ul>
						{#each overview!.recent_adds as d (d.id)}
							<li>
								<a href={`/home3/knowledge?section=kb-input-details&record_id=${d.id}`}>{d.title}</a
								>
								<time title={absTime(d.time)}>{relTime(d.time)}</time>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="feed">
				<header class="feed-head">
					<h3>Recent Edits</h3>
					<span class="feed-sub">most recently modified</span>
				</header>
				{#if loading}
					{@render skeleton()}
				{:else if (overview?.recent_edits.length ?? 0) === 0}
					{@render emptyFeed('No edits recorded yet.')}
				{:else}
					<ul>
						{#each overview!.recent_edits as d (d.id)}
							<li>
								<a href={`/home3/knowledge?section=kb-input-details&record_id=${d.id}`}>{d.title}</a
								>
								<time title={absTime(d.time)}>{relTime(d.time)}</time>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="feed">
				<header class="feed-head">
					<h3>Recent Processed</h3>
					<span class="feed-sub">latest pipeline runs</span>
				</header>
				{#if loading}
					{@render skeleton()}
				{:else if (overview?.recent_processed.length ?? 0) === 0}
					{@render emptyFeed('Nothing has finished processing yet.')}
				{:else}
					<ul>
						{#each overview!.recent_processed as p, i (i)}
							<li>
								{#if p.record_id != null}
									<a href={`/home3/knowledge?section=kb-input-details&record_id=${p.record_id}`}
										>{p.title}</a
									>
								{:else}
									<span class="plain">{p.title}</span>
								{/if}
								<span class="tag">{p.processor}</span>
								<time title={absTime(p.time)}>{relTime(p.time)}</time>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="feed feed-errors">
				<header class="feed-head">
					<h3>Errors</h3>
					<span class="feed-sub">needs attention</span>
				</header>
				{#if loading}
					{@render skeleton()}
				{:else if (overview?.errors.length ?? 0) === 0}
					{@render emptyFeed('No processing errors. All clear.')}
				{:else}
					<ul>
						{#each overview!.errors as e, i (i)}
							<li class="err-row">
								<div class="err-top">
									{#if e.record_id != null}
										<a href={`/home3/knowledge?section=kb-input-details&record_id=${e.record_id}`}
											>{e.title}</a
										>
									{:else}
										<span class="plain">{e.title}</span>
									{/if}
									<time title={absTime(e.time)}>{relTime(e.time)}</time>
								</div>
								<p class="err-msg" title={e.message}>
									<span class="tag tag-err">{e.processor}</span>{e.message}
								</p>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>
	</section>

	<footer class="wiki-foot">
		SemOS: a deep knowledge base. Documents and their artifacts, connected as one wiki.
	</footer>
</div>

{#snippet skeleton()}
	<ul class="sk">
		{#each Array(5) as _, i (i)}
			<li><span class="sk-bar" style={`width:${70 - i * 6}%`}></span></li>
		{/each}
	</ul>
{/snippet}

{#snippet emptyFeed(message: string)}
	<p class="empty">{message}</p>
{/snippet}

<style>
	/* ---- Original LLM Wiki palette: warm paper + ink, one deep-teal accent --- */
	.wiki {
		--paper: oklch(0.985 0.004 95);
		--surface: oklch(0.997 0.003 95);
		--ink: oklch(0.26 0.018 275);
		--ink-soft: oklch(0.46 0.02 272);
		--ink-faint: oklch(0.62 0.018 270);
		--line: oklch(0.9 0.008 95);
		--line-soft: oklch(0.94 0.006 95);
		--accent: oklch(0.52 0.1 213);
		--accent-strong: oklch(0.43 0.11 214);
		--accent-tint: oklch(0.95 0.025 210);
		--danger: oklch(0.52 0.17 28);
		--danger-tint: oklch(0.96 0.03 30);

		--serif: Georgia, 'Iowan Old Style', 'Times New Roman', serif;
		--sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;

		height: 100%;
		overflow: auto;
		background:
			radial-gradient(120% 70% at 50% -8%, oklch(0.99 0.012 95) 0%, transparent 58%), var(--paper);
		color: var(--ink);
		font-family: var(--sans);
		padding: clamp(1.25rem, 3vw, 2.5rem);
		box-sizing: border-box;
	}

	.wiki.dark {
		--paper: oklch(0.17 0.012 265);
		--surface: oklch(0.21 0.014 265);
		--ink: oklch(0.95 0.008 250);
		--ink-soft: oklch(0.78 0.015 255);
		--ink-faint: oklch(0.62 0.02 258);
		--line: oklch(1 0 0 / 0.12);
		--line-soft: oklch(1 0 0 / 0.07);
		--accent: oklch(0.72 0.11 208);
		--accent-strong: oklch(0.8 0.11 205);
		--accent-tint: oklch(0.72 0.11 208 / 0.16);
		--danger: oklch(0.72 0.16 28);
		--danger-tint: oklch(0.72 0.16 28 / 0.16);

		background:
			radial-gradient(130% 80% at 50% -8%, oklch(0.24 0.03 230) 0%, transparent 60%), var(--paper);
	}

	/* ---- Panel B: directory ---- */
	.browse {
		max-width: 1100px;
		margin: clamp(1.75rem, 3.5vw, 3rem) auto 0;
		border-top: 1px solid var(--line);
		padding-top: clamp(1.25rem, 2.5vw, 2rem);
	}
	.panel-title {
		font-family: var(--serif);
		font-weight: 400;
		font-size: 1.35rem;
		margin: 0 0 0.85rem;
		color: var(--ink);
	}
	.portals {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
		gap: 8px;
	}
	.portal {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
		padding: 0.7rem 0.85rem;
		border: 1px solid var(--line);
		border-radius: 10px;
		background: var(--surface);
		text-decoration: none;
		transition:
			background-color 160ms ease,
			border-color 160ms ease,
			transform 200ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.portal:hover {
		background: var(--accent-tint);
		border-color: color-mix(in srgb, var(--accent) 45%, var(--line));
		transform: translateY(-2px);
	}
	.portal:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.portal-label {
		font-size: 0.96rem;
		font-weight: 600;
		color: var(--accent-strong);
	}
	.portal-blurb {
		font-size: 0.8rem;
		color: var(--ink-faint);
	}
	.panel-foot {
		margin: 1rem 0 0;
		font-size: 0.82rem;
		line-height: 1.5;
		color: var(--ink-faint);
	}

	/* ---- Panel C: activity ---- */
	.panel-c {
		max-width: 1100px;
		margin: clamp(1.75rem, 3.5vw, 3rem) auto 0;
		border-top: 1px solid var(--line);
		padding-top: clamp(1.25rem, 2.5vw, 2rem);
	}
	.banner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.7rem 1rem;
		border-radius: 10px;
		margin-bottom: 1.25rem;
		font-size: 0.88rem;
	}
	.banner.error {
		background: var(--danger-tint);
		color: var(--danger);
		border: 1px solid color-mix(in srgb, var(--danger) 40%, var(--line));
	}
	.banner button {
		border: 1px solid currentColor;
		background: transparent;
		color: inherit;
		border-radius: 7px;
		padding: 0.3rem 0.8rem;
		font-size: 0.82rem;
		cursor: pointer;
	}
	.activity {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: clamp(1rem, 2.5vw, 2rem);
	}
	.feed-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
		padding-bottom: 0.55rem;
		margin-bottom: 0.4rem;
		border-bottom: 2px solid var(--ink);
	}
	.feed-errors .feed-head {
		border-bottom-color: var(--danger);
	}
	.feed-head h3 {
		font-family: var(--serif);
		font-weight: 400;
		font-size: 1.1rem;
		margin: 0;
	}
	.feed-errors .feed-head h3 {
		color: var(--danger);
	}
	.feed-sub {
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--ink-faint);
	}
	.feed ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.feed li {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--line-soft);
		font-size: 0.86rem;
	}
	.feed li a {
		color: var(--ink);
		text-decoration: none;
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		transition: color 140ms ease;
	}
	.feed li a:hover {
		color: var(--accent-strong);
		text-decoration: underline;
		text-underline-offset: 2px;
	}
	.feed li .plain {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--ink-soft);
	}
	.feed time {
		flex-shrink: 0;
		font-size: 0.72rem;
		color: var(--ink-faint);
		font-variant-numeric: tabular-nums;
	}
	.tag {
		flex-shrink: 0;
		font-size: 0.66rem;
		padding: 0.1rem 0.4rem;
		border-radius: 5px;
		background: var(--accent-tint);
		color: var(--accent-strong);
		letter-spacing: 0.02em;
	}
	.feed-errors li {
		flex-direction: column;
		align-items: stretch;
		gap: 0.25rem;
	}
	.err-top {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}
	.err-top a,
	.err-top .plain {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.err-msg {
		margin: 0;
		font-size: 0.76rem;
		line-height: 1.45;
		color: var(--ink-soft);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.tag-err {
		background: var(--danger-tint);
		color: var(--danger);
		margin-right: 0.4rem;
	}
	.empty {
		font-size: 0.84rem;
		color: var(--ink-faint);
		padding: 0.75rem 0;
		margin: 0;
		line-height: 1.5;
	}

	.sk {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.sk li {
		padding: 0.55rem 0;
		border-bottom: 1px solid var(--line-soft);
	}
	.sk-bar {
		display: block;
		height: 0.7rem;
		border-radius: 4px;
		background: linear-gradient(90deg, var(--line-soft), var(--line), var(--line-soft));
		background-size: 200% 100%;
		animation: llm-wiki-v3-shimmer 1.4s ease-in-out infinite;
	}
	@keyframes llm-wiki-v3-shimmer {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}

	.wiki-foot {
		max-width: 1100px;
		margin: clamp(1.75rem, 3.5vw, 2.75rem) auto 0;
		padding-top: 1.25rem;
		border-top: 1px solid var(--line-soft);
		font-size: 0.78rem;
		color: var(--ink-faint);
		text-align: center;
	}

	@media (max-width: 760px) {
		.activity {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 540px) {
		.activity {
			grid-template-columns: 1fr;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.portal,
		.feed li a {
			transition: none;
		}
		.sk-bar {
			animation: none;
		}
	}
</style>
