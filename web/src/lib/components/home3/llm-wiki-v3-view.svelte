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

	// v3 keeps the original "LLM Wiki" (/deep-wiki) foundation: its light paper/ink
	// design, the "Explore the wiki" directory (Panel B), and the recent-activity
	// feeds (Panel C). Only Panel A is replaced with the improved, artistically
	// wired SemOS KB diagram (rounded dashed connectors, larger icons). A dark
	// variant is added so the view also reads well inside the dark knowledge shell.
	let { darkMode = true }: { darkMode?: boolean } = $props();

	let overview = $state<WikiOverviewResponse | null>(null);
	let loadError = $state<string | null>(null);
	let loading = $state(true);
	let query = $state('');
	let currentLocale = $state<string>('en');
	let rootWidth = $state(1200);

	const KNOWLEDGE = '/home3/knowledge?section=';
	const GRAPH_HREF = '/knowledge-graph';
	const LOCALE_LABELS: Record<string, string> = { en: 'English', 'zh-cn': '中文' };

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

	type Tone = 'mint' | 'cyan' | 'amber' | 'coral' | 'iris';

	type AtlasItem = {
		n: number;
		label: string;
		keys: ArtifactKey[];
		icon: typeof FilesIcon;
		href: string;
		tone: Tone;
		x: number; // stage coordinate (viewBox 0..STAGE_W)
		y: number; // stage coordinate (viewBox 0..STAGE_H)
	};

	// Stage coordinate system. The container keeps this exact aspect ratio so SVG
	// units and HTML percentage offsets resolve to the same on-screen points.
	const STAGE_W = 960;
	const STAGE_H = 600;
	const CX = STAGE_W / 2;
	const CY = 286;
	const HALF_W = 176; // monitor half-extents, used to anchor connectors on its edge
	const HALF_H = 124;

	// Positions follow the SemOS KB system diagram: the core in the middle, the
	// nine dimensions scattered around it.
	const ATLAS_ITEMS: AtlasItem[] = [
		{
			n: 1,
			label: 'Documents',
			keys: ['documents'],
			icon: FilesIcon,
			href: KNOWLEDGE + 'kb-input-details',
			tone: 'cyan',
			x: 392,
			y: 70
		},
		{
			n: 2,
			label: 'Content Segments',
			keys: ['content_segments'],
			icon: FileSearchIcon,
			href: KNOWLEDGE + 'kb-chunks',
			tone: 'iris',
			x: 104,
			y: 268
		},
		{
			n: 3,
			label: 'Topics',
			keys: ['topics'],
			icon: PresentationIcon,
			href: KNOWLEDGE + 'kb-topic-tree',
			tone: 'amber',
			x: 138,
			y: 92
		},
		{
			n: 4,
			label: 'Semantic Projections',
			keys: ['semantic_projections'],
			icon: GlobeIcon,
			href: KNOWLEDGE + 'kb-semantic-projections',
			tone: 'mint',
			x: 846,
			y: 470
		},
		{
			n: 5,
			label: 'Entities & Relations',
			keys: ['entities', 'relations'],
			icon: NetworkIcon,
			href: GRAPH_HREF,
			tone: 'coral',
			x: 792,
			y: 92
		},
		{
			n: 6,
			label: 'Metrics',
			keys: ['metrics'],
			icon: ChartBarIcon,
			href: KNOWLEDGE + 'kb-metrics',
			tone: 'cyan',
			x: 866,
			y: 264
		},
		{
			n: 7,
			label: 'Parts & Components',
			keys: ['parts_components'],
			icon: BoxesIcon,
			href: KNOWLEDGE + 'kb-products',
			tone: 'iris',
			x: 120,
			y: 506
		},
		{
			n: 8,
			label: 'Provisions',
			keys: ['provisions'],
			icon: ScaleIcon,
			href: KNOWLEDGE + 'kb-provision-tree',
			tone: 'amber',
			x: 412,
			y: 548
		},
		{
			n: 9,
			label: 'Scenes',
			keys: ['scenes'],
			icon: ClapperboardIcon,
			href: KNOWLEDGE + 'kb-scene-blocks',
			tone: 'mint',
			x: 648,
			y: 540
		}
	];

	// Anchor a connector on the monitor edge in the direction of the node.
	function anchor(nx: number, ny: number): { x: number; y: number } {
		const dx = nx - CX;
		const dy = ny - CY;
		const t = Math.min(HALF_W / Math.max(Math.abs(dx), 1), HALF_H / Math.max(Math.abs(dy), 1));
		return { x: CX + dx * t, y: CY + dy * t };
	}

	// A smooth (rounded), dashed cubic curve from monitor edge to node, bowed along
	// the dominant axis so it reads as a hand-routed wire, never a straight spoke.
	function curve(nx: number, ny: number, a: { x: number; y: number }): string {
		const dx = nx - a.x;
		const dy = ny - a.y;
		let c1x: number, c1y: number, c2x: number, c2y: number;
		if (Math.abs(dx) >= Math.abs(dy)) {
			const mx = a.x + dx * 0.5;
			c1x = mx;
			c1y = a.y;
			c2x = mx;
			c2y = ny;
		} else {
			const my = a.y + dy * 0.5;
			c1x = a.x;
			c1y = my;
			c2x = nx;
			c2y = my;
		}
		return `M ${a.x} ${a.y} C ${c1x} ${c1y} ${c2x} ${c2y} ${nx} ${ny}`;
	}

	// Connectors are pure geometry over fixed positions, so compute once.
	const CONNECTORS = ATLAS_ITEMS.map((it) => {
		const a = anchor(it.x, it.y);
		return { n: it.n, tone: it.tone, ax: a.x, ay: a.y, d: curve(it.x, it.y, a) };
	});

	function pct(value: number, total: number): string {
		return `${(value / total) * 100}%`;
	}

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
	const compact = $derived(rootWidth > 0 && rootWidth < 900);
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

	onMount(() => {
		load();
		try {
			currentLocale = getLocale();
		} catch {
			currentLocale = 'en';
		}
	});

	function valueFor(item: AtlasItem): number | null {
		if (!overview) return null;
		return item.keys.reduce((sum, key) => sum + (overview?.counts[key] ?? 0), 0);
	}

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

	function runSearch(event: SubmitEvent) {
		event.preventDefault();
		const q = query.trim();
		goto(`/home3/knowledge?section=kb-search${q ? `&q=${encodeURIComponent(q)}` : ''}`);
	}

	function changeLocale(event: Event) {
		const next = (event.target as HTMLSelectElement).value;
		currentLocale = next;
		try {
			setLocale(next as (typeof locales)[number]);
		} catch {
			/* setLocale owns its own reload strategy. */
		}
	}
