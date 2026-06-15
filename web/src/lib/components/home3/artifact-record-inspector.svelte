<script lang="ts">
	import { buildArtifactRecordGroups } from './artifact-record-groups';
	import { metricWikiCopyForLang } from './metric-wiki-i18n';

	let {
		artifactType,
		record,
		darkMode = true,
		lang = 'en'
	}: {
		artifactType: string;
		record: Record<string, unknown> | null;
		darkMode?: boolean;
		lang?: string;
	} = $props();

	let groups = $derived(record ? buildArtifactRecordGroups(artifactType, record, lang) : []);
	let copy = $derived(metricWikiCopyForLang(lang));
	let panelBg = $derived(darkMode ? 'oklch(20% 0.018 250)' : 'oklch(99% 0.006 84)');
	let border = $derived(darkMode ? 'oklch(30% 0.02 250)' : 'oklch(88% 0.012 84)');
	let ink = $derived(darkMode ? 'oklch(92% 0.01 250)' : 'oklch(28% 0.02 250)');
	let inkSoft = $derived(darkMode ? 'oklch(72% 0.02 250)' : 'oklch(46% 0.02 250)');
</script>

<aside class="inspector" style="--panel-bg:{panelBg}; --border:{border}; --ink:{ink}; --ink-soft:{inkSoft};">
	<h2>{copy.artifactRecord}</h2>
	{#if !record}
		<p class="empty">{copy.loadingGroundedRecord}</p>
	{:else}
		{#each groups as group}
			<section class="group">
				<h3>{group.title}</h3>
				<dl>
					{#each group.rows as row}
						<div class="row">
							<dt>{row.label}</dt>
							<dd><pre>{row.value}</pre></dd>
						</div>
					{/each}
				</dl>
			</section>
		{/each}
	{/if}
</aside>

<style>
	.inspector {
		background: var(--panel-bg);
		border: 1px solid var(--border);
		border-radius: 16px;
		padding: 1rem;
		color: var(--ink);
	}
	h2, h3 {
		margin: 0;
	}
	h2 {
		font-size: 1rem;
		margin-bottom: 0.9rem;
	}
	.group + .group {
		margin-top: 1rem;
	}
	h3 {
		font-size: 0.78rem;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--ink-soft);
		margin-bottom: 0.6rem;
	}
	dl {
		margin: 0;
	}
	.row + .row {
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid var(--border);
	}
	dt {
		font-size: 0.72rem;
		color: var(--ink-soft);
		margin-bottom: 0.25rem;
	}
	dd {
		margin: 0;
	}
	pre {
		margin: 0;
		white-space: pre-wrap;
		word-break: break-word;
		font: inherit;
	}
	.empty {
		margin: 0;
		color: var(--ink-soft);
	}
</style>
