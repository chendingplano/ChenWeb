<script lang="ts">
	import { onMount } from 'svelte';
	import type { WikiOverviewResponse } from '$lib/services/kbService';
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

	// The SemOS KB "Top Panel": the artistically wired knowledge-base diagram plus
	// the knowledge-base search box. Extracted from LLM Wiki v3 so the same hero can
	// be reused as the entrance of any wiki surface. The component is self-contained
	// (it carries its own palette + Search form); the parent only supplies the
	// corpus overview counts and the dark/light flag.
	let {
		darkMode = true,
		overview = null,
		loading = false
	}: {
		darkMode?: boolean;
		overview?: WikiOverviewResponse | null;
		loading?: boolean;
	} = $props();

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

	const nf = new Intl.NumberFormat('en-US');
	const compact = $derived(rootWidth > 0 && rootWidth < 900);

	function valueFor(item: AtlasItem): number | null {
		if (!overview) return null;
		return item.keys.reduce((sum, key) => sum + (overview?.counts[key] ?? 0), 0);
	}

	function fmt(value: number | null): string {
		return value == null ? '—' : nf.format(value);
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

	onMount(() => {
		try {
			currentLocale = getLocale();
		} catch {
			currentLocale = 'en';
		}
	});
</script>

<section
	class="kb-hero"
	class:dark={darkMode}
	class:light={!darkMode}
	aria-label="Corpus overview"
	bind:clientWidth={rootWidth}
>
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
	<!-- data-sveltekit-reload: let the browser own the GET navigation. Without it
	     SvelteKit intercepts the submit and routes client-side, which the proxied
	     Home3 shell can swallow, so Search appears to do nothing. -->
	<form
		class="search"
		action="/home3/knowledge"
		method="GET"
		role="search"
		data-sveltekit-reload
	>
		<input type="hidden" name="section" value="kb-search" />
		<input type="hidden" name="dark" value={darkMode ? '1' : '0'} />
		<svg class="search-icon" viewBox="0 0 24 24" aria-hidden="true">
			<circle cx="11" cy="11" r="7" />
			<line x1="21" y1="21" x2="16.65" y2="16.65" />
		</svg>
		<input
			type="search"
			name="q"
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

<style>
	/* Self-contained palette so the hero can drop into any wiki surface. */
	.kb-hero {
		--paper: oklch(0.985 0.004 95);
		--surface: oklch(0.997 0.003 95);
		--ink: oklch(0.26 0.018 275);
		--ink-faint: oklch(0.62 0.018 270);
		--line: oklch(0.9 0.008 95);
		--accent: oklch(0.52 0.1 213);
		--accent-strong: oklch(0.43 0.11 214);
		--accent-tint: oklch(0.95 0.025 210);

		/* per-dimension tones + monitor parts */
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

		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		max-width: 1100px;
		margin: 0 auto;
		font-family: var(--sans);
		color: var(--ink);
	}

	.kb-hero.dark {
		--paper: oklch(0.17 0.012 265);
		--surface: oklch(0.21 0.014 265);
		--ink: oklch(0.95 0.008 250);
		--ink-faint: oklch(0.62 0.02 258);
		--line: oklch(1 0 0 / 0.12);
		--accent: oklch(0.72 0.11 208);
		--accent-strong: oklch(0.8 0.11 205);
		--accent-tint: oklch(0.72 0.11 208 / 0.16);

		--mint: oklch(0.74 0.13 165);
		--cyan: oklch(0.72 0.12 210);
		--amber: oklch(0.8 0.12 80);
		--coral: oklch(0.72 0.15 30);
		--iris: oklch(0.72 0.13 285);
		--mon-bezel: oklch(0.12 0.008 260);
		--mon-screen: oklch(0.14 0.01 250);
		--mon-ink: oklch(0.82 0.02 250);
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
		color: var(--ink-faint);
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

	@media (max-width: 540px) {
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
		.search {
			transition: none;
		}
	}
</style>
