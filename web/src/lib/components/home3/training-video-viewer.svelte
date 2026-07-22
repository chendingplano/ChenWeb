<script lang="ts">
	import { onMount } from 'svelte';
	import { listVideos, videoStreamUrl, type VideoMeta } from '$lib/services/videoService';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// Tokens aligned with the dashboard app shell (product consistency), tinted
	// toward the indigo accent so neutrals aren't pure grey. Both themes ship.
	const t = $derived(
		darkMode
			? {
					pageText: '#E2E8F0',
					subText: '#93A0B4',
					cardBg: '#1E2333',
					cardBorder: '#2A3042',
					cardHoverBorder: '#3A4256',
					coverBg: '#12151E',
					chipBg: 'rgba(12,15,24,0.72)',
					chipText: '#E6E9F0',
					accent: '#818CF8',
					accentSoft: 'rgba(129,140,248,0.16)',
					scrim: 'linear-gradient(to top, rgba(10,12,20,0.85), rgba(10,12,20,0) 60%)',
					shadow: '0 10px 30px -12px rgba(0,0,0,0.65)'
				}
			: {
					pageText: '#1A2030',
					subText: '#5B6472',
					cardBg: '#FFFFFF',
					cardBorder: '#E6E8EE',
					cardHoverBorder: '#C7CBD8',
					coverBg: '#EEF0F5',
					chipBg: 'rgba(20,24,34,0.66)',
					chipText: '#F5F6FA',
					accent: '#5B57E6',
					accentSoft: 'rgba(91,87,230,0.10)',
					scrim: 'linear-gradient(to top, rgba(10,12,20,0.78), rgba(10,12,20,0) 60%)',
					shadow: '0 12px 28px -14px rgba(30,34,60,0.35)'
				}
	);

	let videos = $state<VideoMeta[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let playing = $state<VideoMeta | null>(null);

	function formatBytes(n: number): string {
		if (!n || n < 1024) return `${n || 0} B`;
		const units = ['KB', 'MB', 'GB', 'TB'];
		let value = n / 1024;
		let i = 0;
		while (value >= 1024 && i < units.length - 1) {
			value /= 1024;
			i++;
		}
		return `${value.toFixed(1)} ${units[i]}`;
	}

	function formatDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	function open(video: VideoMeta) {
		playing = video;
	}
	function close() {
		playing = null;
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape') close();
	}

	async function load() {
		loading = true;
		error = null;
		try {
			videos = await listVideos();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load videos';
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:window onkeydown={onKey} />

<div class="tvv" style="--text:{t.pageText}; --sub:{t.subText}; --card:{t.cardBg}; --border:{t.cardBorder}; --border-h:{t.cardHoverBorder}; --cover:{t.coverBg}; --chip-bg:{t.chipBg}; --chip-text:{t.chipText}; --accent:{t.accent}; --accent-soft:{t.accentSoft}; --scrim:{t.scrim}; --shadow:{t.shadow};">
	<header class="head">
		<h1>Training</h1>
		<p>A library of training videos. Select one to watch.</p>
	</header>

	{#if loading}
		<div class="grid" aria-hidden="true">
			{#each Array(6) as _, i (i)}
				<div class="skeleton">
					<div class="sk-cover"></div>
					<div class="sk-line w70"></div>
					<div class="sk-line w90"></div>
					<div class="sk-line w40"></div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div class="state">
			<p class="state-title">Couldn't load videos</p>
			<p class="state-sub">{error}</p>
			<button class="retry" onclick={load}>Try again</button>
		</div>
	{:else if videos.length === 0}
		<div class="state">
			<div class="state-mark" aria-hidden="true">
				<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
					<rect x="2" y="4" width="20" height="16" rx="2.5" />
					<path d="M10 9.5v5l4-2.5z" fill="currentColor" stroke="none" />
				</svg>
			</div>
			<p class="state-title">No training videos yet</p>
			<p class="state-sub">Once videos are published in the admin area, they appear here as a gallery you can browse and play.</p>
		</div>
	{:else}
		<div class="grid">
			{#each videos as video (video.id)}
				<button class="card" onclick={() => open(video)} aria-label={`Play ${video.name}`}>
					<div class="cover">
						{#if video.image_url}
							<img src={video.image_url} alt="" loading="lazy" />
						{:else}
							<div class="cover-fallback" aria-hidden="true">
								<svg viewBox="0 0 24 24" width="30" height="30" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">
									<rect x="2" y="4" width="20" height="16" rx="2.5" />
									<path d="M10 9.5v5l4-2.5z" fill="currentColor" stroke="none" />
								</svg>
							</div>
						{/if}
						<span class="scrim" aria-hidden="true"></span>
						<span class="play" aria-hidden="true">
							<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor"><path d="M8 5.5v13l11-6.5z" /></svg>
						</span>
						<span class="size-chip">{formatBytes(video.size_bytes)}</span>
					</div>
					<div class="meta">
						<h2 class="title" title={video.name}>{video.name}</h2>
						{#if video.description}
							<p class="desc">{video.description}</p>
						{/if}
						<div class="foot">
							<span class="tag">{video.source}</span>
							<span class="dot">·</span>
							<time>{formatDate(video.created_at)}</time>
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</div>

{#if playing}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="lightbox" onclick={close}>
		<div class="player" onclick={(e) => e.stopPropagation()}>
			<div class="player-bar">
				<span class="player-title">{playing.name}</span>
				<button class="player-close" onclick={close} aria-label="Close player">✕</button>
			</div>
			<!-- svelte-ignore a11y_media_has_caption -->
			<video src={videoStreamUrl(playing.id)} controls autoplay></video>
			{#if playing.description}
				<p class="player-desc">{playing.description}</p>
			{/if}
		</div>
	</div>
{/if}

<style>
	.tvv {
		padding: 28px 32px 40px;
		color: var(--text);
		font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
	}
	.head {
		margin-bottom: 22px;
	}
	.head h1 {
		font-size: 22px;
		font-weight: 650;
		letter-spacing: -0.01em;
		margin: 0 0 4px;
	}
	.head p {
		font-size: 14px;
		color: var(--sub);
		margin: 0;
		line-height: 1.7;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(272px, 1fr));
		gap: 20px;
	}

	/* Card */
	.card {
		display: flex;
		flex-direction: column;
		text-align: left;
		padding: 0;
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		overflow: hidden;
		cursor: pointer;
		color: inherit;
		transition:
			transform 180ms cubic-bezier(0.22, 1, 0.36, 1),
			border-color 180ms ease,
			box-shadow 180ms ease;
	}
	.card:hover {
		transform: translateY(-3px);
		border-color: var(--border-h);
		box-shadow: var(--shadow);
	}
	.card:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	.cover {
		position: relative;
		aspect-ratio: 16 / 9;
		background: var(--cover);
		overflow: hidden;
	}
	.cover img {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
		transition: transform 260ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.card:hover .cover img {
		transform: scale(1.04);
	}
	.cover-fallback {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--sub);
		opacity: 0.5;
	}
	.scrim {
		position: absolute;
		inset: 0;
		background: var(--scrim);
		opacity: 0;
		transition: opacity 180ms ease;
		pointer-events: none;
	}
	.card:hover .scrim {
		opacity: 1;
	}
	.play {
		position: absolute;
		left: 50%;
		top: 50%;
		width: 46px;
		height: 46px;
		border-radius: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		color: #12151e;
		background: rgba(255, 255, 255, 0.94);
		transform: translate(-50%, -50%) scale(0.82);
		opacity: 0;
		transition:
			transform 200ms cubic-bezier(0.22, 1, 0.36, 1),
			opacity 160ms ease;
		pointer-events: none;
	}
	.play svg {
		margin-left: 2px;
	}
	.card:hover .play {
		opacity: 1;
		transform: translate(-50%, -50%) scale(1);
	}
	.size-chip {
		position: absolute;
		right: 8px;
		bottom: 8px;
		padding: 2px 7px;
		border-radius: 6px;
		font-size: 11px;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
		color: var(--chip-text);
		background: var(--chip-bg);
		backdrop-filter: blur(2px);
	}

	.meta {
		padding: 12px 14px 14px;
		display: flex;
		flex-direction: column;
		gap: 5px;
	}
	.title {
		font-size: 14.5px;
		font-weight: 600;
		margin: 0;
		line-height: 1.35;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.desc {
		font-size: 12.5px;
		color: var(--sub);
		margin: 0;
		line-height: 1.6;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 7px;
		margin-top: 3px;
		font-size: 11.5px;
		color: var(--sub);
	}
	.tag {
		padding: 1.5px 8px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 500;
		color: var(--accent);
		background: var(--accent-soft);
	}
	.foot .dot {
		opacity: 0.6;
	}
	.foot time {
		font-variant-numeric: tabular-nums;
	}

	/* States */
	.state {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
		padding: 72px 24px;
		gap: 8px;
	}
	.state-mark {
		width: 56px;
		height: 56px;
		border-radius: 16px;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--accent);
		background: var(--accent-soft);
		margin-bottom: 6px;
	}
	.state-title {
		font-size: 15px;
		font-weight: 600;
		margin: 0;
	}
	.state-sub {
		font-size: 13px;
		color: var(--sub);
		margin: 0;
		max-width: 42ch;
		line-height: 1.7;
	}
	.retry {
		margin-top: 12px;
		padding: 8px 16px;
		border-radius: 8px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text);
		font-size: 13px;
		cursor: pointer;
		transition: border-color 150ms ease;
	}
	.retry:hover {
		border-color: var(--accent);
	}

	/* Skeleton */
	.skeleton {
		border: 1px solid var(--border);
		border-radius: 14px;
		overflow: hidden;
		background: var(--card);
	}
	.sk-cover {
		aspect-ratio: 16 / 9;
		background: var(--cover);
	}
	.sk-line {
		height: 10px;
		border-radius: 6px;
		background: var(--cover);
		margin: 12px 14px 0;
	}
	.sk-line.w70 {
		width: 70%;
	}
	.sk-line.w90 {
		width: 90%;
	}
	.sk-line.w40 {
		width: 40%;
		margin-bottom: 16px;
	}
	.skeleton .sk-cover,
	.skeleton .sk-line {
		animation: pulse 1.4s ease-in-out infinite;
	}
	@keyframes pulse {
		0%, 100% {
			opacity: 1;
		}
		50% {
			opacity: 0.55;
		}
	}

	/* Lightbox player */
	.lightbox {
		position: fixed;
		inset: 0;
		z-index: 60;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 24px;
		background: rgba(6, 8, 14, 0.78);
		animation: fade 160ms ease;
	}
	.player {
		width: 100%;
		max-width: 960px;
		background: var(--card);
		border: 1px solid var(--border);
		border-radius: 14px;
		overflow: hidden;
		box-shadow: var(--shadow);
	}
	.player-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 16px;
		border-bottom: 1px solid var(--border);
	}
	.player-title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}
	.player-close {
		background: none;
		border: none;
		color: var(--sub);
		font-size: 15px;
		cursor: pointer;
		line-height: 1;
	}
	.player-close:hover {
		color: var(--text);
	}
	.player video {
		width: 100%;
		max-height: 68vh;
		background: #000;
		display: block;
	}
	.player-desc {
		font-size: 13px;
		color: var(--sub);
		line-height: 1.7;
		margin: 0;
		padding: 12px 16px 16px;
	}
	@keyframes fade {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.card,
		.cover img,
		.scrim,
		.play,
		.skeleton .sk-cover,
		.skeleton .sk-line,
		.lightbox {
			transition: none;
			animation: none;
		}
		.card:hover {
			transform: none;
		}
		.card:hover .cover img {
			transform: none;
		}
		.card:hover .play {
			transform: translate(-50%, -50%);
		}
	}
</style>
