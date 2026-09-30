<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { importMechanicalProductNames, previewMechanicalProductNames } from './mechanical-product-names-import-client';
	import UploadIcon from '@lucide/svelte/icons/upload';
	import FileSpreadsheetIcon from '@lucide/svelte/icons/file-spreadsheet';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import CheckCircle2Icon from '@lucide/svelte/icons/circle-check';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let file = $state<File | null>(null);
	let previewing = $state(false);
	let importing = $state(false);
	let error = $state('');
	let notice = $state('');
	let previewCount = $state<number | null>(null);
	let result = $state<{ inserted: number; skipped: number } | null>(null);

	let panel = $derived(darkMode ? '#202536' : '#FFFFFF');
	let surface = $derived(darkMode ? '#171B26' : '#F7F8FA');
	let border = $derived(darkMode ? '#333A4E' : '#E3E6EA');
	let text = $derived(darkMode ? '#E5EAF3' : '#19212B');
	let muted = $derived(darkMode ? '#A1ACBE' : '#657181');
	let accent = $derived(darkMode ? '#75D6C0' : '#087E70');
	let accentSoft = $derived(darkMode ? 'rgba(117,214,192,.13)' : 'rgba(8,126,112,.09)');

	function selectFile(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		file = input.files?.[0] ?? null;
		previewCount = null;
		result = null;
		error = '';
		notice = '';
	}

	async function preview() {
		if (!file) return;
		previewing = true;
		error = '';
		notice = '';
		try {
			const response = await previewMechanicalProductNames(file);
			previewCount = response.row_count ?? 0;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			previewing = false;
		}
	}

	async function runImport() {
		if (!file || previewCount === null) return;
		if (!confirm(`Translate and import ${previewCount.toLocaleString()} product names? Existing records will remain unchanged.`)) return;
		importing = true;
		error = '';
		notice = '';
		result = null;
		try {
			const response = await importMechanicalProductNames(file);
			result = { inserted: response.inserted ?? 0, skipped: response.skipped ?? 0 };
			notice = m.mechanical_product_names_import_import_completed();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			importing = false;
		}
	}
</script>

<svelte:head>
	<title>{m.mechanical_product_names_import_mechanical_product_names_import()}</title>
</svelte:head>

