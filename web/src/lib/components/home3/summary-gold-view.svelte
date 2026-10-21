<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import benchmark from '../../../../../benchmark/summaries/datasets/1.0.0/benchmark.json';

	type EvidenceSpan = { line: number; start: number; end: number };
	type GoldUnit = {
		id: string;
		kind: string;
		critical: boolean;
		weight: number;
		rationale: string;
		evidence: EvidenceSpan[][];
	};
	type Case = {
		id: string;
		language: string;
		stratum: string;
		tags: string[];
		budget_chars: number;
		source: { kind: string; description: string };
		lines: { id: number; kind: string; text: string }[];
		units: GoldUnit[];
	};
	const cases = benchmark.cases as Case[];
	let { darkMode = true }: { darkMode: boolean } = $props();
	let languageFilter = $state('all');
	let stratumFilter = $state('all');
	let selectedId = $state(cases[0]?.id ?? '');
	const filteredCases = $derived(cases.filter((item) =>
		(languageFilter === 'all' || item.language === languageFilter) &&
		(stratumFilter === 'all' || item.stratum === stratumFilter)
	));
	const selectedCase = $derived(filteredCases.find((item) => item.id === selectedId) ?? filteredCases[0] ?? null);
	const bg = $derived(darkMode ? '#10141B' : '#F6F3EB');
	const panel = $derived(darkMode ? '#171D27' : '#FFFFFF');
	const panelAlt = $derived(darkMode ? '#1D2531' : '#F0ECE2');
	const line = $derived(darkMode ? '#303A49' : '#DDD7CA');
	const text = $derived(darkMode ? '#E8EAF0' : '#25231F');
	const muted = $derived(darkMode ? '#A0A9B8' : '#716B60');
	const accent = $derived(darkMode ? '#D6AA5B' : '#946A22');
	const critical = $derived(darkMode ? '#F0A28B' : '#A3442D');
	function excerpt(span: EvidenceSpan, item: Case): string {
		const source = item.lines.find((entry) => entry.id === span.line)?.text ?? '';
		return Array.from(source).slice(span.start, span.end).join('');
	}
	function choose(item: Case) {
		selectedId = item.id;
	}
	$effect(() => {
		if (!filteredCases.some((item) => item.id === selectedId)) selectedId = filteredCases[0]?.id ?? '';
	});
</script>

<svelte:head>
	<title>{m.sgv_title()}</title>
</svelte:head>


