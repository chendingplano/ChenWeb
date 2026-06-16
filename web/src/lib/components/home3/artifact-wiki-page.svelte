<script lang="ts">
	import {
		getArtifactWiki,
		getArtifactWikiRecord,
		type ArtifactWikiResponse
	} from '$lib/services/artifactWikiService';
	import ArtifactRecordInspector from './artifact-record-inspector.svelte';
	import ArtifactWikiArticleView from './artifact-wiki-article-view.svelte';

	let {
		artifactType,
		artifactId,
		lang = 'en',
		darkMode = true
	}: {
		artifactType: string;
		artifactId: string;
		lang?: string;
		darkMode?: boolean;
	} = $props();

	let articlePayload = $state<ArtifactWikiResponse | null>(null);
	let recordPayload = $state<ArtifactWikiResponse | null>(null);
	let articleLoading = $state(false);
	let recordLoading = $state(false);
	let articleError = $state('');
	let recordError = $state('');
	let lastKey = $state('');
	let pageBg = $derived(darkMode ? 'oklch(16% 0.013 250)' : 'oklch(96% 0.012 84)');
	let surface = $derived(darkMode ? 'oklch(20% 0.018 250)' : 'oklch(99% 0.006 84)');
	let border = $derived(darkMode ? 'oklch(30% 0.02 250)' : 'oklch(88% 0.012 84)');
	let ink = $derived(darkMode ? 'oklch(92% 0.01 250)' : 'oklch(28% 0.02 250)');
	let inkSoft = $derived(darkMode ? 'oklch(72% 0.02 250)' : 'oklch(46% 0.02 250)');

	async function loadRecord() {
		recordLoading = true;
		recordError = '';
		try {
			recordPayload = await getArtifactWikiRecord(artifactType, artifactId, lang);
		} catch (error) {
			recordError = error instanceof Error ? error.message : String(error);
		} finally {
			recordLoading = false;
		}
	}

	async function loadArticle() {
		articleLoading = true;
		articleError = '';
		try {
			articlePayload = await getArtifactWiki(artifactType, artifactId, lang);
		} catch (error) {
			articleError = error instanceof Error ? error.message : String(error);
		} finally {
			articleLoading = false;
		}
	}

	$effect(() => {
		const key = `${artifactType}:${artifactId}:${lang}`;
		if (!artifactType || !artifactId || key === lastKey) return;
		lastKey = key;
		recordPayload = null;
		articlePayload = null;
		void loadRecord();
		void loadArticle();
	});
</script>

<div class="artifact-page" style="--bg:{pageBg}; --surface:{surface}; --border:{border}; --ink:{ink}; --ink-soft:{inkSoft};">
	<div class="layout">
		<article class="article">
			<ArtifactWikiArticleView
				article={(articlePayload?.article ?? null) as Record<string, unknown> | null}
				{artifactType}
				artifactId={artifactId}
				{darkMode}
				{lang}
				showInfobox={false}
				loading={articleLoading}
				error={articleError}
				generated={articlePayload?.fresh ?? false}
				retry={loadArticle}
			/>
		</article>

		<div class="sidebar">
			{#if recordError}
				<p class="state error">{recordError}</p>
			{:else}
				<ArtifactRecordInspector {artifactType} record={recordPayload?.record ?? null} {darkMode} {lang} />
				{#if recordLoading && !recordPayload}
					<p class="sidebar-note">{lang === 'zh-cn' ? '正在获取有据可依的记录…' : 'Fetching grounded record…'}</p>
				{/if}
			{/if}
		</div>
	</div>
</div>

<style>
	.artifact-page {
		height: 100%;
		overflow-y: auto;
		background: var(--bg);
		color: var(--ink);
	}
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 360px;
		gap: 1.5rem;
		max-width: 1280px;
		margin: 0 auto;
		padding: 1.5rem;
	}
	.article {
		min-width: 0;
		background: var(--surface);
		border: 1px solid var(--border);
		border-radius: 18px;
		padding: 0;
		overflow: hidden;
	}
	.sidebar {
		align-self: start;
		position: sticky;
		top: 1rem;
	}
	.sidebar-note {
		margin: 0.75rem 0 0;
		color: var(--ink-soft);
		font-size: 0.88rem;
	}
	@media (max-width: 980px) {
		.layout {
			grid-template-columns: 1fr;
		}
		.sidebar {
			position: static;
		}
	}
</style>
