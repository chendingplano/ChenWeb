<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { getWikiOverview, type WikiOverviewResponse } from '$lib/services/kbService';
	import { getLocale, setLocale, locales } from '$lib/paraglide/runtime.js';
	import FilesIcon from '@lucide/svelte/icons/files';
	import FileSearchIcon from '@lucide/svelte/icons/file-search';
	import PresentationIcon from '@lucide/svelte/icons/presentation';
	import GlobeIcon from '@lucide/svelte/icons/globe';
	import NetworkIcon from '@lucide/svelte/icons/network';
	import ChartBarIcon from '@lucide/svelte/icons/chart-bar';
	import BoxesIcon from '@lucide/svelte/icons/boxes';
	import ScaleIcon from '@lucide/svelte/icons/scale';
	import ClapperboardIcon from '@lucide/svelte/icons/clapperboard';

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
	// Navigation targets (shared by hero stat nodes and the Panel B directory)
	// ---------------------------------------------------------------------------
	const KNOWLEDGE = '/home3/knowledge?section=';
	const GRAPH_HREF = '/knowledge-graph';

	type ArtifactKey =
		| 'documents'
		| 'content_segments'
		| 'topics'
		| 'semantic_projections'
		| 'metrics'
		| 'provisions'
		| 'parts_components'
		| 'scenes'
		| 'entities'
		| 'relations';

	type StatNode = {
		n: number;
		label: string;
		keys: ArtifactKey[];
		icon: typeof FilesIcon;
		href: string;
	};

	// The nine corpus dimensions that orbit the SemOS KB core (Panel A), numbered
	// and ordered to match the system diagram. "Entities and Relations" is one node
	// (the graph), so its value sums both counts.
	const STAT_NODES: StatNode[] = [
		{
			n: 1,
			label: 'Documents',
			keys: ['documents'],
			icon: FilesIcon,
			href: KNOWLEDGE + 'kb-input-details'
		},
		{
			n: 2,
			label: 'Content Segments',
			keys: ['content_segments'],
			icon: FileSearchIcon,
			href: KNOWLEDGE + 'kb-chunks'
		},
		{
			n: 3,
			label: 'Topics',
			keys: ['topics'],
			icon: PresentationIcon,
			href: KNOWLEDGE + 'kb-topic-tree'
		},
		{
			n: 4,
			label: 'Semantic Projections',
			keys: ['semantic_projections'],
			icon: GlobeIcon,
			href: KNOWLEDGE + 'kb-semantic-projections'
		},
		{
			n: 5,
			label: 'Entities & Relations',
			keys: ['entities', 'relations'],
			icon: NetworkIcon,
			href: GRAPH_HREF
		},
		{
			n: 6,
			label: 'Metrics',
			keys: ['metrics'],
			icon: ChartBarIcon,
			href: KNOWLEDGE + 'kb-metrics'
		},
		{
			n: 7,
			label: 'Parts & Components',
			keys: ['parts_components'],
			icon: BoxesIcon,
			href: KNOWLEDGE + 'kb-products'
		},
		{
			n: 8,
			label: 'Provisions',
			keys: ['provisions'],
			icon: ScaleIcon,
			href: KNOWLEDGE + 'kb-provision-tree'
		},
		{
			n: 9,
			label: 'Scenes',
			keys: ['scenes'],
			icon: ClapperboardIcon,
			href: KNOWLEDGE + 'kb-scene-blocks'
		}
	];

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
	// Radial geometry — places the nine nodes evenly around the emblem
	// ---------------------------------------------------------------------------
	const RING_RADIUS = 248;
	const positioned = $derived(
		STAT_NODES.map((node, i) => {
			const angle = ((-90 + (360 / STAT_NODES.length) * i) * Math.PI) / 180;
			const counts = overview?.counts;
			const value = counts ? node.keys.reduce((sum, k) => sum + (counts[k] ?? 0), 0) : null;
			return {
				...node,
				x: Math.round(Math.cos(angle) * RING_RADIUS),
				y: Math.round(Math.sin(angle) * RING_RADIUS),
				value
			};
		})
	);

	// Collapse the constellation into a stacked grid only once the ring (a ~620px
	// box of nodes orbiting the emblem) can no longer fit the Panel A column.
	let heroWidth = $state(1200);
	const compact = $derived(heroWidth < 680);

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

	// ---------------------------------------------------------------------------
	// Search + language
	// ---------------------------------------------------------------------------
	let query = $state('');
	function runSearch(event: SubmitEvent) {
		event.preventDefault();
		const q = query.trim();
		goto(`/home3/knowledge?section=kb-search${q ? `&q=${encodeURIComponent(q)}` : ''}`);
	}

	const LOCALE_LABELS: Record<string, string> = { en: 'English', 'zh-cn': '中文' };
	let currentLocale = $state<string>('en');
	onMount(() => {
		try {
			currentLocale = getLocale();
		} catch {
			currentLocale = 'en';
		}
	});
	function changeLocale(event: Event) {
		const next = (event.target as HTMLSelectElement).value;
		currentLocale = next;
		try {
			setLocale(next as (typeof locales)[number]);
		} catch {
			/* setLocale handles its own reload strategy; ignore failures */
		}
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
	<!-- ===================== PANEL A + PANEL B ===================== -->
	<div class="masthead">
		<!-- Panel A: radial hero -------------------------------------------- -->
		<section class="panel-a" aria-label="Corpus overview" bind:clientWidth={heroWidth}>
			<p class="eyebrow">The deep knowledge base</p>
			<h1 class="wordmark">SemOS</h1>

			<div class="hero-ring" class:compact>
				{#if !compact}
					<svg class="connectors" viewBox="-320 -320 640 640" aria-hidden="true">
						{#each positioned as n (n.n)}
							<line x1="0" y1="0" x2={n.x} y2={n.y} />
						{/each}
					</svg>
				{/if}

				<!-- The SemOS KB core: a workstation onto the knowledge base -->
				<div class="core" aria-hidden="true">
					<svg viewBox="0 0 240 214" class="core-svg">
						<!-- monitor bezel + screen -->
						<rect x="22" y="6" width="196" height="138" rx="14" class="mon-bezel" />
						<rect x="33" y="17" width="174" height="116" rx="7" class="mon-screen" />
						<!-- on-screen knowledge core: cloud, database, linked nodes -->
						<path
							class="scr-stroke"
							d="M150 44c0-6-5-11-11-11-1-5-6-9-12-9-5 0-10 3-11 8-5 0-9 4-9 9 0 5 4 9 10 9h35c5 0 9-4 9-8 0-3-2-5-5-6z"
						/>
						<g class="scr-node">
							<circle cx="74" cy="60" r="6" />
							<circle cx="166" cy="96" r="6" />
							<circle cx="92" cy="108" r="6" />
						</g>
						<g class="scr-link">
							<line x1="74" y1="60" x2="120" y2="86" />
							<line x1="166" y1="96" x2="120" y2="86" />
							<line x1="92" y1="108" x2="120" y2="86" />
						</g>
						<!-- central database cylinder -->
						<g class="scr-db">
							<ellipse cx="120" cy="74" rx="17" ry="6" />
							<path d="M103 74v18c0 3 8 6 17 6s17-3 17-6V74" />
							<path class="scr-db-line" d="M103 83c0 3 8 6 17 6s17-3 17-6" />
						</g>
						<text x="120" y="125" class="mon-label">SemOS KB</text>
						<!-- stand -->
						<rect x="109" y="144" width="22" height="26" rx="3" class="mon-stand" />
						<rect x="84" y="170" width="72" height="12" rx="6" class="mon-stand" />
					</svg>
				</div>

				<div class="stat-grid">
					{#each positioned as n (n.n)}
						{@const Icon = n.icon}
						<a class="stat-node" href={n.href} style={compact ? '' : `--x:${n.x}px; --y:${n.y}px`}>
							<span class="stat-icon"><Icon size={20} strokeWidth={1.75} /></span>
							<span class="stat-label"><span class="stat-n">{n.n}</span>{n.label}</span>
							<span class="stat-value">{fmt(n.value)}</span>
						</a>
					{/each}
				</div>
			</div>

			<!-- search + language -->
			<form class="search" onsubmit={runSearch} role="search">
				<svg class="search-icon" viewBox="0 0 24 24" aria-hidden="true">
					<circle cx="11" cy="11" r="7" />
					<line x1="21" y1="21" x2="16.65" y2="16.65" />
				</svg>
				<input
					type="search"
					placeholder="Search the knowledge base"
					aria-label="Search the knowledge base"
					bind:value={query}
					autocomplete="off"
				/>
				<label class="lang">
					<span class="sr-only">Language</span>
					<select value={currentLocale} onchange={changeLocale} aria-label="Select language">
						{#each locales as code}
							<option value={code}>{LOCALE_LABELS[code] ?? code}</option>
						{/each}
					</select>
				</label>
				<button type="submit">Search</button>
			</form>
		</section>

		<!-- Panel B: portal directory --------------------------------------- -->
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
	</div>

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

	/* ---- Masthead: Panel A | Panel B -------------------------------------- */
	.masthead {
		display: grid;
		grid-template-columns: minmax(0, 1.55fr) minmax(280px, 0.85fr);
		gap: clamp(1.5rem, 3vw, 3.5rem);
		max-width: 1320px;
		margin: 0 auto;
		align-items: start;
	}

	/* ---- Panel A ---------------------------------------------------------- */
	.panel-a {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
	}
	.eyebrow {
		margin: 0;
		font-size: 0.72rem;
		letter-spacing: 0.22em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.wordmark {
		margin: 0.15rem 0 0;
		font-family: var(--serif);
		font-weight: 400;
		font-size: clamp(2.6rem, 4.4vw, 3.6rem);
		letter-spacing: -0.01em;
		color: var(--ink);
	}

	.hero-ring {
		position: relative;
		width: 100%;
		height: 632px;
		display: flex;
		align-items: center;
		justify-content: center;
		margin: 0.25rem 0 1.5rem;
	}
	.connectors {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		pointer-events: none;
	}
	.connectors line {
		stroke: var(--accent);
		stroke-width: 1.25;
		stroke-dasharray: 2 6;
		stroke-linecap: round;
		opacity: 0.5;
	}

	/* SemOS KB core: the workstation monitor ------------------------------- */
	.core {
		position: relative;
		width: 252px;
		z-index: 2;
	}
	.core-svg {
		width: 100%;
		height: auto;
		overflow: visible;
	}
	.mon-bezel {
		fill: var(--ink);
	}
	.mon-screen {
		fill: var(--surface);
	}
	.scr-stroke {
		fill: none;
		stroke: var(--ink-faint);
		stroke-width: 2.25;
		stroke-linejoin: round;
	}
	.scr-node circle {
		fill: var(--accent);
	}
	.scr-link line {
		stroke: var(--ink-faint);
		stroke-width: 1.5;
	}
	.scr-db ellipse {
		fill: var(--accent-tint);
		stroke: var(--accent-strong);
		stroke-width: 2;
	}
	.scr-db path {
		fill: none;
		stroke: var(--accent-strong);
		stroke-width: 2;
	}
	.mon-label {
		fill: var(--accent-strong);
		font-family: var(--serif);
		font-size: 16px;
		font-weight: 600;
		text-anchor: middle;
		letter-spacing: 0.01em;
	}
	.mon-stand {
		fill: var(--ink-soft);
	}

	/* In radial mode the grid wrapper is transparent so each node positions
	   absolutely against .hero-ring; in compact mode it becomes a real grid. */
	.stat-grid {
		display: contents;
	}

	/* Stat node -------------------------------------------------------------*/
	.stat-node {
		position: absolute;
		left: 50%;
		top: 50%;
		transform: translate(calc(-50% + var(--x)), calc(-50% + var(--y)));
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.3rem;
		width: 146px;
		padding: 0.55rem 0.45rem;
		border-radius: 14px;
		text-decoration: none;
		color: var(--ink);
		z-index: 3;
		transition:
			background-color 180ms cubic-bezier(0.22, 1, 0.36, 1),
			box-shadow 180ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.stat-icon {
		display: grid;
		place-items: center;
		width: 46px;
		height: 46px;
		border-radius: 50%;
		background: var(--accent-tint);
		color: var(--accent-strong);
		border: 1px solid oklch(0.87 0.035 210);
		transition:
			transform 220ms cubic-bezier(0.22, 1, 0.36, 1),
			background-color 180ms ease;
	}
	.stat-node:hover {
		background: var(--surface);
		box-shadow: 0 6px 22px oklch(0.4 0.02 270 / 0.1);
	}
	.stat-node:hover .stat-icon {
		transform: translateY(-2px);
		background: oklch(0.92 0.05 210);
	}
	.stat-node:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.stat-label {
		font-size: 0.78rem;
		font-weight: 600;
		line-height: 1.2;
		color: var(--ink);
	}
	.stat-n {
		color: var(--accent-strong);
		font-weight: 700;
		margin-right: 0.25rem;
	}
	.stat-value {
		font-family: var(--serif);
		font-size: 1.4rem;
		line-height: 1;
		color: var(--ink-soft);
		font-variant-numeric: tabular-nums;
	}
	.stat-node:hover .stat-value {
		color: var(--accent-strong);
	}

	/* Compact: constellation becomes a grid of chips ---------------------- */
	.hero-ring.compact {
		height: auto;
		flex-direction: column;
		gap: 1.25rem;
		padding: 1.5rem 0 0;
	}
	.hero-ring.compact .core {
		margin-bottom: 0.25rem;
		width: 220px;
	}
	.hero-ring.compact .stat-grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.5rem;
		width: 100%;
	}
	.hero-ring.compact .stat-node {
		position: static;
		transform: none;
		width: auto;
		border: 1px solid var(--line);
	}

	/* Search ---------------------------------------------------------------*/
	.search {
		display: flex;
		align-items: stretch;
		width: min(640px, 100%);
		background: var(--surface);
		border: 1px solid var(--line);
		border-radius: 14px;
		box-shadow: 0 2px 10px oklch(0.4 0.02 270 / 0.05);
		overflow: hidden;
		transition:
			border-color 180ms ease,
			box-shadow 180ms ease;
	}
	.search:focus-within {
		border-color: var(--accent);
		box-shadow: 0 0 0 3px var(--accent-tint);
	}
	.search-icon {
		width: 20px;
		height: 20px;
		align-self: center;
		margin-left: 0.85rem;
		fill: none;
		stroke: var(--ink-faint);
		stroke-width: 2;
		stroke-linecap: round;
	}
	.search input {
		flex: 1;
		border: 0;
		background: transparent;
		padding: 0.85rem 0.75rem;
		font-size: 0.95rem;
		color: var(--ink);
		min-width: 0;
	}
	.search input:focus {
		outline: none;
	}
	.lang {
		display: flex;
		align-items: center;
		border-left: 1px solid var(--line);
	}
	.lang select {
		height: 100%;
		border: 0;
		background: transparent;
		padding: 0 0.5rem;
		font-size: 0.85rem;
		color: var(--ink-soft);
		cursor: pointer;
	}
	.lang select:focus {
		outline: none;
		color: var(--accent-strong);
	}
	.search button {
		border: 0;
		background: var(--accent);
		color: oklch(0.99 0.01 210);
		padding: 0 1.35rem;
		font-size: 0.9rem;
		font-weight: 600;
		cursor: pointer;
		transition: background-color 160ms ease;
	}
	.search button:hover {
		background: var(--accent-strong);
	}
	.search button:focus-visible {
		outline: 2px solid var(--accent-strong);
		outline-offset: 2px;
	}

	/* ---- Panel B ---------------------------------------------------------- */
	.panel-b {
		border-left: 1px solid var(--line);
		padding-left: clamp(1.25rem, 2.5vw, 2.5rem);
		padding-top: 2.6rem;
	}
	.panel-title {
		font-family: var(--serif);
		font-weight: 400;
		font-size: 1.35rem;
		margin: 0 0 0.35rem;
		color: var(--ink);
	}
	.portals {
		display: flex;
		flex-direction: column;
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

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	/* ---- Responsive ------------------------------------------------------- */
	@media (max-width: 980px) {
		.masthead {
			grid-template-columns: 1fr;
		}
		.panel-b {
			border-left: 0;
			border-top: 1px solid var(--line);
			padding-left: 0;
			padding-top: 1.5rem;
			margin-top: 1rem;
		}
		.panel-foot {
			max-width: none;
		}
	}
	@media (max-width: 760px) {
		.activity {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 540px) {
		/* let the search controls wrap instead of overflowing */
		.search {
			flex-wrap: wrap;
		}
		.search input {
			flex: 1 1 100%;
			order: -1;
			border-bottom: 1px solid var(--line);
		}
		.lang {
			flex: 1;
			border-left: 0;
		}
		.search button {
			flex: 1;
			padding: 0.75rem 1rem;
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
		.stat-node,
		.stat-icon,
		.portal,
		.search,
		.feed li a {
			transition: none;
		}
	}
</style>
