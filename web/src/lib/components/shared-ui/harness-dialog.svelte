<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import WrenchIcon from '@lucide/svelte/icons/wrench';
	import ShieldQuestionIcon from '@lucide/svelte/icons/shield-question';
	import type { HarnessDecision } from '$lib/services/harnessFrontendService';
	import {
		answeredInput,
		questionsOf,
		renderMarkdown,
		toolSummary,
		type HarnessItem
	} from './harness-conversation';

	type Colors = {
		cardBg: string;
		surface2: string;
		borderColor: string;
		accent: string;
		accentTint: string;
		textPrimary: string;
		textSecondary: string;
		textMuted: string;
	};

	let {
		items,
		running = false,
		colors,
		onDecide
	}: {
		items: HarnessItem[];
		running?: boolean;
		colors: Colors;
		onDecide: (requestId: string, decision: HarnessDecision) => void;
	} = $props();

	// Answers picked for each open AskUserQuestion request, by question text.
	let picks = $state<Record<string, Record<string, string[]>>>({});

	function pick(requestId: string, question: string, label: string, multi: boolean) {
		const current = picks[requestId]?.[question] ?? [];
		const next = multi
			? current.includes(label)
				? current.filter((l) => l !== label)
				: [...current, label]
			: [label];
		picks = { ...picks, [requestId]: { ...(picks[requestId] ?? {}), [question]: next } };
	}

	function noticeText(item: Extract<HarnessItem, { kind: 'notice' }>): string {
		switch (item.code) {
			case 'retrying':
				return m.harness_dialog_retrying({ attempt: item.detail ?? '' });
			case 'interrupted':
				return m.harness_dialog_interrupted();
			case 'max_turns':
				return m.harness_dialog_max_turns();
			default:
				return item.detail ? m.harness_dialog_error_detail({ detail: item.detail }) : m.harness_dialog_error();
		}
	}

	const pretty = (value: unknown) => JSON.stringify(value, null, 2);
</script>

