<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import KbInputRecordBrowser from './kb-input-record-browser.svelte';
	import PdfViewWindow from './pdf-view-window.svelte';
	import { listKbReferences, type KbReferenceMention, type KbInputRecord } from '$lib/services/kbService';

	let { darkMode = true }: { darkMode?: boolean } = $props();
	let record = $state<KbInputRecord | null>(null);
	let mentions = $state<KbReferenceMention[]>([]);
	let selected = $state<KbReferenceMention | null>(null);
	let loading = $state(false);
	let error = $state('');
	let pageNo = $state(1);
	let zoom = $state(0.7);
	let numPages = $state(0);
	let palette = $derived(darkMode ? {
		bg: '#0E1116', panel: '#161A22', panelAlt: '#1C212C', line: '#2A3140',
		text: '#EDE7D3', secondary: '#B5AE94', muted: '#7C7560', accent: '#D4A24C'
	} : {
		bg: '#F5F1E8', panel: '#FBF8F0', panelAlt: '#F0EADB', line: '#D7CFB8',
		text: '#1A1410', secondary: '#5C5345', muted: '#8F8472', accent: '#B8801E'
	});

	async function selectRecord(value: KbInputRecord) {
		record = value;
		selected = null;
		mentions = [];
		loading = true;
		error = '';
		try {
			const response = await listKbReferences(value.id);
			mentions = response.results ?? [];
			selected = mentions[0] ?? null;
		} catch (e) {
			error = e instanceof Error ? e.message : m.references_mgmt_load_failed();
		} finally {
			loading = false;
		}
	}

	function mapRecord(input: KbInputRecord) {
		return {
			id: input.id,
			title: input.title?.trim() || input.name?.trim() || input.file_name?.trim() || `Input #${input.id}`,
			subtitle: input.file_name || input.name || '',
			meta: [input.doc_no || '', input.type || ''].filter(Boolean),
			status: input.type || '',
			description: m.references_mgmt_select_document_to_review(),
			badges: [input.type || '']
		};
	}
</script>

