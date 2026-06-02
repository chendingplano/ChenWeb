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
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import SearchIcon from '@lucide/svelte/icons/search';
	import LanguagesIcon from '@lucide/svelte/icons/languages';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ArrowUpRightIcon from '@lucide/svelte/icons/arrow-up-right';

	let {
		darkMode = true
	}: {
		darkMode?: boolean;
	} = $props();

	let overview = $state<WikiOverviewResponse | null>(null);
	let loadError = $state<string | null>(null);
	let loading = $state(true);
	let query = $state('');
	let currentLocale = $state<string>('en');

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

	type AtlasItem = {
		n: number;
		label: string;
		shortLabel: string;
		keys: ArtifactKey[];
		icon: typeof FilesIcon;
		href: string;
		tone: 'paper' | 'mint' | 'cyan' | 'amber' | 'coral';
	};

	const ATLAS_ITEMS: AtlasItem[] = [
		{
			n: 1,
			label: 'Documents archive',
			shortLabel: 'Documents',
			keys: ['documents'],
			icon: FilesIcon,
			href: KNOWLEDGE + 'kb-input-details',
			tone: 'paper'
		},
		{
			n: 2,
			label: 'Content segments',
			shortLabel: 'Segments',
			keys: ['content_segments'],
			icon: FileSearchIcon,
			href: KNOWLEDGE + 'kb-chunks',
			tone: 'cyan'
		},
		{
			n: 3,
			label: 'Topics',
			shortLabel: 'Topics',
			keys: ['topics'],
			icon: PresentationIcon,
			href: KNOWLEDGE + 'kb-topic-tree',
			tone: 'amber'
		},
		{
			n: 4,
			label: 'Semantic projections',
			shortLabel: 'Projections',
			keys: ['semantic_projections'],
			icon: GlobeIcon,
			href: KNOWLEDGE + 'kb-semantic-projections',
			tone: 'mint'
		},
		{
			n: 5,
			label: 'Entities and relations',
			shortLabel: 'Graph',
			keys: ['entities', 'relations'],
			icon: NetworkIcon,
			href: GRAPH_HREF,
			tone: 'coral'
		},
		{
			n: 6,
			label: 'Metrics',
			shortLabel: 'Metrics',
			keys: ['metrics'],
			icon: ChartBarIcon,
			href: KNOWLEDGE + 'kb-metrics',
			tone: 'cyan'
		},
		{
			n: 7,
			label: 'Parts and components',
			shortLabel: 'Parts',
			keys: ['parts_components'],
			icon: BoxesIcon,
			href: KNOWLEDGE + 'kb-products',
			tone: 'amber'
		},
		{
			n: 8,
			label: 'Provisions',
			shortLabel: 'Provisions',
			keys: ['provisions'],
			icon: ScaleIcon,
			href: KNOWLEDGE + 'kb-provision-tree',
			tone: 'paper'
		},
		{
			n: 9,
			label: 'Scenes',
			shortLabel: 'Scenes',
			keys: ['scenes'],
			icon: ClapperboardIcon,
			href: KNOWLEDGE + 'kb-scene-blocks',
			tone: 'mint'
		}
	];

	const LEFT_ITEMS = ATLAS_ITEMS.slice(0, 4);
	const RIGHT_ITEMS = ATLAS_ITEMS.slice(4);

	const PORTALS: { label: string; blurb: string; href: string; tone: AtlasItem['tone'] }[] = [
		{
			label: 'Documents',
			blurb: 'Documents, notes, articles',
			href: KNOWLEDGE + 'kb-input-details',
			tone: 'paper'
		},
		{
			label: 'Content Segments',
			blurb: 'Content segments and chunks',
			href: KNOWLEDGE + 'kb-chunks',
			tone: 'cyan'
		},
		{
			label: 'Topics',
			blurb: 'Browse and search topics',
			href: KNOWLEDGE + 'kb-topic-tree',
			tone: 'amber'
		},
		{
			label: 'Metrics',
			blurb: 'Browse and search metrics',
			href: KNOWLEDGE + 'kb-metrics',
			tone: 'cyan'
		},
		{
			label: 'Parts and Components',
			blurb: 'Browse products and parts',
			href: KNOWLEDGE + 'kb-products',
			tone: 'amber'
		},
		{
			label: 'Scenes',
			blurb: 'Browse event scenes',
			href: KNOWLEDGE + 'kb-scene-blocks',
			tone: 'mint'
		},
		{
			label: 'Provisions',
			blurb: 'Browse provisions',
			href: KNOWLEDGE + 'kb-provision-tree',
			tone: 'paper'
		},
		{
			label: 'Graphs',
			blurb: 'Entities and relations',
			href: GRAPH_HREF,
			tone: 'coral'
		}
	];

	const nf = new Intl.NumberFormat('en-US');

	const totalArtifacts = $derived(
		overview
			? ATLAS_ITEMS.reduce(
					(sum, item) => sum + item.keys.reduce((inner, key) => inner + (overview?.counts[key] ?? 0), 0),
					0
				)
			: null
	);
	const graphCount = $derived(
		overview ? (overview.counts.entities ?? 0) + (overview.counts.relations ?? 0) : null
	);

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
			/* Locale runtime owns reload behavior. */
		}
	}