<div class="space-y-3 select-text">
	{#each items as item (item.id)}
		{#if item.kind === 'user'}
			<div class="flex justify-end">
				<div
					class="max-w-[78%] rounded-xl px-3 py-2 text-sm whitespace-pre-wrap"
					style="background:{colors.accent}; color:white; border:1px solid {colors.accent};"
				>
					{item.text}
				</div>
			</div>
		{:else if item.kind === 'assistant'}
			<div class="flex justify-start">
				<div
					class="harness-markdown max-w-[88%] rounded-xl px-3 py-2 text-sm"
					style="background:{colors.cardBg}; color:{colors.textPrimary}; border:1px solid {colors.borderColor};"
				>
					<!-- eslint-disable-next-line svelte/no-at-html-tags -- renderMarkdown escapes raw HTML and unsafe links -->
					{@html renderMarkdown(item.text)}
				</div>
			</div>
		{:else if item.kind === 'tool'}
			<details
				class="max-w-[88%] rounded-lg px-3 py-1.5 text-xs"
				style="background:{colors.surface2}; border:1px solid {colors.borderColor}; color:{colors.textSecondary};"
			>
				<summary class="cursor-pointer">
					<WrenchIcon class="mr-1 inline h-3.5 w-3.5" />
					<span class="font-semibold" style="color:{colors.textPrimary};">{item.name}</span>
					<span class="ml-1 font-mono">{toolSummary(item.name, item.input)}</span>
					{#if item.result?.isError}
						<span class="ml-1 font-semibold" style="color:#F87171;">{m.harness_dialog_failed()}</span>
					{:else if !item.result && running}
						<span class="ml-1" style="color:{colors.textMuted};">{m.harness_dialog_working()}</span>
					{/if}
				</summary>
				<div class="mt-2 space-y-2">
					<div>
						<div class="mb-1 font-semibold" style="color:{colors.textMuted};">{m.harness_dialog_input()}</div>
						<pre class="max-h-64 overflow-auto rounded p-2 whitespace-pre-wrap" style="background:{colors.cardBg};">{pretty(item.input)}</pre>
					</div>
					{#if item.result}
						<div>
							<div class="mb-1 font-semibold" style="color:{colors.textMuted};">{m.harness_dialog_result()}</div>
							<pre class="max-h-64 overflow-auto rounded p-2 whitespace-pre-wrap" style="background:{colors.cardBg};">{item.result.text}</pre>
						</div>
					{/if}
				</div>
			</details>
		{:else if item.kind === 'permission'}
			{@const questions = item.toolName === 'AskUserQuestion' ? questionsOf(item.input) : []}
			<div
				class="max-w-[88%] rounded-xl p-3 text-sm"
				style="background:{colors.accentTint}; border:1px solid {colors.accent}; color:{colors.textPrimary};"
			>
				<div class="mb-1 font-semibold">
					<ShieldQuestionIcon class="mr-1 inline h-4 w-4" />
					{item.title ?? (questions.length ? m.harness_dialog_question() : m.harness_dialog_allow_tool({ tool: item.toolName }))}
				</div>
				{#if item.description}
					<div class="mb-2 text-xs" style="color:{colors.textSecondary};">{item.description}</div>
				{/if}
				{#if questions.length}
					{#each questions as question (question.question)}
						<div class="mb-2">
							<div class="mb-1 text-xs font-semibold">{question.question}</div>
							<div class="flex flex-wrap gap-1">
								{#each question.options as option (option.label)}
									{@const chosen = picks[item.requestId]?.[question.question]?.includes(option.label)}
									<button
										class="cursor-pointer rounded-lg px-2 py-1 text-xs disabled:cursor-default"
										style="background:{chosen ? colors.accent : colors.cardBg}; color:{chosen ? 'white' : colors.textPrimary}; border:1px solid {colors.borderColor};"
										title={option.description}
										disabled={item.state !== 'pending'}
										onclick={() => pick(item.requestId, question.question, option.label, Boolean(question.multiSelect))}
									>
										{option.label}
									</button>
								{/each}
							</div>
						</div>
					{/each}
				{:else if (toolSummary(item.toolName, item.input) || pretty(item.input)) !== item.description}
					<pre class="mb-2 max-h-48 overflow-auto rounded p-2 text-xs whitespace-pre-wrap" style="background:{colors.cardBg};">{toolSummary(item.toolName, item.input) || pretty(item.input)}</pre>
				{/if}
				{#if item.state === 'pending'}
					<div class="flex gap-2">
						{#if questions.length}
							<button
								class="cursor-pointer rounded-lg px-3 py-1 text-xs font-semibold disabled:opacity-50"
								style="background:{colors.accent}; color:white;"
								disabled={questions.some((q) => !picks[item.requestId]?.[q.question]?.length)}
								onclick={() => onDecide(item.requestId, { behavior: 'allow', updated_input: answeredInput(item.input, picks[item.requestId] ?? {}) })}
							>
								{m.harness_dialog_submit_answers()}
							</button>
						{:else}
							<button
								class="cursor-pointer rounded-lg px-3 py-1 text-xs font-semibold"
								style="background:{colors.accent}; color:white;"
								onclick={() => onDecide(item.requestId, { behavior: 'allow' })}
							>
								{m.harness_dialog_allow()}
							</button>
						{/if}
						<button
							class="cursor-pointer rounded-lg px-3 py-1 text-xs font-semibold"
							style="background:{colors.cardBg}; color:{colors.textPrimary}; border:1px solid {colors.borderColor};"
							onclick={() => onDecide(item.requestId, { behavior: 'deny' })}
						>
							{m.harness_dialog_deny()}
						</button>
					</div>
				{:else}
					<div class="text-xs font-semibold" style="color:{colors.textSecondary};">
						{item.state === 'allowed' ? m.harness_dialog_allowed() : m.harness_dialog_denied()}
					</div>
				{/if}
			</div>
		{:else if item.kind === 'notice'}
			<div class="text-xs" style="color:{item.code === 'error' ? '#F87171' : colors.textMuted};">
				{noticeText(item)}
			</div>
		{/if}
	{/each}
	{#if running}
		<div class="text-xs" style="color:{colors.textMuted};">{m.harness_dialog_working()}</div>
	{/if}
</div>

<style>
	.harness-markdown :global(p) {
		margin: 0.25rem 0;
	}
	.harness-markdown :global(pre) {
		overflow-x: auto;
		padding: 0.5rem;
		border-radius: 0.375rem;
		background: rgba(127, 127, 127, 0.12);
	}
	.harness-markdown :global(code) {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 0.8em;
	}
	.harness-markdown :global(ul) {
		list-style: disc;
		padding-left: 1.25rem;
	}
	.harness-markdown :global(ol) {
		list-style: decimal;
		padding-left: 1.25rem;
	}
	.harness-markdown :global(a) {
		text-decoration: underline;
	}
	.harness-markdown :global(table) {
		border-collapse: collapse;
	}
	.harness-markdown :global(th),
	.harness-markdown :global(td) {
		border: 1px solid rgba(127, 127, 127, 0.3);
		padding: 0.15rem 0.4rem;
	}
</style>
