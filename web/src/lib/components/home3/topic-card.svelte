<script lang="ts">
	import type { TopicCard } from './topic-types';

	let {
		topic,
		selected = false,
		onSelect = () => {}
	}: {
		topic: TopicCard;
		selected?: boolean;
		onSelect?: () => void;
	} = $props();
</script>

<button class:selected={selected} class="topic-card" type="button" onclick={onSelect}>
	<div class="keyword-row">
		{#each topic.topicKeywords as keyword}
			<span class="keyword">{keyword}</span>
		{/each}
	</div>
	<p>{topic.topicText}</p>
</button>

<style>
	.topic-card {
		width: 100%;
		border-radius: 16px;
		border: 1px solid rgba(148, 163, 184, 0.18);
		background: rgba(15, 23, 42, 0.38);
		padding: 14px;
		text-align: left;
		color: inherit;
		transition:
			border-color 140ms ease,
			transform 140ms ease,
			background 140ms ease;
	}

	.topic-card:hover,
	.topic-card.selected {
		border-color: rgba(34, 197, 94, 0.42);
		background: rgba(13, 148, 136, 0.12);
		transform: translateY(-1px);
	}

	.keyword-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		margin-bottom: 10px;
	}

	.keyword {
		border-radius: 999px;
		padding: 0.22rem 0.55rem;
		background: rgba(34, 197, 94, 0.14);
		font-size: 0.72rem;
		font-weight: 600;
		color: #4ade80;
	}

	p {
		margin: 0;
		font-size: 0.84rem;
		line-height: 1.55;
		opacity: 0.9;
	}
</style>