<div class="references-page select-text" style={`--ref-bg:${palette.bg};--ref-panel:${palette.panel};--ref-alt:${palette.panelAlt};--ref-line:${palette.line};--ref-text:${palette.text};--ref-secondary:${palette.secondary};--ref-muted:${palette.muted};--ref-accent:${palette.accent}`}>
	<header class="heading">
		<div><div class="eyebrow">{m.references_mgmt_knowledge_system_vol_iii()}</div><h1>{m.references_mgmt_title()}</h1><p>{m.references_mgmt_subtitle()}</p></div>
		<div class="stats"><small>{m.references_mgmt_record()}</small><strong>{record?.id ?? '—'}</strong><small>{m.references_mgmt_references()}</small><strong>{String(mentions.length).padStart(3, '0')}</strong></div>
	</header>
	<div class="workspace">
		<aside class="records">
			<KbInputRecordBrowser {darkMode} instanceKey="references-record-browser" title={m.references_mgmt_kb_inputs()} subtitle={m.references_mgmt_search_and_select()} emptyTitle={m.references_mgmt_no_records()} emptySubtitle={m.references_mgmt_use_search_to_browse()} autoSelectFirstRecord={false} selectedRecordId={record?.id ?? null} mapRecord={mapRecord} onSelect={selectRecord} onError={(e) => (error = e.message)} />
		</aside>
		<section class="mention-column" aria-label={m.references_mgmt_references()}>
			<div class="column-head"><strong>{m.references_mgmt_references()}</strong><span>{mentions.length} {m.references_mgmt_found()}</span></div>
			{#if loading}<p class="notice">{m.references_mgmt_loading()}</p>
			{:else if error}<p class="notice error">{error}</p>
			{:else if !record}<p class="notice">{m.references_mgmt_select_document()}</p>
			{:else if mentions.length === 0}<p class="notice">{m.references_mgmt_no_references_for_document()}</p>
			{:else}
				<div class="mention-list">
					{#each mentions as mention, i (mention.id)}
						<button class="mention-card" class:active={selected?.id === mention.id} onclick={() => (selected = mention)}>
							<div class="card-meta"><span>№ {String(i + 1).padStart(3, '0')} · {mention.ref_id}</span><span>{mention.normativity}</span></div>
							<strong>{mention.target_title || mention.cited_raw || mention.resource_title || m.references_mgmt_untitled_reference()}</strong>
							<p>{mention.evidence || mention.cited_raw || m.references_mgmt_no_excerpt()}</p>
							<div class="chips"><span>{mention.target_type} · {mention.target_content}</span>{#if mention.r2_state}<span>{mention.r2_state}</span>{/if}{#if mention.metric_state}<span>{mention.metric_state}</span>{/if}</div>
						</button>
					{/each}
				</div>
			{/if}
		</section>
		<section class="document" aria-label={m.references_mgmt_source_document()}>
			{#if record}
				<PdfViewWindow {darkMode} inputId={record.id} fileUrl={`/api/v1/kb/inputs/${record.id}/file`} bind:page={pageNo} bind:zoom bind:numPages sidebarTitle={m.references_mgmt_reference_details()} sidebarSettingsKey="references-pdf-sidebar">
					{#snippet sidebar()}
						{#if selected}
							<div class="detail"><div class="detail-label">{m.references_mgmt_selected_reference()}</div><h2>{selected.target_title || selected.cited_raw || selected.resource_title || m.references_mgmt_untitled_reference()}</h2>
								<dl>
									<dt>{m.references_mgmt_type()}</dt><dd>{selected.target_type} / {selected.target_subtype}</dd>
									<dt>{m.references_mgmt_content()}</dt><dd>{selected.target_content}</dd>
									<dt>{m.references_mgmt_normativity()}</dt><dd>{selected.normativity}{selected.list_kind ? ` · ${selected.list_kind}` : ''}</dd>
									<dt>{m.references_mgmt_holding_resolution()}</dt><dd>{selected.r2_state || m.references_mgmt_unresolved()}{selected.target_input_title ? ` · ${selected.target_input_title}` : ''}</dd>
									<dt>{m.references_mgmt_resource_identity()}</dt><dd>{selected.identity_key || '—'}</dd>
									<dt>{m.references_mgmt_identifiers()}</dt><dd>{selected.identifiers?.join(', ') || '—'}</dd>
									<dt>{m.references_mgmt_source_excerpt()}</dt><dd>{selected.evidence || selected.cited_raw || '—'}</dd>
								</dl>
							</div>
						{:else}<p class="notice">{m.references_mgmt_select_reference()}</p>{/if}
					{/snippet}
				</PdfViewWindow>
			{:else}
				<div class="empty-document">{m.references_mgmt_select_document()}</div>
			{/if}
		</section>
	</div>
</div>

<style>
	.references-page{height:100%;min-height:0;display:flex;flex-direction:column;background:var(--ref-bg);color:var(--ref-text);font-family:'Inter Tight',system-ui,sans-serif;user-select:text}
	.heading{flex:none;min-height:126px;padding:22px 28px;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--ref-line);background:var(--ref-bg)}
	.eyebrow,.stats small,.detail-label{font:600 10px 'JetBrains Mono',monospace;letter-spacing:.16em;text-transform:uppercase;color:var(--ref-accent)}
	h1{font:600 34px 'Cormorant Garamond',Georgia,serif;margin:6px 0;color:var(--ref-text)}
	.heading p{margin:0;color:var(--ref-secondary);font:italic 13px Georgia,serif}
	.stats{display:grid;grid-template-columns:auto auto;gap:5px 18px;text-align:right;padding:12px 16px;background:var(--ref-panel);border:1px solid var(--ref-line)}
	.stats strong{font:600 14px 'JetBrains Mono',monospace}
	.workspace{flex:1;min-height:0;display:grid;grid-template-columns:minmax(265px,23%) minmax(280px,25%) minmax(400px,1fr);overflow:hidden}
	.records,.mention-column,.document{min-width:0;min-height:0;border-right:1px solid var(--ref-line);background:var(--ref-panel)}
	.records{position:relative;overflow:auto}.mention-column{display:flex;flex-direction:column;background:var(--ref-bg)}.document{overflow:hidden;border-right:0;background:var(--ref-bg)}
	.column-head{height:58px;flex:none;display:flex;align-items:center;justify-content:space-between;padding:0 18px;border-bottom:1px solid var(--ref-line);color:var(--ref-text)}
	.column-head span{font:10px 'JetBrains Mono',monospace;color:var(--ref-accent);text-transform:uppercase;letter-spacing:.1em}
	.mention-list{min-height:0;overflow:auto;padding:10px;display:flex;flex-direction:column;gap:8px}
	.mention-card{width:100%;text-align:left;padding:12px;background:var(--ref-panel);border:1px solid var(--ref-line);border-left:3px solid transparent;color:var(--ref-text);cursor:pointer}
	.mention-card:hover,.mention-card.active{border-left-color:var(--ref-accent);background:var(--ref-alt)}
	.card-meta{display:flex;justify-content:space-between;gap:8px;font:9px 'JetBrains Mono',monospace;color:var(--ref-muted);text-transform:uppercase}
	.mention-card strong{display:block;margin:9px 0 5px;font-size:14px}.mention-card p{margin:0;color:var(--ref-secondary);font-size:12px;line-height:1.5;display:-webkit-box;line-clamp:3;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden}
	.chips{display:flex;flex-wrap:wrap;gap:5px;margin-top:10px}.chips span{padding:3px 6px;border:1px solid var(--ref-line);color:var(--ref-accent);font:9px 'JetBrains Mono',monospace}
	.notice,.empty-document{padding:24px;color:var(--ref-muted);font-size:13px}.error{color:#c8553d}.empty-document{height:100%;display:grid;place-items:center}
	.detail{padding:16px;color:var(--ref-text)}.detail h2{font:600 19px Georgia,serif;margin:10px 0 16px;overflow-wrap:anywhere}.detail dl{display:grid;grid-template-columns:115px minmax(0,1fr);gap:9px 10px;margin:0}.detail dt{font:9px 'JetBrains Mono',monospace;color:var(--ref-muted);text-transform:uppercase;letter-spacing:.08em}.detail dd{margin:0;font-size:12px;overflow-wrap:anywhere;line-height:1.45}
	@media(max-width:1000px){.workspace{grid-template-columns:240px minmax(250px,1fr)}.document{grid-column:1/-1;min-height:400px}.heading{min-height:110px}}
</style>
