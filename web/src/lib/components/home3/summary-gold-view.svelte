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
	<header class="flex flex-wrap items-center justify-between gap-3 border-b px-6 py-4" style="border-color:{line};">
		<div>
			<div class="text-xs font-semibold uppercase tracking-[0.16em]" style="color:{accent};">{m.sgv_eyebrow()}</div>
			<h1 class="mt-1 text-xl font-semibold">{m.sgv_title()}</h1>
			<p class="mt-1 text-sm" style="color:{muted};">{m.sgv_subtitle()}</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<label class="text-xs" for="sgv-language" style="color:{muted};">{m.sgv_language()}</label>
			<select id="sgv-language" bind:value={languageFilter} class="rounded-md border px-2 py-1.5 text-sm" style="background:{panel};border-color:{line};color:{text};">
				<option value="all">{m.sgv_all_languages()}</option>
				<option value="en">{m.sgv_english()}</option>
				<option value="zh">{m.sgv_chinese()}</option>
			</select>
			<label class="text-xs" for="sgv-stratum" style="color:{muted};">{m.sgv_group()}</label>
			<select id="sgv-stratum" bind:value={stratumFilter} class="rounded-md border px-2 py-1.5 text-sm" style="background:{panel};border-color:{line};color:{text};">
				<option value="all">{m.sgv_all_groups()}</option>
				<option value="diagnostic">{m.sgv_diagnostic()}</option>
				<option value="real-pilot">{m.sgv_real_pilot()}</option>
			</select>
		</div>
	</header>

	<div class="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[250px_minmax(0,1fr)]">
		<aside class="border-b p-3 lg:border-b-0 lg:border-r" style="border-color:{line};background:{panel};">
			<div class="mb-2 flex items-center justify-between px-2 text-xs font-semibold uppercase tracking-wide" style="color:{muted};">
				<span>{m.sgv_cases()}</span><span>{filteredCases.length}</span>
			</div>
			<div class="flex gap-2 overflow-x-auto lg:flex-col lg:overflow-x-visible">
				{#each filteredCases as item (item.id)}
					<button type="button" onclick={() => choose(item)} class="min-w-48 rounded-lg border px-3 py-2 text-left transition-colors lg:min-w-0" style="border-color:{item.id === selectedCase?.id ? accent : line};background:{item.id === selectedCase?.id ? panelAlt : 'transparent'};color:{text};">
						<span class="block truncate text-sm font-medium">{item.id}</span>
						<span class="mt-1 block text-xs" style="color:{muted};">{item.language === 'zh' ? m.sgv_chinese() : m.sgv_english()} · {item.stratum === 'real-pilot' ? m.sgv_real_pilot() : m.sgv_diagnostic()}</span>
						<span class="mt-1 block text-xs" style="color:{muted};">{m.sgv_unit_count({ count: item.units.length })} · {m.sgv_budget({ count: item.budget_chars })}</span>
					</button>
				{/each}
				{#if filteredCases.length === 0}<p class="px-2 py-4 text-sm" style="color:{muted};">{m.sgv_no_cases()}</p>{/if}
			</div>
		</aside>

		{#if selectedCase}
			<section class="min-w-0 p-4 lg:p-6">
				<div class="mb-4 flex flex-wrap items-start justify-between gap-3">
					<div>
						<h2 class="text-lg font-semibold">{selectedCase.id}</h2>
						<p class="mt-1 max-w-3xl text-sm" style="color:{muted};">{selectedCase.source.description}</p>
					</div>
					<div class="flex flex-wrap gap-2 text-xs">
						<span class="rounded-full border px-2.5 py-1" style="border-color:{line};background:{panel};">{selectedCase.language === 'zh' ? m.sgv_chinese() : m.sgv_english()}</span>
						<span class="rounded-full border px-2.5 py-1" style="border-color:{line};background:{panel};">{selectedCase.stratum === 'real-pilot' ? m.sgv_real_pilot() : m.sgv_diagnostic()}</span>
						<span class="rounded-full border px-2.5 py-1" style="border-color:{line};background:{panel};">{m.sgv_budget({ count: selectedCase.budget_chars })}</span>
					</div>
				</div>
				<div class="mb-5 flex flex-wrap gap-1.5">
					{#each selectedCase.tags as tag}<span class="rounded bg-black/5 px-2 py-1 text-xs dark:bg-white/10">{tag}</span>{/each}
				</div>
				<div class="grid min-w-0 gap-4 xl:grid-cols-2">
					<section class="min-w-0 overflow-hidden rounded-xl border" style="border-color:{line};background:{panel};">
						<h3 class="border-b px-4 py-3 text-sm font-semibold" style="border-color:{line};">{m.sgv_source_lines()}</h3>
						<ol class="max-h-[55vh] overflow-auto p-2 font-mono text-xs leading-relaxed">
							{#each selectedCase.lines as sourceLine (sourceLine.id)}
								<li class="grid grid-cols-[3rem_minmax(0,1fr)] gap-2 rounded px-2 py-1.5" style="background:{sourceLine.kind === 'heading' ? panelAlt : 'transparent'};">
									<span class="select-text text-right" style="color:{muted};">{sourceLine.id}</span><span class="whitespace-pre-wrap break-words">{sourceLine.text}</span>
								</li>
							{/each}
						</ol>
					</section>
					<section class="min-w-0 overflow-hidden rounded-xl border" style="border-color:{line};background:{panel};">
						<div class="flex items-center justify-between border-b px-4 py-3" style="border-color:{line};">
							<h3 class="text-sm font-semibold">{m.sgv_gold_units()}</h3>
							<span class="text-xs" style="color:{muted};">{selectedCase.units.length}</span>
						</div>
						<div class="max-h-[55vh] space-y-3 overflow-auto p-3">
							{#each selectedCase.units as unit (unit.id)}
								<article class="rounded-lg border p-3" style="border-color:{line};background:{panelAlt};">
									<div class="flex flex-wrap items-center gap-2">
										<span class="font-mono text-xs font-semibold">{unit.id}</span>
										<span class="rounded px-2 py-0.5 text-xs" style="background:{unit.critical ? 'rgba(192,83,58,0.14)' : 'rgba(120,130,145,0.15)'};color:{unit.critical ? critical : muted};">{unit.critical ? m.sgv_critical() : m.sgv_noncritical()}</span>
										<span class="text-xs" style="color:{muted};">{m.sgv_weight({ count: unit.weight })}</span>
									</div>
									<p class="mt-2 text-sm">{unit.rationale}</p>
									<div class="mt-3 space-y-2">
										{#each unit.evidence as alternative, alternativeIndex}
											<div class="rounded border px-2.5 py-2" style="border-color:{line};background:{panel};">
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
				</div>
			</section>
		{:else}
			<div class="p-8 text-sm" style="color:{muted};">{m.sgv_no_cases()}</div>
		{/if}
	</div>
</div>
