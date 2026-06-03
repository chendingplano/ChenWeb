<script lang="ts">
	import { onMount } from 'svelte';
	import { getWikiOverview, type WikiOverviewResponse } from '$lib/services/kbService';
	import SemosKbHero from '$lib/components/home3/semos-kb-hero.svelte';

	// ---------------------------------------------------------------------------
	// Data
	// ---------------------------------------------------------------------------
	let overview = $state<WikiOverviewResponse | null>(null);
	let loadError = $state<string | null>(null);
	let loading = $state(true);

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

	// ---------------------------------------------------------------------------
	// Navigation targets for the Panel B directory
	// ---------------------------------------------------------------------------
	const KNOWLEDGE = '/home3/knowledge?section=';
	const GRAPH_HREF = '/knowledge-graph';

	// The eight portals (Panel B) — each a doorway into a browse/search surface.
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

	// ---------------------------------------------------------------------------
	// Formatting helpers
	// ---------------------------------------------------------------------------
	const nf = new Intl.NumberFormat('en-US');
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

	const totalDocs = $derived(overview?.counts.documents ?? null);
</script>

<svelte:head>
	<title>SemOS · Deep Wiki</title>
	<meta
		name="description"
		content="The entrance to SemOS, a deep knowledge base of documents and their artifacts."
	/>
</svelte:head>

<div class="wiki">
	<!-- ===================== PANEL A: reusable SemOS KB hero ===================== -->
	<!-- Full width so the hero clears its compact breakpoint and renders the wired
	     diagram identically to the LLM Wiki v3 Top Panel. -->
	<SemosKbHero darkMode={false} {overview} {loading} />

	<!-- ===================== PANEL B: portal directory ===================== -->
	<section class="panel-b" aria-label="Browse by artifact type">
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
				Artifacts are linked through relations, forming one navigable wiki across
				{fmt(totalDocs)} documents.
			{:else}
				Artifacts are linked through relations, forming one navigable wiki.
			{/if}
		</p>
	</section>

	<!-- ===================== PANEL C ===================== -->
	<section class="panel-c" aria-label="Recent activity">
		{#if loadError}
			<div class="banner error">
				<span>{loadError}</span>
				<button type="button" onclick={load}>Retry</button>
			</div>
		{/if}

		<div class="activity">
			<!-- Recent Adds -->
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

			<!-- Recent Edits -->
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

			<!-- Recent Processed -->
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

			<!-- Errors -->
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
	/* ---- Palette: warm paper + ink, one deep-teal accent ------------------ */
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

		min-height: 100vh;
		background:
			radial-gradient(120% 80% at 50% -10%, oklch(0.99 0.012 95) 0%, transparent 60%), var(--paper);
		color: var(--ink);
		font-family: var(--sans);
		padding: clamp(1.25rem, 3vw, 2.75rem);
		box-sizing: border-box;
		overflow-x: clip;
	}

	/* ---- Panel B: full-width directory below the hero (Top/Middle/Bottom) -- */
	.panel-b {
		max-width: 1320px;
		margin: clamp(2rem, 4vw, 3.5rem) auto 0;
		border-top: 1px solid var(--line);
		padding-top: clamp(1.5rem, 3vw, 2.5rem);
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
		grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
		gap: 0 clamp(1rem, 3vw, 2.5rem);
	}
	.portal {
		display: flex;
		flex-direction: column;
		gap: 0.05rem;
		padding: 0.7rem 0.5rem;
		border-bottom: 1px solid var(--line-soft);
		text-decoration: none;
		border-radius: 8px;
		transition:
			background-color 160ms ease,
			padding-left 200ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.portal:hover {
		background: var(--accent-tint);
		padding-left: 0.85rem;
	}
	.portal:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: -2px;
	}
	.portal-label {
		font-size: 0.98rem;
		font-weight: 600;
		color: var(--accent-strong);
	}
	.portal-blurb {
		font-size: 0.82rem;
		color: var(--ink-faint);
	}
	.panel-foot {
		margin: 1.1rem 0 0;
		font-size: 0.82rem;
		line-height: 1.5;
		color: var(--ink-faint);
		max-width: 34ch;
	}

	/* ---- Panel C ---------------------------------------------------------- */
	.panel-c {
		max-width: 1320px;
		margin: clamp(2rem, 4vw, 3.5rem) auto 0;
		border-top: 1px solid var(--line);
		padding-top: clamp(1.5rem, 3vw, 2.5rem);
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
		border: 1px solid oklch(0.8 0.08 30);
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
		gap: clamp(1rem, 2.5vw, 2.25rem);
	}
	.feed-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
		padding-bottom: 0.6rem;
		margin-bottom: 0.4rem;
		border-bottom: 2px solid var(--ink);
	}
	.feed-errors .feed-head {
		border-bottom-color: var(--danger);
	}
	.feed-head h3 {
		font-family: var(--serif);
		font-weight: 400;
		font-size: 1.12rem;
		margin: 0;
	}
	.feed-errors .feed-head h3 {
		color: var(--danger);
	}
	.feed-sub {
		font-size: 0.7rem;
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
		font-size: 0.88rem;
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
		font-size: 0.74rem;
		color: var(--ink-faint);
		font-variant-numeric: tabular-nums;
	}
	.tag {
		flex-shrink: 0;
		font-size: 0.68rem;
		padding: 0.1rem 0.4rem;
		border-radius: 5px;
		background: var(--accent-tint);
		color: var(--accent-strong);
		letter-spacing: 0.02em;
	}
	/* Errors are denser: title row + wrapped message */
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
		font-size: 0.78rem;
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

	/* Skeleton -------------------------------------------------------------*/
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
		animation: shimmer 1.4s ease-in-out infinite;
	}
	@keyframes shimmer {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}

	.wiki-foot {
		max-width: 1320px;
		margin: clamp(2rem, 4vw, 3rem) auto 0;
		padding-top: 1.25rem;
		border-top: 1px solid var(--line-soft);
		font-size: 0.78rem;
		color: var(--ink-faint);
		text-align: center;
	}

	/* ---- Responsive ------------------------------------------------------- */
	@media (max-width: 980px) {
		.panel-foot {
			max-width: none;
		}
	}
	@media (max-width: 760px) {
		.activity {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 480px) {
		.activity {
			grid-template-columns: 1fr;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sk-bar {
			animation: none;
		}
		.portal,
		.feed li a {
			transition: none;
		}
	}
</style>