</script>

<div
	class="llm-wiki-v2"
	class:dark={darkMode}
	class:light={!darkMode}
	style="--load-opacity:{loading ? 0.58 : 1};"
>
	<section class="atlas-panel" aria-label="SemOS knowledge atlas">
		<div class="atlas-kicker">
			<span>SemOS KB</span>
			<span>LLM Wiki v2</span>
		</div>

		<div class="atlas-layout">
			<div class="artifact-column left">
				{#each LEFT_ITEMS as item (item.n)}
					{@render atlasCard(item)}
				{/each}
			</div>

			<div class="atlas-core" aria-label="SemOS KB core">
				<div class="core-label">Deep Wiki</div>
				<div class="core-device">
					<svg viewBox="0 0 360 320" class="core-svg" aria-hidden="true">
						<defs>
							<pattern id="llmWikiGrid" width="22" height="22" patternUnits="userSpaceOnUse">
								<path d="M22 0H0V22" />
							</pattern>
						</defs>
						<rect x="42" y="38" width="276" height="174" rx="18" class="device-shell" />
						<rect x="58" y="54" width="244" height="128" rx="10" class="device-screen" />
						<rect x="58" y="54" width="244" height="128" rx="10" class="device-grid" />
						<path
							class="link-line link-a"
							d="M100 109C127 87 151 85 180 112C206 137 232 133 263 98"
						/>
						<path
							class="link-line link-b"
							d="M96 145C129 129 153 137 180 124C211 110 237 120 266 151"
						/>
						<g class="doc-stack">
							<rect x="86" y="76" width="48" height="60" rx="5" />
							<path d="M96 92h25M96 106h22M96 120h29" />
						</g>
						<g class="db-core">
							<ellipse cx="181" cy="107" rx="28" ry="10" />
							<path d="M153 107v50c0 6 13 11 28 11s28-5 28-11v-50" />
							<path d="M153 124c0 6 13 11 28 11s28-5 28-11" />
							<path d="M153 141c0 6 13 11 28 11s28-5 28-11" />
						</g>
						<g class="graph-nodes">
							<circle cx="260" cy="91" r="12" />
							<circle cx="238" cy="139" r="10" />
							<circle cx="282" cy="147" r="10" />
							<path d="M260 91l-22 48M260 91l22 56M238 139l44 8" />
						</g>
						<path class="device-base" d="M159 212h42l9 42h-60z" />
						<rect x="118" y="254" width="124" height="18" rx="9" class="device-foot" />
						<circle cx="180" cy="197" r="5" class="device-light" />
					</svg>
					<div class="core-stat-row">
						<div>
							<span class="core-stat-value">{fmt(totalArtifacts)}</span>
							<span class="core-stat-label">artifacts</span>
						</div>
						<div>
							<span class="core-stat-value">{fmt(graphCount)}</span>
							<span class="core-stat-label">graph facts</span>
						</div>
					</div>
				</div>

				<form class="atlas-search" onsubmit={runSearch} role="search">
					<SearchIcon size={18} strokeWidth={2} />
					<input
						type="search"
						placeholder="Search documents, artifacts, and relations"
						aria-label="Search documents, artifacts, and relations"
						bind:value={query}
						autocomplete="off"
					/>
					<label class="language">
						<LanguagesIcon size={16} strokeWidth={2} aria-hidden="true" />
						<select value={currentLocale} onchange={changeLocale} aria-label="Select language">
							{#each locales as code}
								<option value={code}>{LOCALE_LABELS[code] ?? code}</option>
							{/each}
						</select>
					</label>
					<button type="submit">Search</button>
				</form>
			</div>

			<div class="artifact-column right">
				{#each RIGHT_ITEMS as item (item.n)}
					{@render atlasCard(item)}
				{/each}
			</div>
		</div>

		<div class="atlas-status" aria-live="polite">
			<div>
				<span class="status-label">Overview feed</span>
				<span>{loading ? 'Loading corpus counters' : 'Live corpus counters loaded'}</span>
			</div>
			<button type="button" onclick={load} disabled={loading}>
				<RefreshCwIcon size={15} strokeWidth={2} />
				Refresh
			</button>
		</div>

		{#if loadError}
			<div class="load-error">
				<span>{loadError}</span>
				<button type="button" onclick={load}>Retry</button>
			</div>
		{/if}
	</section>

	<section class="wiki-lower" aria-label="Wiki browse and activity">
		<div class="portal-panel">
			<header>
				<p>Browse</p>
				<h2>Artifact doors</h2>
			</header>
			<div class="portal-grid">
				{#each PORTALS as portal (portal.label)}
					<a class="portal-card tone-{portal.tone}" href={portal.href}>
						<span>
							<strong>{portal.label}</strong>
							<small>{portal.blurb}</small>
						</span>
						<ArrowUpRightIcon size={17} strokeWidth={2} />
					</a>
				{/each}
			</div>
		</div>

		<div class="activity-panel">
			<header>
				<p>Recent</p>
				<h2>Corpus movement</h2>
			</header>
			<div class="activity-grid">
				{@render recentDocs('Recent Adds', overview?.recent_adds ?? [], 'added')}
				{@render recentDocs('Recent Edits', overview?.recent_edits ?? [], 'edited')}
				{@render processedFeed()}
				{@render errorsFeed()}
			</div>
		</div>
	</section>
</div>

{#snippet atlasCard(item: AtlasItem)}
	{@const Icon = item.icon}
	<a class="atlas-card tone-{item.tone}" href={item.href} style="opacity:var(--load-opacity);">
		<span class="atlas-number">{item.n}</span>
		<span class="atlas-icon"><Icon size={20} strokeWidth={1.8} /></span>
		<span class="atlas-copy">
			<span class="atlas-label">{item.label}</span>
			<span class="atlas-value">{fmt(valueFor(item))}</span>
		</span>
	</a>
{/snippet}

{#snippet recentDocs(title: string, docs: NonNullable<WikiOverviewResponse['recent_adds']>, emptyLabel: string)}
	<div class="feed-card">
		<h3>{title}</h3>
		{#if loading}
			{@render skeleton()}
		{:else if docs.length === 0}
			<p class="empty">No documents {emptyLabel} yet.</p>
		{:else}
			<ul>
				{#each docs.slice(0, 4) as doc (doc.id)}
					<li>
						<a href={`/home3/knowledge?section=kb-input-details&record_id=${doc.id}`}>{doc.title}</a>
						<time title={absTime(doc.time)}>{relTime(doc.time)}</time>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{/snippet}

{#snippet processedFeed()}
	<div class="feed-card">
		<h3>Recent Processed</h3>
		{#if loading}
			{@render skeleton()}
		{:else if (overview?.recent_processed.length ?? 0) === 0}
			<p class="empty">No pipeline runs yet.</p>
		{:else}
			<ul>
				{#each overview!.recent_processed.slice(0, 4) as item, i (i)}
					<li>
						{#if item.record_id != null}
							<a href={`/home3/knowledge?section=kb-input-details&record_id=${item.record_id}`}
								>{item.title}</a
							>
						{:else}
							<span>{item.title}</span>
						{/if}
						<time title={absTime(item.time)}>{relTime(item.time)}</time>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{/snippet}

{#snippet errorsFeed()}
	<div class="feed-card error-feed">
		<h3>Errors</h3>
		{#if loading}
			{@render skeleton()}
		{:else if (overview?.errors.length ?? 0) === 0}
			<p class="empty">No processing errors.</p>
		{:else}
			<ul>
				{#each overview!.errors.slice(0, 4) as item, i (i)}
					<li>
						{#if item.record_id != null}
							<a href={`/home3/knowledge?section=kb-input-details&record_id=${item.record_id}`}
								>{item.title}</a
							>
						{:else}
							<span>{item.title}</span>
						{/if}
						<time title={absTime(item.time)}>{relTime(item.time)}</time>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{/snippet}

{#snippet skeleton()}
	<div class="skeleton">
		{#each Array(4) as _, i (i)}
			<span style={`width:${82 - i * 10}%`}></span>
		{/each}
	</div>
{/snippet}

<style>
	.llm-wiki-v2 {
		--bg: #10100e;
		--panel: #181814;
		--panel-2: #202018;
		--ink: #f2eedf;
		--ink-soft: #c7c0a9;
		--ink-muted: #8d8778;
		--line: rgba(242, 238, 223, 0.14);
		--line-strong: rgba(242, 238, 223, 0.28);
		--paper: #ece4cd;
		--paper-ink: #29251d;
		--mint: #71d6a6;
		--cyan: #6ec7d9;
		--amber: #e5b55c;
		--coral: #e17864;
		--shadow: rgba(0, 0, 0, 0.3);
		--atlas-grid: rgba(242, 238, 223, 0.08);
		height: 100%;
		overflow: auto;
		background:
			linear-gradient(90deg, rgba(242, 238, 223, 0.04) 1px, transparent 1px),
			linear-gradient(0deg, rgba(242, 238, 223, 0.035) 1px, transparent 1px), var(--bg);
		background-size:
			32px 32px,
			32px 32px,
			auto;
		color: var(--ink);
		font-family:
			'IBM Plex Sans',
			'Gill Sans',
			'Avenir Next',
			system-ui,
			sans-serif;
		padding: 22px;
		box-sizing: border-box;
	}

	.llm-wiki-v2.light {
		--bg: #f4f0e5;
		--panel: #fffaf0;
		--panel-2: #eee5d2;
		--ink: #25221a;
		--ink-soft: #595141;
		--ink-muted: #827867;
		--line: rgba(37, 34, 26, 0.14);
		--line-strong: rgba(37, 34, 26, 0.24);
		--paper: #29251d;
		--paper-ink: #f6eedc;
		--shadow: rgba(83, 68, 38, 0.16);
		--atlas-grid: rgba(37, 34, 26, 0.08);
	}

	.atlas-panel,
	.portal-panel,
	.activity-panel {
		border: 1px solid var(--line);
		background:
			linear-gradient(135deg, rgba(255, 255, 255, 0.045), transparent 28%),
			linear-gradient(180deg, var(--panel), var(--panel-2));
		box-shadow: 0 18px 44px var(--shadow);
	}

	.atlas-panel {
		position: relative;
		overflow: hidden;
		border-radius: 8px;
		padding: 18px;
	}

	.atlas-panel::before {
		content: '';
		position: absolute;
		inset: 0;
		background:
			linear-gradient(90deg, transparent 0 48%, var(--atlas-grid) 49% 51%, transparent 52%),
			linear-gradient(0deg, transparent 0 48%, var(--atlas-grid) 49% 51%, transparent 52%);
		background-size: 96px 96px;
		mask-image: linear-gradient(180deg, black, transparent 72%);
		pointer-events: none;
	}

	.atlas-kicker {
		position: relative;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		border-bottom: 1px solid var(--line);
		padding-bottom: 12px;
		color: var(--ink-muted);
		font-size: 0.78rem;
		font-weight: 800;
		letter-spacing: 0.16em;
		text-transform: uppercase;
	}

	.atlas-layout {
		position: relative;
		z-index: 1;
		display: grid;
		grid-template-columns: minmax(220px, 0.9fr) minmax(360px, 1.28fr) minmax(220px, 0.9fr);
		gap: 18px;
		align-items: stretch;
		padding-top: 18px;
	}

	.artifact-column {
		display: grid;
		align-content: center;
		gap: 10px;
	}

	.atlas-card {
		position: relative;
		display: grid;
		grid-template-columns: 34px 42px minmax(0, 1fr);
		align-items: center;
		gap: 10px;
		min-height: 72px;
		padding: 10px;
		border: 1px solid var(--line);
		border-radius: 8px;
		background: rgba(255, 255, 255, 0.035);
		color: var(--ink);
		text-decoration: none;
		transition:
			transform 180ms ease,
			border-color 180ms ease,
			background-color 180ms ease;
	}

	.atlas-card::after {
		content: '';
		position: absolute;
		top: 50%;
		width: 18px;
		border-top: 1px dashed color-mix(in srgb, var(--tone) 72%, transparent);
	}

	.left .atlas-card::after {
		right: -19px;
	}

	.right .atlas-card::after {
		left: -19px;
	}

	.atlas-card:hover {
		transform: translateY(-2px);
		border-color: color-mix(in srgb, var(--tone) 62%, var(--line));
		background: color-mix(in srgb, var(--tone) 10%, transparent);
	}

	.atlas-card:focus-visible,
	.portal-card:focus-visible,
	.atlas-search button:focus-visible,
	.atlas-status button:focus-visible {
		outline: 2px solid var(--tone, var(--cyan));
		outline-offset: 2px;
	}

	.tone-paper {
		--tone: var(--paper);
	}

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

	.atlas-number {
		display: grid;
		place-items: center;
		width: 30px;
		height: 30px;
		border: 1px solid color-mix(in srgb, var(--tone) 58%, var(--line));
		border-radius: 50%;
		color: var(--tone);
		font-family: Georgia, 'Iowan Old Style', serif;
		font-size: 1rem;
		font-weight: 700;
	}

	.atlas-icon {
		display: grid;
		place-items: center;
		width: 40px;
		height: 40px;
		border-radius: 8px;
		background: color-mix(in srgb, var(--tone) 14%, transparent);
		color: var(--tone);
	}

	.atlas-copy {
		display: grid;
		gap: 3px;
		min-width: 0;
	}

	.atlas-label {
		overflow: hidden;
		color: var(--ink);
		font-size: 0.9rem;
		font-weight: 800;
		line-height: 1.12;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.atlas-value {
		color: var(--tone);
		font-family: Georgia, 'Iowan Old Style', serif;
		font-size: 1.25rem;
		line-height: 1;
		font-variant-numeric: tabular-nums;
	}

	.atlas-core {
		display: grid;
		align-content: center;
		gap: 12px;
		min-height: 530px;
	}

	.core-label {
		justify-self: center;
		border: 1px solid var(--line);
		border-radius: 999px;
		padding: 7px 14px;
		background: rgba(255, 255, 255, 0.04);
		color: var(--ink-soft);
		font-size: 0.76rem;
		font-weight: 900;
		letter-spacing: 0.18em;
		text-transform: uppercase;
	}

	.core-device {
		position: relative;
		display: grid;
		place-items: center;
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 12px;
		background:
			linear-gradient(135deg, rgba(110, 199, 217, 0.1), transparent 34%),
			linear-gradient(225deg, rgba(229, 181, 92, 0.09), transparent 30%),
			rgba(255, 255, 255, 0.03);
	}

	.core-svg {
		width: min(100%, 420px);
		height: auto;
		overflow: visible;
	}

	#llmWikiGrid path {
		fill: none;
		stroke: currentColor;
		stroke-width: 1;
	}

	.device-shell {
		fill: var(--paper);
	}

	.device-screen {
		fill: color-mix(in srgb, var(--bg) 84%, #0b1715);
	}

	.device-grid {
		fill: url('#llmWikiGrid');
		color: color-mix(in srgb, var(--cyan) 28%, transparent);
	}

	.link-line {
		fill: none;
		stroke-width: 3;
		stroke-linecap: round;
		stroke-dasharray: 8 10;
	}

	.link-a {
		stroke: var(--cyan);
	}

	.link-b {
		stroke: var(--amber);
	}

	.doc-stack rect {
		fill: color-mix(in srgb, var(--paper) 86%, var(--cyan));
		stroke: var(--paper-ink);
		stroke-width: 3;
	}

	.doc-stack path {
		fill: none;
		stroke: var(--paper-ink);
		stroke-width: 3;
		stroke-linecap: round;
		opacity: 0.68;
	}

	.db-core ellipse,
	.db-core path {
		fill: color-mix(in srgb, var(--mint) 36%, var(--paper));
		stroke: var(--paper-ink);
		stroke-width: 3;
	}

	.graph-nodes circle {
		fill: color-mix(in srgb, var(--coral) 70%, var(--paper));
		stroke: var(--paper-ink);
		stroke-width: 3;
	}

	.graph-nodes path {
		fill: none;
		stroke: var(--paper-ink);
		stroke-width: 3;
		stroke-linecap: round;
	}

	.device-base,
	.device-foot {
		fill: color-mix(in srgb, var(--paper) 84%, #000);
	}

	.device-light {
		fill: var(--mint);
	}

	.core-stat-row {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 8px;
		width: 100%;
	}

	.core-stat-row div {
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 9px 10px;
		background: rgba(0, 0, 0, 0.12);
	}

	.core-stat-value,
	.core-stat-label {
		display: block;
	}

	.core-stat-value {
		color: var(--paper);
		font-family: Georgia, 'Iowan Old Style', serif;
		font-size: 1.45rem;
		line-height: 1;
		font-variant-numeric: tabular-nums;
	}

	.core-stat-label {
		margin-top: 3px;
		color: var(--ink-muted);
		font-size: 0.72rem;
		font-weight: 800;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.atlas-search {
		display: grid;
		grid-template-columns: 24px minmax(0, 1fr) auto auto;
		align-items: center;
		gap: 8px;
		border: 1px solid var(--line-strong);
		border-radius: 8px;
		padding: 8px;
		background: color-mix(in srgb, var(--panel) 78%, #000);
		color: var(--ink-muted);
	}

	.atlas-search:focus-within {
		border-color: var(--cyan);
		box-shadow: 0 0 0 3px rgba(110, 199, 217, 0.14);
	}

	.atlas-search input,
	.language select {
		border: 0;
		background: transparent;
		color: var(--ink);
		font: inherit;
	}

	.atlas-search input {
		min-width: 0;
		padding: 7px 0;
		font-size: 0.9rem;
	}

	.atlas-search input:focus,
	.language select:focus {
		outline: none;
	}

	.language {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		border-left: 1px solid var(--line);
		padding-left: 10px;
	}

	.language select {
		cursor: pointer;
		font-size: 0.82rem;
	}

	.atlas-search button,
	.atlas-status button,
	.load-error button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		border: 0;
		border-radius: 7px;
		background: var(--paper);
		color: var(--paper-ink);
		font-size: 0.84rem;
		font-weight: 900;
		cursor: pointer;
	}

	.atlas-search button {
		padding: 9px 14px;
	}

	.atlas-status {
		position: relative;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		margin-top: 14px;
		border-top: 1px solid var(--line);
		padding-top: 12px;
		color: var(--ink-muted);
		font-size: 0.82rem;
	}

	.atlas-status div {
		display: flex;
		align-items: center;
		gap: 10px;
		min-width: 0;
	}

	.status-label {
		color: var(--mint);
		font-weight: 900;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.atlas-status button {
		flex-shrink: 0;
		padding: 8px 11px;
		background: transparent;
		border: 1px solid var(--line-strong);
		color: var(--ink-soft);
	}

	.atlas-status button:disabled {
		cursor: wait;
		opacity: 0.55;
	}

	.load-error {
		position: relative;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		margin-top: 12px;
		border: 1px solid color-mix(in srgb, var(--coral) 42%, var(--line));
		border-radius: 8px;
		padding: 10px 12px;
		background: color-mix(in srgb, var(--coral) 10%, transparent);
		color: var(--coral);
		font-size: 0.84rem;
	}

	.load-error button {
		padding: 7px 10px;
		background: var(--coral);
		color: #170f0d;
	}

	.wiki-lower {
		display: grid;
		grid-template-columns: minmax(280px, 0.72fr) minmax(0, 1.28fr);
		gap: 16px;
		margin-top: 16px;
	}

	.portal-panel,
	.activity-panel {
		border-radius: 8px;
		padding: 16px;
	}

	.portal-panel header,
	.activity-panel header {
		margin-bottom: 12px;
		border-bottom: 1px solid var(--line);
		padding-bottom: 10px;
	}

	.portal-panel p,
	.activity-panel p {
		margin: 0 0 3px;
		color: var(--ink-muted);
		font-size: 0.72rem;
		font-weight: 900;
		letter-spacing: 0.14em;
		text-transform: uppercase;
	}

	.portal-panel h2,
	.activity-panel h2 {
		margin: 0;
		color: var(--ink);
		font-family: Georgia, 'Iowan Old Style', serif;
		font-size: 1.24rem;
		font-weight: 500;
	}

	.portal-grid {
		display: grid;
		gap: 8px;
	}

	.portal-card {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 10px;
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 10px;
		background: rgba(255, 255, 255, 0.025);
		color: var(--ink);
		text-decoration: none;
		transition:
			border-color 160ms ease,
			background-color 160ms ease,
			transform 160ms ease;
	}

	.portal-card:hover {
		transform: translateX(3px);
		border-color: color-mix(in srgb, var(--tone) 54%, var(--line));
		background: color-mix(in srgb, var(--tone) 8%, transparent);
	}

	.portal-card strong,
	.portal-card small {
		display: block;
	}

	.portal-card strong {
		font-size: 0.88rem;
		line-height: 1.2;
	}

	.portal-card small {
		margin-top: 2px;
		color: var(--ink-muted);
		font-size: 0.75rem;
	}

	.portal-card svg {
		flex-shrink: 0;
		color: var(--tone);
	}

	.activity-grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 10px;
	}

	.feed-card {
		min-width: 0;
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 10px;
		background: rgba(255, 255, 255, 0.026);
	}

	.feed-card h3 {
		margin: 0 0 8px;
		color: var(--ink);
		font-size: 0.86rem;
		font-weight: 900;
	}

	.error-feed h3 {
		color: var(--coral);
	}

	.feed-card ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.feed-card li {
		display: flex;
		align-items: baseline;
		gap: 8px;
		border-top: 1px solid var(--line);
		padding: 7px 0 0;
		margin-top: 7px;
		font-size: 0.8rem;
	}

	.feed-card a,
	.feed-card li span {
		min-width: 0;
		flex: 1;
		overflow: hidden;
		color: var(--ink-soft);
		text-decoration: none;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.feed-card a:hover {
		color: var(--cyan);
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.feed-card time {
		flex-shrink: 0;
		color: var(--ink-muted);
		font-size: 0.7rem;
		font-variant-numeric: tabular-nums;
	}

	.empty {
		color: var(--ink-muted);
		font-size: 0.8rem;
		line-height: 1.45;
	}

	.skeleton {
		display: grid;
		gap: 8px;
	}

	.skeleton span {
		display: block;
		height: 10px;
		border-radius: 999px;
		background: linear-gradient(90deg, var(--line), var(--line-strong), var(--line));
		background-size: 180% 100%;
		animation: llm-wiki-v2-shimmer 1.4s ease-in-out infinite;
	}

	@keyframes llm-wiki-v2-shimmer {
		0% {
			background-position: 180% 0;
		}
		100% {
			background-position: -180% 0;
		}
	}

	@media (max-width: 1180px) {
		.atlas-layout {
			grid-template-columns: 1fr;
		}

		.artifact-column {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}

		.atlas-core {
			min-height: auto;
			order: -1;
		}

		.atlas-card::after {
			display: none;
		}

		.wiki-lower {
			grid-template-columns: 1fr;
		}
	}

	@media (max-width: 920px) {
		.activity-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}

	@media (max-width: 640px) {
		.llm-wiki-v2 {
			padding: 12px;
		}

		.atlas-panel,
		.portal-panel,
		.activity-panel {
			padding: 12px;
		}

		.atlas-kicker,
		.atlas-status {
			align-items: flex-start;
			flex-direction: column;
		}

		.artifact-column,
		.activity-grid {
			grid-template-columns: 1fr;
		}

		.atlas-card {
			grid-template-columns: 32px 38px minmax(0, 1fr);
		}

		.atlas-search {
			grid-template-columns: 24px minmax(0, 1fr);
		}

		.language {
			border-left: 0;
			border-top: 1px solid var(--line);
			padding: 8px 0 0;
		}

		.atlas-search button {
			padding-block: 10px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.atlas-card,
		.portal-card {
			transition: none;
		}

		.skeleton span {
			animation: none;
		}
	}
</style>