</script>

<div class="wiki" class:dark={darkMode} class:light={!darkMode} bind:clientWidth={rootWidth}>
	<!-- ===================== PANEL A (improved, wired) ===================== -->
	<section class="hero" aria-label="Corpus overview">
		<p class="eyebrow">The deep knowledge base</p>
		<h1 class="wordmark">SemOS</h1>

		{#if !compact}
			<div class="stage" style="aspect-ratio:{STAGE_W} / {STAGE_H};">
				<svg class="wires" viewBox="0 0 {STAGE_W} {STAGE_H}" aria-hidden="true">
					{#each CONNECTORS as c (c.n)}
						<path class="wire tone-{c.tone}" d={c.d} />
						<circle class="wire-dot tone-{c.tone}" cx={c.ax} cy={c.ay} r="4.5" />
					{/each}

					<!-- SemOS KB monitor -->
					<rect x={CX - 158} y="166" width="316" height="210" rx="20" class="mon-frame" />
					<rect x={CX - 142} y="182" width="284" height="160" rx="10" class="mon-screen" />
					<path
						class="scr-cloud"
						d="M{CX +
							36} 232c0-9-7-16-16-16-2-8-9-13-17-13-7 0-14 5-16 12-8 0-14 6-14 14s7 14 15 14h50c8 0 14-6 14-13 0-6-4-10-10-12z"
					/>
					<g class="scr-doc">
						<rect x={CX - 118} y="222" width="44" height="56" rx="5" />
						<path d="M{CX - 108} 236h24M{CX - 108} 248h20M{CX - 108} 260h26" />
					</g>
					<g class="scr-db">
						<ellipse cx={CX} cy="248" rx="26" ry="9" />
						<path d="M{CX - 26} 248v44c0 5 12 10 26 10s26-5 26-10v-44" />
						<path d="M{CX - 26} 264c0 5 12 10 26 10s26-5 26-10" />
					</g>
					<g class="scr-graph">
						<circle cx={CX + 96} cy="232" r="11" />
						<circle cx={CX + 74} cy="276" r="9" />
						<circle cx={CX + 116} cy="282" r="9" />
						<path d="M{CX + 96} 232l-22 44M{CX + 96} 232l20 50M{CX + 74} 276l42 6" />
					</g>
					<text x={CX} y="326" class="mon-label">SemOS KB</text>
					<rect x={CX - 15} y="376" width="30" height="26" class="mon-stand" />
					<rect x={CX - 54} y="400" width="108" height="15" rx="7" class="mon-stand" />
				</svg>

				{#each ATLAS_ITEMS as item (item.n)}
					{@const Icon = item.icon}
					<a
						class="node tone-{item.tone}"
						href={item.href}
						style="left:{pct(item.x, STAGE_W)};top:{pct(item.y, STAGE_H)};opacity:{loading
							? 0.6
							: 1};"
					>
						<span class="node-badge">
							<Icon size={30} strokeWidth={1.7} />
							<span class="node-num">{item.n}</span>
						</span>
						<span class="node-copy">
							<span class="node-label">{item.label}</span>
							<span class="node-value">{fmt(valueFor(item))}</span>
						</span>
					</a>
				{/each}
			</div>
		{:else}
			<div class="compact-core">
				<svg viewBox="0 0 320 250" aria-hidden="true">
					<rect x="20" y="14" width="280" height="190" rx="18" class="mon-frame" />
					<rect x="34" y="28" width="252" height="142" rx="9" class="mon-screen" />
					<g class="scr-db">
						<ellipse cx="160" cy="84" rx="26" ry="9" />
						<path d="M134 84v44c0 5 12 10 26 10s26-5 26-10V84" />
						<path d="M134 100c0 5 12 10 26 10s26-5 26-10" />
					</g>
					<text x="160" y="158" class="mon-label">SemOS KB</text>
					<rect x="145" y="204" width="30" height="24" class="mon-stand" />
					<rect x="108" y="226" width="104" height="14" rx="7" class="mon-stand" />
				</svg>
			</div>
			<div class="node-grid">
				{#each ATLAS_ITEMS as item (item.n)}
					{@const Icon = item.icon}
					<a class="node tone-{item.tone}" href={item.href} style="opacity:{loading ? 0.6 : 1};">
						<span class="node-badge">
							<Icon size={28} strokeWidth={1.7} />
							<span class="node-num">{item.n}</span>
						</span>
						<span class="node-copy">
							<span class="node-label">{item.label}</span>
							<span class="node-value">{fmt(valueFor(item))}</span>
						</span>
					</a>
				{/each}
			</div>
		{/if}

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

		/* Panel A diagram: per-dimension tones + monitor parts */
		--mint: oklch(0.6 0.12 165);
		--cyan: oklch(0.55 0.11 215);
		--amber: oklch(0.66 0.12 75);
		--coral: oklch(0.6 0.15 30);
		--iris: oklch(0.55 0.14 285);
		--mon-bezel: oklch(0.3 0.02 265);
		--mon-screen: oklch(0.965 0.006 95);
		--mon-ink: oklch(0.33 0.02 265);

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

		--mint: oklch(0.74 0.13 165);
		--cyan: oklch(0.72 0.12 210);
		--amber: oklch(0.8 0.12 80);
		--coral: oklch(0.72 0.15 30);
		--iris: oklch(0.72 0.13 285);
		--mon-bezel: oklch(0.12 0.008 260);
		--mon-screen: oklch(0.14 0.01 250);
		--mon-ink: oklch(0.82 0.02 250);

		background:
			radial-gradient(130% 80% at 50% -8%, oklch(0.24 0.03 230) 0%, transparent 60%), var(--paper);
	}

	/* ---- Tones ---- */
	.tone-mint {
		--tone: var(--mint);
	}
	.tone-cyan {
		--tone: var(--cyan);
	}
	.tone-amber {
		--tone: var(--amber);
	}
	.tone-coral {
		--tone: var(--coral);
	}
	.tone-iris {
		--tone: var(--iris);
	}

	/* ---- Panel A: hero ---- */
	.hero {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		max-width: 1100px;
		margin: 0 auto;
	}
	.eyebrow {
		margin: 0;
		font-size: 0.72rem;
		letter-spacing: 0.22em;
		text-transform: uppercase;
		color: var(--ink-faint);
	}
	.wordmark {
		margin: 0.15rem 0 0.5rem;
		font-family: var(--serif);
		font-weight: 400;
		font-size: clamp(2.4rem, 4.2vw, 3.4rem);
		letter-spacing: -0.01em;
		color: var(--ink);
	}

	.stage {
		position: relative;
		width: 100%;
		max-width: 1000px;
		margin: 0.25rem auto 1.25rem;
	}
	.wires {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		overflow: visible;
	}
	.wire {
		fill: none;
		stroke: var(--tone);
		stroke-width: 2;
		stroke-linecap: round;
		stroke-dasharray: 1.5 9;
		opacity: 0.62;
		vector-effect: non-scaling-stroke;
	}
	.wire-dot {
		fill: var(--tone);
		opacity: 0.9;
	}

	.mon-frame {
		fill: var(--mon-bezel);
	}
	.mon-screen {
		fill: var(--mon-screen);
	}
	.scr-cloud {
		fill: none;
		stroke: var(--mon-ink);
		stroke-width: 2.5;
		stroke-linejoin: round;
		opacity: 0.6;
	}
	.scr-doc rect {
		fill: color-mix(in srgb, var(--mon-screen) 80%, var(--cyan));
		stroke: var(--mon-ink);
		stroke-width: 2.5;
	}
	.scr-doc path {
		fill: none;
		stroke: var(--mon-ink);
		stroke-width: 2.5;
		stroke-linecap: round;
		opacity: 0.62;
	}
	.scr-db ellipse,
	.scr-db path {
		fill: color-mix(in srgb, var(--mint) 32%, var(--mon-screen));
		stroke: var(--mon-ink);
		stroke-width: 2.5;
	}
	.scr-db path {
		fill: none;
	}
	.scr-graph circle {
		fill: color-mix(in srgb, var(--coral) 60%, var(--mon-screen));
		stroke: var(--mon-ink);
		stroke-width: 2.5;
	}
	.scr-graph path {
		fill: none;
		stroke: var(--mon-ink);
		stroke-width: 2.5;
		stroke-linecap: round;
	}
	.mon-label {
		fill: var(--accent-strong);
		font-family: var(--serif);
		font-size: 21px;
		font-weight: 600;
		text-anchor: middle;
		letter-spacing: 0.01em;
	}
	.mon-stand {
		fill: var(--mon-bezel);
	}

	/* ---- Node card (shared by stage + compact grid) ---- */
	.node {
		display: flex;
		align-items: center;
		gap: 12px;
		width: 184px;
		padding: 11px 13px;
		border: 1px solid var(--line);
		border-radius: 14px;
		background: var(--surface);
		box-shadow: 0 1px 3px oklch(0.4 0.02 270 / 0.06);
		color: var(--ink);
		text-decoration: none;
		transition:
			transform 200ms cubic-bezier(0.22, 1, 0.36, 1),
			border-color 200ms ease,
			box-shadow 200ms ease;
	}
	.stage .node {
		position: absolute;
		transform: translate(-50%, -50%);
		z-index: 2;
	}
	.node:hover {
		border-color: color-mix(in srgb, var(--tone) 58%, var(--line));
		box-shadow: 0 10px 26px oklch(0.4 0.02 270 / 0.14);
	}
	.stage .node:hover {
		transform: translate(-50%, -50%) scale(1.04);
	}
	.node:focus-visible {
		outline: 2px solid var(--tone);
		outline-offset: 3px;
	}
	.node-badge {
		position: relative;
		display: grid;
		place-items: center;
		flex-shrink: 0;
		width: 54px;
		height: 54px;
		border-radius: 15px;
		background: color-mix(in srgb, var(--tone) 16%, transparent);
		border: 1px solid color-mix(in srgb, var(--tone) 38%, transparent);
		color: var(--tone);
	}
	.node-num {
		position: absolute;
		top: -7px;
		right: -7px;
		display: grid;
		place-items: center;
		width: 20px;
		height: 20px;
		border-radius: 50%;
		background: var(--tone);
		color: var(--paper);
		font-family: var(--serif);
		font-size: 0.72rem;
		font-weight: 700;
	}
	.node-copy {
		display: grid;
		gap: 2px;
		min-width: 0;
		text-align: left;
	}
	.node-label {
		color: var(--ink);
		font-size: 0.86rem;
		font-weight: 700;
		line-height: 1.15;
	}
	.node-value {
		color: var(--tone);
		font-family: var(--serif);
		font-size: 1.35rem;
		line-height: 1;
		font-variant-numeric: tabular-nums;
	}

	/* ---- Compact layout ---- */
	.compact-core {
		display: grid;
		place-items: center;
		padding: 8px 0 4px;
	}
	.compact-core svg {
		width: min(100%, 300px);
		height: auto;
		overflow: visible;
	}
	.node-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 10px;
		width: 100%;
		margin-bottom: 0.5rem;
	}
	.node-grid .node {
		width: auto;
	}

	/* ---- Search ---- */
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

	@media (max-width: 760px) {
		.activity {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
	@media (max-width: 540px) {
		.activity {
			grid-template-columns: 1fr;
		}
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

	@media (prefers-reduced-motion: reduce) {
		.node,
		.portal,
		.search,
		.feed li a {
			transition: none;
		}
		.sk-bar {
			animation: none;
		}
	}
</style>
