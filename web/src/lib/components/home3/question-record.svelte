<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { getLocale } from '$lib/paraglide/runtime';
	import NameValueVisualizer from '$lib/components/shared-ui/name-value/name-value-visualizer.svelte';
	import type { NameValueInput } from '$lib/components/shared-ui/name-value/name-value';
	import type { GeneratedQuestion } from '$lib/services/questionGeneratorService';

	let { question, index }: { question: GeneratedQuestion; index: number } = $props();
	const title = $derived(m.qg_question_number({ number: index }));
	const fields: NameValueInput[] = $derived([
		{
			key: 'question',
			label: m.qg_field_question(),
			values: [{ text: question.question, lang: question.lang }]
		},
		{
			key: 'answer',
			label: m.qg_answer(),
			values: [{ text: question.answer, lang: question.lang }]
		},
		{
			key: 'purpose',
			label: m.qg_filter_purpose(),
			values: [question.purpose === 'search_metrics' ? m.qg_search_metrics() : question.purpose]
		},
		{
			key: 'run',
			label: m.qg_filter_run(),
			values: [{ text: String(question.run_id), mono: true }]
		},
		{
			key: 'record',
			label: m.qg_filter_record(),
			values: [{ text: String(question.input_record_id), mono: true }]
		},
		{
			key: 'language',
			label: m.qg_filter_language(),
			values: [
				question.lang === 'en'
					? m.qg_english()
					: question.lang === 'zh-cn'
						? m.qg_chinese()
						: question.lang
			]
		},
		{
			key: 'hop',
			label: m.qg_field_hop_type(),
			values: [
				question.hop_type === 'one_hop'
					? m.qg_one_hop()
					: question.hop_type === 'two_hop'
						? m.qg_two_hop()
						: m.qg_multi_hop()
			]
		},
		{
			key: 'metric',
			label: m.qg_field_metric(),
			values: [{ text: question.metric_id, mono: true }]
		},
		{ key: 'model', label: m.qg_field_model(), values: [question.model_name] },
		{ key: 'prompt', label: m.qg_field_prompt(), values: [question.prompt_name] },
		{
			key: 'created',
			label: m.qg_field_created_at(),
			values: [new Date(question.created_at).toLocaleString(getLocale())]
		}
	]);
	const evidenceFields: NameValueInput[] = $derived([
		{
			key: 'reasoning',
			label: m.qg_reasoning(),
			values: (question.reasoning_steps ?? []).map((step, index) => ({
				text: `${index + 1}. ${step}${question.step_line_ids?.[index]?.length ? `\n${m.qg_step_lines({ lines: question.step_line_ids[index].join(', ') })}` : ''}`,
				lang: question.lang
			}))
		},
		{
			key: 'evidence',
			label: m.qg_evidence(),
			values: (question.evidence ?? []).map((item) => ({
				text: `${m.qg_source_line({ line: item.line_id })}\n${item.text}`
			}))
		}
	]);
</script>

<article class="question select-text" aria-label={title}>
	<h3 class="question-heading">{title}</h3>
	<NameValueVisualizer {fields} labelWidth="9rem" ariaLabel={title} />
	<details>
		<summary>{m.qg_reasoning_evidence()}</summary>
		<NameValueVisualizer
			fields={evidenceFields}
			labelWidth="9rem"
			ariaLabel={m.qg_reasoning_evidence()}
		/>
	</details>
</article>

<style>
	.question {
		min-width: 0;
		border: 1px solid var(--line);
		border-radius: 8px;
		padding: 20px 16px;
		overflow-wrap: anywhere;
	}
	.question-heading {
		display: flex;
		align-items: center;
		gap: 10px;
		margin: 0 0 20px;
		font-size: 14px;
		font-weight: 600;
	}
	summary {
		margin: 20px 0 12px;
		font-size: 13px;
		color: var(--accent);
		cursor: pointer;
	}
	summary:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
</style>