<div class="flex min-h-full min-w-0 flex-col select-text" style="background:{bg};color:{text};">
	<header class="flex min-h-[58px] flex-wrap items-center justify-between gap-3 border-b px-4 py-2.5" style="border-color:{line};background:{panel};">
		<div class="flex min-w-0 items-baseline gap-4">
			<h1 class="shrink-0 text-[22px] font-semibold tracking-tight" style="font-family:Georgia,'Times New Roman',serif;">{m.sgv_title()}</h1>
			<p class="hidden truncate text-xs xl:block" style="color:{muted};">{m.sgv_subtitle()}</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<label class="text-[11px]" for="sgv-language" style="color:{muted};">{m.sgv_language()}</label>
			<select id="sgv-language" bind:value={languageFilter} class="rounded-md border px-2 py-1 text-xs" style="background:{bg};border-color:{line};color:{text};">
				<option value="all">{m.sgv_all_languages()}</option>
				<option value="en">{m.sgv_english()}</option>
				<option value="zh">{m.sgv_chinese()}</option>
			</select>
			<label class="text-[11px]" for="sgv-stratum" style="color:{muted};">{m.sgv_group()}</label>
			<select id="sgv-stratum" bind:value={stratumFilter} class="rounded-md border px-2 py-1 text-xs" style="background:{bg};border-color:{line};color:{text};">
				<option value="all">{m.sgv_all_groups()}</option>
				<option value="diagnostic">{m.sgv_diagnostic()}</option>
				<option value="real-pilot">{m.sgv_real_pilot()}</option>
			</select>
		</div>
	</header>

	<div class="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[220px_minmax(280px,0.9fr)_minmax(0,2fr)]">
		<aside class="flex min-h-0 flex-col border-b p-2 lg:border-b-0 lg:border-r" style="border-color:{line};background:{panel};">
			<div class="mb-2 flex items-center justify-between border-b px-2 py-2 text-[10px] font-semibold uppercase tracking-[0.14em]" style="border-color:{line};color:{muted};">
				<span>{m.sgv_cases()}</span><span>{filteredCases.length}</span>
			</div>
			<div class="flex gap-2 overflow-x-auto lg:min-h-0 lg:flex-1 lg:flex-col lg:overflow-x-hidden lg:overflow-y-auto">
				{#each filteredCases as item (item.id)}
					<button type="button" onclick={() => choose(item)} class="min-w-48 rounded-md border px-2.5 py-2 text-left transition-colors lg:min-w-0" style="border-color:{item.id === selectedCase?.id ? accent : line};background:{item.id === selectedCase?.id ? panelAlt : 'transparent'};color:{text};">
						<span class="block truncate text-xs font-semibold">{item.id}</span>
						<span class="mt-1 block text-[10px] leading-snug" style="color:{muted};">{item.language === 'zh' ? m.sgv_chinese() : m.sgv_english()} · {item.stratum === 'real-pilot' ? m.sgv_real_pilot() : m.sgv_diagnostic()}</span>
						<span class="mt-1 block text-[10px] leading-snug" style="color:{muted};">{m.sgv_unit_count({ count: item.units.length })} · {m.sgv_budget({ count: item.budget_chars })}</span>
					</button>
				{/each}
				{#if filteredCases.length === 0}<p class="px-2 py-4 text-sm" style="color:{muted};">{m.sgv_no_cases()}</p>{/if}
			</div>
		</aside>

		{#if selectedCase}
			<section class="flex min-h-0 min-w-0 flex-col border-b lg:border-b-0 lg:border-r" style="border-color:{line};background:{bg};">
			<div class="flex items-center justify-between border-b px-3 py-2.5" style="border-color:{line};background:{panel};">
				<h2 class="text-xs font-semibold">{m.sgv_source_lines()}</h2>
				<span class="text-[10px]" style="color:{muted};">{selectedCase.lines.length}</span>
			</div>
			<div class="border-b px-3 py-2" style="border-color:{line};">
				<p class="text-xs font-medium">{selectedCase.id}</p>
				<p class="mt-1 text-[10px] leading-relaxed" style="color:{muted};">{selectedCase.source.description}</p>
				<div class="mt-2 flex flex-wrap gap-1">
					{#each selectedCase.tags as tag}<span class="rounded px-1.5 py-0.5 text-[9px]" style="background:{panelAlt};color:{muted};">{tag}</span>{/each}
				</div>
			</div>
			<ol class="min-h-0 flex-1 overflow-auto p-2 font-mono text-[11px] leading-relaxed">
							{#each selectedCase.lines as sourceLine (sourceLine.id)}
								<li class="grid grid-cols-[1.5rem_minmax(0,1fr)] gap-2 rounded px-1.5 py-1.5" style="background:{sourceLine.kind === 'heading' ? panelAlt : 'transparent'};">
									<span class="select-text text-right" style="color:{muted};">{sourceLine.id}</span><span class="whitespace-pre-wrap break-words">{sourceLine.text}</span>
								</li>
							{/each}
						</ol>
			</section>
			<section class="flex min-h-0 min-w-0 flex-col" style="background:{bg};">
				<div class="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2.5" style="border-color:{line};background:{panel};">
					<div class="flex items-baseline gap-3"><h2 class="text-xs font-semibold">{m.sgv_gold_units()}</h2><span class="text-[10px]" style="color:{muted};">{selectedCase.units.length}</span></div>
					<div class="flex flex-wrap gap-1.5 text-[10px]">
						<span class="rounded border px-2 py-1" style="border-color:{line};background:{bg};">{selectedCase.language === 'zh' ? m.sgv_chinese() : m.sgv_english()}</span>
						<span class="rounded border px-2 py-1" style="border-color:{line};background:{bg};">{selectedCase.stratum === 'real-pilot' ? m.sgv_real_pilot() : m.sgv_diagnostic()}</span>
						<span class="rounded border px-2 py-1" style="border-color:{line};background:{bg};">{m.sgv_budget({ count: selectedCase.budget_chars })}</span>
					</div>
				</div>
				<div class="min-h-0 flex-1 space-y-2 overflow-auto p-3">
							{#each selectedCase.units as unit (unit.id)}
								<article class="rounded-md border p-3" style="border-color:{line};background:{panel};">
									<div class="flex flex-wrap items-center gap-2">
										<span class="font-mono text-xs font-semibold">{unit.id}</span>
										<span class="rounded px-2 py-0.5 text-xs" style="background:{unit.critical ? 'rgba(192,83,58,0.14)' : 'rgba(120,130,145,0.15)'};color:{unit.critical ? critical : muted};">{unit.critical ? m.sgv_critical() : m.sgv_noncritical()}</span>
										<span class="text-xs" style="color:{muted};">{m.sgv_weight({ count: unit.weight })}</span>
									</div>
										<p class="mt-2 text-xs leading-relaxed">{unit.rationale}</p>
									<div class="mt-3 space-y-2">
										{#each unit.evidence as alternative, alternativeIndex}
											<div class="rounded border px-2.5 py-2" style="border-color:{line};background:{bg};">
												<div class="mb-1 text-[10px] font-semibold uppercase tracking-wide" style="color:{muted};">{m.sgv_evidence_option({ count: alternativeIndex + 1 })}</div>
												{#each alternative as span}
													<p class="font-mono text-xs leading-relaxed">{m.sgv_line_ref({ line: span.line })} {excerpt(span, selectedCase)}</p>
												{/each}
											</div>
										{/each}
									</div>
								</article>
							{/each}
				</div>
			</section>
		{:else}
			<div class="p-8 text-sm" style="color:{muted};">{m.sgv_no_cases()}</div>
		{/if}
	</div>
</div>