<div class="mx-auto max-w-5xl px-6 py-8" style={`color:${text}`}>
	<div class="mb-7 flex items-start justify-between gap-6">
		<div>
			<p class="mb-2 text-xs font-semibold uppercase tracking-[.18em]" style={`color:${accent}`}>{m.mechanical_product_names_import_system_admin_resources_import_product()}</p>
			<h1 class="text-2xl font-semibold tracking-tight">{m.mechanical_product_names_import_mechanical_product_names()}</h1>
			<p class="mt-2 max-w-2xl text-sm leading-6" style={`color:${muted}`}>
				{m.mechanical_product_names_import_append_the_china_mechanical_product()} <code>kb.product_names</code>{m.mechanical_product_names_import_product_names_are_translated_to()}
			</p>
		</div>
		<div class="hidden rounded-xl px-4 py-3 text-right sm:block" style={`background:${accentSoft}`}>
			<div class="text-[11px] font-semibold uppercase tracking-wider" style={`color:${accent}`}>{m.mechanical_product_names_import_source()}</div>
			<div class="mt-1 font-mono text-sm" style={`color:${text}`}>{m.mechanical_product_names_import_china_mechanical()}</div>
		</div>
	</div>

	<div class="grid gap-5 lg:grid-cols-[1.3fr_.7fr]">
		<section class="rounded-2xl border p-6" style={`background:${panel};border-color:${border}`}>
			<div class="mb-5 flex items-center gap-3">
				<div class="rounded-xl p-2.5" style={`background:${accentSoft};color:${accent}`}><FileSpreadsheetIcon class="h-5 w-5" /></div>
				<div><h2 class="font-semibold">{m.mechanical_product_names_import_choose_a_catalog_csv()}</h2><p class="mt-1 text-xs" style={`color:${muted}`}>{m.mechanical_product_names_import_utf_8_csv_up_to()}</p></div>
			</div>

			<label class="flex min-h-36 cursor-pointer flex-col items-center justify-center rounded-xl border border-dashed px-5 py-6 text-center transition-colors hover:brightness-110" style={`background:${surface};border-color:${border}`}>
				<UploadIcon class="mb-2 h-5 w-5" style={`color:${accent}`} />
				<strong class="text-sm">{file ? file.name : m.mechanical_product_names_import_select_the_mechanical_catalog_csv()}</strong>
				<span class="mt-1 text-xs" style={`color:${muted}`}>{file ? `${(file.size / 1024).toFixed(0)} KB` : m.mechanical_product_names_import_choose_file_to_upload()}</span>
				<input class="sr-only" type="file" accept=".csv,text/csv" onchange={selectFile} />
			</label>

			{#if previewCount !== null}
				<div class="mt-4 flex items-center gap-2 rounded-xl px-4 py-3 text-sm" style={`background:${accentSoft};color:${accent}`}>
					<CheckCircle2Icon class="h-4 w-4 shrink-0" />
					<span>{m.mechanical_product_names_import_csv_is_valid()} <strong>{previewCount.toLocaleString()}</strong> {m.mechanical_product_names_import_records_ready()}</span>
				</div>
			{/if}
			{#if result}
				<div class="mt-3 rounded-xl border px-4 py-3 text-sm" style={`border-color:${border};background:${surface}`}>
					<strong>{result.inserted.toLocaleString()}</strong> {m.mechanical_product_names_import_inserted()} <span style={`color:${muted}`}>·</span> <strong>{result.skipped.toLocaleString()}</strong> {m.mechanical_product_names_import_already_existed_and_were_skipped()}
				</div>
			{/if}
			{#if error}<p class="mt-4 rounded-lg px-3 py-2 text-sm" style="background:rgba(220,70,70,.1);color:#E87575">{error}</p>{/if}
			{#if notice}<p class="mt-4 text-sm" style={`color:${accent}`}>{notice}</p>{/if}

			<div class="mt-5 flex flex-wrap gap-3">
				<button class="rounded-lg border px-4 py-2.5 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50" style={`border-color:${border};color:${text}`} onclick={preview} disabled={!file || previewing || importing}>
					{#if previewing}<LoaderCircleIcon class="mr-2 inline h-4 w-4 animate-spin" />{/if}{m.mechanical_product_names_import_preview_csv()}
				</button>
				<button class="rounded-lg px-4 py-2.5 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-45" style={`background:${accent};color:${darkMode ? '#10201E' : '#FFFFFF'}`} onclick={runImport} disabled={!file || previewCount === null || importing || previewing}>
					{#if importing}<LoaderCircleIcon class="mr-2 inline h-4 w-4 animate-spin" />{m.mechanical_product_names_import_translating_and_importing()}{:else}{m.mechanical_product_names_import_translate_import()}{/if}
				</button>
			</div>
		</section>

		<aside class="rounded-2xl border p-6" style={`background:${panel};border-color:${border}`}>
			<h2 class="text-sm font-semibold">{m.mechanical_product_names_import_field_mapping()}</h2>
			<dl class="mt-4 space-y-3 text-xs">
				{#each [['code_class_large', 'sub_catalog'], ['code_class_medium', 'category_l1'], ['code_class_small', 'category_l2'], ['product_name', 'product_name'], ['note', 'notes'], ['code_group', 'code_group'], ['child_code_group', 'child_code_group'], ['industry_code', 'industry_code'], ['cpc', 'cpc'], ['entry_no', 'entry_no'], ['entry_no_new', 'entry_no_new'], [m.mechanical_product_names_import_translated_name(), 'product_name_en']] as pair}
					<div class="flex items-center justify-between gap-3 border-b pb-2" style={`border-color:${border}`}><dt class="font-mono" style={`color:${muted}`}>{pair[0]}</dt><dd class="font-mono text-right">{pair[1]}</dd></div>
				{/each}
			</dl>
			<p class="mt-5 rounded-xl p-3 text-xs leading-5" style={`background:${surface};color:${muted}`}>
				{m.mechanical_product_names_import_imports_only_add_records_existing()}
			</p>
		</aside>
	</div>
</div>
