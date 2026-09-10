<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '$lib/paraglide/messages.js';
	import {
		AlertTriangle,
		ArrowUpRight,
		ChevronDown,
		ChevronRight,
		CircleDot,
		Download,
		FileText,
		FolderTree,
		Layers,
		Pencil,
		Plus,
		RefreshCw,
		Trash2,
		X
	} from '@lucide/svelte';
	import {
		acceptNode,
		addNode,
		deleteNode,
		getProfile,
		getReview,
		getRun,
		getRunDiff,
		getRunDocuments,
		getRunResults,
		needsReconcileReview,
		rejectNode,
		rerunReview,
		runExportUrl,
		updateNode,
		type ProfileNode,
		type ResultRow,
		type ReviewRun,
		type RunDiff,
		type ScopedDocument
	} from '$lib/services/productMetricReviewService';

	let { darkMode = false }: { darkMode?: boolean } = $props();

	// ── url params ────────────────────────────────────────────────────────────
	function param(name: string): string {
		if (typeof window === 'undefined') return '';
		return new URLSearchParams(window.location.search).get(name) ?? '';
	}
	let runIdInput = $state(param('run'));
	let runId = $state<number | null>(param('run') ? Number(param('run')) : null);

	// ── loaded state ──────────────────────────────────────────────────────────
	let loading = $state(false);
	let error = $state('');
	let run = $state<ReviewRun | null>(null);
	let profileId = $state<number | null>(null);
	let profileVersion = $state<number | null>(null);
	let currentProfileVersion = $state<number | null>(null);
	let profileName = $state('');
	let nodes = $state<ProfileNode[]>([]);
	let results = $state<ResultRow[]>([]);
	let scopedDocs = $state<ScopedDocument[]>([]);
	let diff = $state<RunDiff | null>(null);

	// ── ui state ──────────────────────────────────────────────────────────────
	let tab = $state<'results' | 'report'>('results');
	let selectedNodeId = $state<number | null>(null);
	let tierFilter = $state<'' | 'direct' | 'part' | 'aspect' | 'document_scope'>('');
	let pathFilter = $state<'' | 'document_first' | 'direct'>('');
	let docFilter = $state<number | null>(null);
	let textFilter = $state('');
	let showDocumentScope = $state(false);
	let drawer = $state<ResultRow | null>(null);
	let collapsed = $state<Record<number, boolean>>({});
	let addingUnder = $state<number | null>(null);
	let newLabel = $state('');
	let newKind = $state<'module' | 'part'>('part');
	let editingNode = $state<number | null>(null);
	let editLabel = $state('');
	let busyNode = $state<number | null>(null);

	const staleVersion = $derived(
		profileVersion != null &&
			currentProfileVersion != null &&
			currentProfileVersion !== profileVersion
	);

	async function load() {
		if (!runId) return;
		loading = true;
		error = '';
		try {
			const r = await getRun(runId);
			run = r.run;
			const review = await getReview(r.run.request_id);
			profileId = review.request.profile_id;
			profileVersion = review.request.profile_version;
			const prof = await getProfile(profileId);
			profileName = prof.profile.name;
			currentProfileVersion = prof.profile.version;
			nodes = prof.nodes;
			const [res, docs, d] = await Promise.all([
				getRunResults(runId),
				getRunDocuments(runId),
				getRunDiff(runId).catch(() => ({ diff: null as unknown as RunDiff }))
			]);
			results = res.results;
			scopedDocs = docs.documents;
			diff = d.diff;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			loading = false;
		}
	}

	async function reloadNodesAndResults() {
		if (!profileId || !runId) return;
		try {
			const [prof, res] = await Promise.all([getProfile(profileId), getRunResults(runId)]);
			nodes = prof.nodes;
			currentProfileVersion = prof.profile.version;
			results = res.results;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	onMount(load);

	function openRun() {
		const n = Number(runIdInput);
		if (!Number.isFinite(n) || n <= 0) return;
		runId = n;
		if (typeof window !== 'undefined') {
			const u = new URL(window.location.href);
			u.searchParams.set('run', String(n));
			window.history.replaceState({}, '', u);
		}
		load();
	}

	async function doRerun() {
		if (!run) return;
		busyNode = -1;
		try {
			const out = await rerunReview(run.request_id);
			runId = out.run.id;
			runIdInput = String(out.run.id);
			if (typeof window !== 'undefined') {
				const u = new URL(window.location.href);
				u.searchParams.set('run', String(out.run.id));
				window.history.replaceState({}, '', u);
			}
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	// ── scope tree ────────────────────────────────────────────────────────────
	type TreeNode = ProfileNode & { children: TreeNode[] };
	const tree = $derived.by(() => {
		const byId = new Map<number, TreeNode>();
		for (const n of nodes) byId.set(n.id, { ...n, children: [] });
		const roots: TreeNode[] = [];
		for (const n of byId.values()) {
			if (n.parent_node_id != null && byId.has(n.parent_node_id)) {
				byId.get(n.parent_node_id)!.children.push(n);
			} else {
				roots.push(n);
			}
		}
		const kindRank: Record<string, number> = { product: 0, module: 1, part: 2, aspect: 3 };
		const sortRec = (list: TreeNode[]) => {
			list.sort(
				(a, b) => (kindRank[a.node_kind] ?? 9) - (kindRank[b.node_kind] ?? 9) || a.id - b.id
			);
			list.forEach((c) => sortRec(c.children));
		};
		sortRec(roots);
		return roots;
	});

	const coverageByNode = $derived.by(() => {
		const map = new Map<number, { artifacts: number; docs: number }>();
		const rep = run?.report_json as
			| { coverage?: { node_id: number; artifact_count: number; document_count: number }[] }
			| undefined;
		for (const c of rep?.coverage ?? [])
			map.set(c.node_id, { artifacts: c.artifact_count, docs: c.document_count });
		return map;
	});

	const nodeLabel = $derived.by(() => {
		const map = new Map<number, string>();
		for (const n of nodes) map.set(n.id, n.label);
		return map;
	});

	// ── results view ──────────────────────────────────────────────────────────
	const filtered = $derived.by(() => {
		const t = textFilter.trim().toLowerCase();
		return results.filter((r) => {
			if (selectedNodeId != null && r.node_id !== selectedNodeId) return false;
			if (tierFilter && r.tier !== tierFilter) return false;
			if (pathFilter && !r.paths.includes(pathFilter)) return false;
			if (docFilter != null && r.input_record_id !== docFilter) return false;
			if (
				t &&
				!`${r.primary_label} ${r.inclusion_reason} ${r.artifact_id}`.toLowerCase().includes(t)
			)
				return false;
			return true;
		});
	});

	const groups = $derived.by(() => {
		const attributed = new Map<number, ResultRow[]>();
		const scope: ResultRow[] = [];
		for (const r of filtered) {
			if (r.tier === 'document_scope' || r.node_id == null) {
				scope.push(r);
			} else {
				const arr = attributed.get(r.node_id) ?? [];
				arr.push(r);
				attributed.set(r.node_id, arr);
			}
		}
		const ordered = [...attributed.entries()].sort(
			(a, b) => b[1].length - a[1].length || a[0] - b[0]
		);
		return { ordered, scope };
	});

	const gaps = $derived.by(() => {
		const rep = run?.report_json as
			| { gaps?: { node_id: number; label: string; kind: string; grounding: string }[] }
			| undefined;
		return rep?.gaps ?? [];
	});

	function selectNode(id: number | null) {
		selectedNodeId = selectedNodeId === id ? null : id;
	}

	async function setStatus(node: ProfileNode, next: 'accepted' | 'rejected') {
		if (!profileId) return;
		busyNode = node.id;
		try {
			if (next === 'accepted') await acceptNode(profileId, node.id);
			else await rejectNode(profileId, node.id);
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	async function removeNode(node: ProfileNode) {
		if (!profileId) return;
		busyNode = node.id;
		try {
			await deleteNode(profileId, node.id);
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	function startRename(node: ProfileNode) {
		editingNode = editingNode === node.id ? null : node.id;
		editLabel = node.label;
	}

	async function submitRename(node: ProfileNode) {
		if (!profileId || !editLabel.trim() || editLabel.trim() === node.label) {
			editingNode = null;
			return;
		}
		busyNode = node.id;
		try {
			await updateNode(profileId, node.id, { label: editLabel.trim() });
			editingNode = null;
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	async function submitNewNode() {
		if (!profileId || addingUnder == null || !newLabel.trim()) return;
		busyNode = addingUnder;
		try {
			await addNode(profileId, {
				parent_node_id: addingUnder,
				node_kind: newKind,
				label: newLabel.trim()
			});
			newLabel = '';
			addingUnder = null;
			await reloadNodesAndResults();
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busyNode = null;
		}
	}

	function recordHref(recordId: number): string {
		return `/home3/knowledge?section=kb-input-details&record_id=${recordId}&dark=${darkMode ? '1' : '0'}`;
	}

	function fmtScore(s: number): string {
		return s.toFixed(3);
	}
	function spansText(spans: unknown): string {
		if (Array.isArray(spans))
			return spans.map((s) => (Array.isArray(s) ? s.join('–') : String(s))).join(', ');
		return typeof spans === 'string' ? spans : '';
	}
</script>

<div class="pmr-shell" class:dark={darkMode}>
	<header class="topbar">
		<div class="brand">
			<span class="brand-mark">PMR</span>
			<div>
				<p class="kicker">{m.pmr_kicker()}</p>
				<p class="brand-name">{m.pmr_title()}</p>
			</div>
		</div>
		<div class="crumbs">
			{#if profileName}
				<span>{profileName}</span><span class="slash">/</span>
			{/if}
			{#if run}<strong>{m.pmr_run_n({ n: run.run_number })}</strong>{/if}
			<span class="route-badge">/home3/product-metric-review</span>
		</div>
	</header>

	<div class="content">
		{#if error}
			<div class="note error">{error}</div>
		{/if}

		{#if !runId}
			<div class="empty-hero">
				<Layers size={22} />
				<h1>{m.pmr_open_run_heading()}</h1>
				<p>{m.pmr_open_run_hint()}</p>
				<div class="run-open">
					<input
						type="number"
						min="1"
						placeholder={m.pmr_run_id_placeholder()}
						bind:value={runIdInput}
						onkeydown={(e) => e.key === 'Enter' && openRun()}
					/>
					<button class="primary" onclick={openRun}>{m.pmr_open()}</button>
				</div>
			</div>
		{:else if loading && !run}
			<div class="note">{m.pmr_loading()}</div>
		{:else if run}
			{#if staleVersion}
				<div class="note warn">
					<AlertTriangle size={14} />
					<span
						>{m.pmr_stale_version({
							shown: profileVersion ?? 0,
							current: currentProfileVersion ?? 0
						})}</span
					>
					<button class="linky" onclick={doRerun} disabled={busyNode === -1}>
						<RefreshCw size={12} />{m.pmr_rerun()}
					</button>
				</div>
			{/if}

			<div class="title-row">
				<div>
					<p class="kicker">{m.pmr_review_of()}</p>
					<h1>{profileName || m.pmr_untitled()}</h1>
					<p class="lede">
						{m.pmr_run_summary({
							status: run.status,
							attributed: run.attributed_count,
							scope: run.document_scope_count,
							docs: run.scoped_document_count
						})}
						{#if run.truncated_count > 0}
							· <span class="warn-text">{m.pmr_truncated({ n: run.truncated_count })}</span>
						{/if}
					</p>
				</div>
				<div class="title-actions">
					<a class="ghost" href={runExportUrl(runId)}><Download size={13} />{m.pmr_export()}</a>
					<button class="ghost" onclick={doRerun} disabled={busyNode === -1}>
						<RefreshCw size={13} />{m.pmr_rerun()}
					</button>
				</div>
			</div>

			<div class="tabs">
				<button class:active={tab === 'results'} onclick={() => (tab = 'results')}>
					<Layers size={13} />{m.pmr_tab_results()} <span>{results.length}</span>
				</button>
				<button class:active={tab === 'report'} onclick={() => (tab = 'report')}>
					<FileText size={13} />{m.pmr_tab_report()} <span>{gaps.length}</span>
				</button>
			</div>

			<div class="layout" class:report-mode={tab === 'report'}>
				<!-- scope tree -->
				<aside class="scope-pane">
					<div class="pane-head">
						<FolderTree size={13} /><span>{m.pmr_scope_tree()}</span>
						{#if selectedNodeId != null}
							<button class="linky" onclick={() => (selectedNodeId = null)}>{m.pmr_clear()}</button>
						{/if}
					</div>
					<div class="tree">
						{#each tree as root (root.id)}
							{@render treeNode(root, 0)}
						{/each}
					</div>
				</aside>

				<!-- main -->
				<section class="main-pane">
					{#if tab === 'results'}
						<div class="filters">
							<select bind:value={tierFilter}>
								<option value="">{m.pmr_all_tiers()}</option>
								<option value="direct">direct</option>
								<option value="part">part</option>
								<option value="aspect">aspect</option>
								<option value="document_scope">document_scope</option>
							</select>
							<select bind:value={pathFilter}>
								<option value="">{m.pmr_all_paths()}</option>
								<option value="direct">direct</option>
								<option value="document_first">document_first</option>
							</select>
							<select bind:value={docFilter}>
								<option value={null}>{m.pmr_all_documents()}</option>
								{#each scopedDocs as d (d.input_record_id)}
									<option value={d.input_record_id}
										>#{d.input_record_id}{d.doc_kind ? ` · ${d.doc_kind}` : ''}</option
									>
								{/each}
							</select>
							<input type="search" placeholder={m.pmr_filter_text()} bind:value={textFilter} />
						</div>

						{#if filtered.length === 0}
							<div class="empty-state">
								<p>{m.pmr_no_results()}</p>
								{#if gaps.length > 0}
									<button class="linky" onclick={() => (tab = 'report')}
										>{m.pmr_see_gaps({ n: gaps.length })}</button
									>
								{/if}
							</div>
						{:else}
							{#each groups.ordered as [nodeId, rows] (nodeId)}
								<div class="result-group">
									<button
										class="group-head"
										onclick={() => selectNode(nodeId)}
										class:sel={selectedNodeId === nodeId}
									>
										<span class="g-label">{nodeLabel.get(nodeId) ?? `#${nodeId}`}</span>
										<span class="g-count">{rows.length}</span>
									</button>
									{#each rows as r (r.artifact_id)}
										{@render resultRow(r)}
									{/each}
								</div>
							{/each}

							{#if groups.scope.length > 0}
								<div class="result-group scope-group">
									<button
										class="group-head"
										onclick={() => (showDocumentScope = !showDocumentScope)}
									>
										{#if showDocumentScope}<ChevronDown size={13} />{:else}<ChevronRight
												size={13}
											/>{/if}
										<span class="g-label">{m.pmr_document_scope()}</span>
										<span class="g-count">{groups.scope.length}</span>
									</button>
									{#if showDocumentScope}
										{#each groups.scope as r (r.artifact_id)}
											{@render resultRow(r)}
										{/each}
									{/if}
								</div>
							{/if}
						{/if}
					{:else}
						{@render reportView()}
					{/if}
				</section>
			</div>
		{/if}
	</div>

	<!-- evidence drawer -->
	{#if drawer}
		<div class="drawer-scrim" onclick={() => (drawer = null)} role="presentation"></div>
		<aside class="drawer">
			<div class="drawer-head">
				<div>
					<p class="kicker">{drawer.tier} · {drawer.artifact_type}</p>
					<h2>{drawer.primary_label || drawer.artifact_id}</h2>
				</div>
				<button class="icon" onclick={() => (drawer = null)}><X size={16} /></button>
			</div>
			<dl class="drawer-fields">
				<dt>{m.pmr_field_artifact()}</dt>
				<dd class="mono">{drawer.artifact_id}</dd>
				<dt>{m.pmr_field_document()}</dt>
				<dd>
					<a href={recordHref(drawer.input_record_id)} target="_blank" rel="noopener">
						#{drawer.input_record_id}
						<ArrowUpRight size={12} />
					</a>
				</dd>
				<dt>{m.pmr_field_line_spans()}</dt>
				<dd class="mono">{spansText(drawer.source_line_spans) || '—'}</dd>
				<dt>{m.pmr_field_matched_node()}</dt>
				<dd>
					{drawer.node_id != null
						? (nodeLabel.get(drawer.node_id) ?? `#${drawer.node_id}`)
						: m.pmr_none()}
				</dd>
				<dt>{m.pmr_field_paths()}</dt>
				<dd class="mono">{drawer.paths.join(' · ') || '—'}</dd>
				<dt>{m.pmr_field_score()}</dt>
				<dd class="mono">{fmtScore(drawer.score)}</dd>
				<dt>{m.pmr_field_reason()}</dt>
				<dd>{drawer.inclusion_reason}</dd>
			</dl>
			<a
				class="ghost wide"
				href={recordHref(drawer.input_record_id)}
				target="_blank"
				rel="noopener"
			>
				{m.pmr_open_source()}
				<ArrowUpRight size={13} />
			</a>
		</aside>
	{/if}
</div>

{#snippet treeNode(node: TreeNode, depth: number)}
	{@const cov = coverageByNode.get(node.id)}
	<div class="tnode" style="--d:{depth}">
		<div
			class="tnode-row"
			class:sel={selectedNodeId === node.id}
			class:rejected={node.status === 'rejected'}
		>
			{#if node.children.length > 0}
				<button
					class="twist"
					onclick={() => (collapsed = { ...collapsed, [node.id]: !collapsed[node.id] })}
				>
					{#if collapsed[node.id]}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
				</button>
			{:else}
				<span class="twist-spacer"></span>
			{/if}
			<button class="tnode-label" onclick={() => selectNode(node.id)}>
				<span class="badge k-{node.node_kind}">{node.node_kind}</span>
				<span class="t-name">{node.label}</span>
				{#if needsReconcileReview(node)}
					<span class="flag" title={node.reconcile_status}><AlertTriangle size={11} /></span>
				{/if}
				<span class="ground g-{node.grounding}" title={node.grounding}><CircleDot size={10} /></span
				>
				<span class="origin"
					>{node.origin === 'llm_proposed'
						? 'llm'
						: node.origin === 'graph_expanded'
							? 'graph'
							: 'user'}</span
				>
				{#if cov}<span class="t-count">{cov.artifacts}</span>{/if}
			</button>
			<div class="tnode-actions">
				{#if node.status !== 'accepted'}
					<button
						class="mini"
						title={m.pmr_accept()}
						disabled={busyNode === node.id}
						onclick={() => setStatus(node, 'accepted')}>✓</button
					>
				{/if}
				{#if node.status !== 'rejected'}
					<button
						class="mini"
						title={m.pmr_reject()}
						disabled={busyNode === node.id}
						onclick={() => setStatus(node, 'rejected')}>✕</button
					>
				{/if}
				<button class="mini" title={m.pmr_edit()} onclick={() => startRename(node)}>
					<Pencil size={11} />
				</button>
				<button
					class="mini"
					title={m.pmr_add_child()}
					onclick={() => (addingUnder = addingUnder === node.id ? null : node.id)}
					><Plus size={11} /></button
				>
				{#if node.node_kind !== 'product'}
					<button
						class="mini"
						title={m.pmr_delete()}
						disabled={busyNode === node.id}
						onclick={() => removeNode(node)}><Trash2 size={11} /></button
					>
				{/if}
			</div>
		</div>
		{#if editingNode === node.id}
			<div class="add-row" style="--d:{depth + 1}">
				<input bind:value={editLabel} onkeydown={(e) => e.key === 'Enter' && submitRename(node)} />
				<button class="mini" onclick={() => submitRename(node)}>{m.pmr_save()}</button>
				<button class="mini" onclick={() => (editingNode = null)}>{m.pmr_cancel()}</button>
			</div>
		{/if}
		{#if addingUnder === node.id}
			<div class="add-row" style="--d:{depth + 1}">
				<select bind:value={newKind}>
					<option value="part">part</option>
					<option value="module">module</option>
				</select>
				<input
					placeholder={m.pmr_new_node_label()}
					bind:value={newLabel}
					onkeydown={(e) => e.key === 'Enter' && submitNewNode()}
				/>
				<button class="mini" onclick={submitNewNode}>{m.pmr_add()}</button>
			</div>
		{/if}
		{#if !collapsed[node.id]}
			{#each node.children as child (child.id)}
				{@render treeNode(child, depth + 1)}
			{/each}
		{/if}
	</div>
{/snippet}

{#snippet resultRow(r: ResultRow)}
	<button class="rrow" onclick={() => (drawer = r)}>
		<span class="r-tier t-{r.tier}">{r.tier}</span>
		<span class="r-label">{r.primary_label || r.artifact_id}</span>
		<span class="r-doc">#{r.input_record_id}</span>
		<span class="r-paths">{r.paths.join('·')}</span>
		<span class="r-score mono">{fmtScore(r.score)}</span>
	</button>
{/snippet}

{#snippet reportView()}
	{@const rep = run?.report_json as
		| {
				coverage?: {
					node_id: number;
					label: string;
					kind: string;
					grounding: string;
					artifact_count: number;
					document_count: number;
				}[];
				per_document_truncated?: Record<string, number>;
		  }
		| undefined}
	<div class="report">
		<div class="metrics-strip">
			<div><strong>{run?.attributed_count ?? 0}</strong><span>{m.pmr_attributed()}</span></div>
			<div>
				<strong>{run?.document_scope_count ?? 0}</strong><span>{m.pmr_document_scope()}</span>
			</div>
			<div>
				<strong>{run?.scoped_document_count ?? 0}</strong><span>{m.pmr_docs_in_scope()}</span>
			</div>
			<div class:warn-tone={(run?.truncated_count ?? 0) > 0}>
				<strong>{run?.truncated_count ?? 0}</strong><span>{m.pmr_truncated_label()}</span>
			</div>
		</div>

		<h3>{m.pmr_coverage()}</h3>
		<div class="tbl-wrap">
			<table>
				<thead>
					<tr
						><th>{m.pmr_node()}</th><th>{m.pmr_kind()}</th><th>{m.pmr_grounding()}</th><th
							class="num">{m.pmr_metrics()}</th
						><th class="num">{m.pmr_documents()}</th></tr
					>
				</thead>
				<tbody>
					{#each rep?.coverage ?? [] as c (c.node_id)}
						<tr class:zero={c.artifact_count === 0}>
							<td>{c.label}</td>
							<td class="mono">{c.kind}</td>
							<td class="mono">{c.grounding}</td>
							<td class="num">{c.artifact_count}</td>
							<td class="num">{c.document_count}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<h3>{m.pmr_gaps_heading({ n: gaps.length })}</h3>
		{#if gaps.length === 0}
			<p class="muted">{m.pmr_no_gaps()}</p>
		{:else}
			<ul class="gaps">
				{#each gaps as g (g.node_id)}
					<li><strong>{g.label}</strong> <span class="mono">{g.kind} · {g.grounding}</span></li>
				{/each}
			</ul>
		{/if}

		{#if rep?.per_document_truncated && Object.keys(rep.per_document_truncated).length > 0}
			<h3>{m.pmr_truncation()}</h3>
			<ul class="gaps">
				{#each Object.entries(rep.per_document_truncated) as [doc, n] (doc)}
					<li><span class="mono">#{doc}</span> — {m.pmr_truncated({ n })}</li>
				{/each}
			</ul>
		{/if}

		{#if diff && (diff.added.length || diff.removed.length || diff.retiered.length || diff.profile_version_changed)}
			<h3>{m.pmr_diff_heading({ prev: diff.previous_run_number })}</h3>
			{#if diff.profile_version_changed}
				<p class="warn-text">
					{m.pmr_diff_version({
						a: diff.previous_profile_version,
						b: diff.current_profile_version
					})}
				</p>
			{/if}
			<div class="diff-cols">
				<div>
					<h4>{m.pmr_added({ n: diff.added.length })}</h4>
					<ul>
						{#each diff.added as a (a.artifact_id)}<li class="mono">
								{a.artifact_id} <em>{a.tier}</em>
							</li>{/each}
					</ul>
				</div>
				<div>
					<h4>{m.pmr_removed({ n: diff.removed.length })}</h4>
					<ul>
						{#each diff.removed as a (a.artifact_id)}<li class="mono">
								{a.artifact_id} <em>{a.tier}</em>
							</li>{/each}
					</ul>
				</div>
				<div>
					<h4>{m.pmr_retiered({ n: diff.retiered.length })}</h4>
					<ul>
						{#each diff.retiered as a (a.artifact_id)}<li class="mono">
								{a.artifact_id} <em>{a.from}→{a.to}</em>
							</li>{/each}
					</ul>
				</div>
			</div>
		{/if}
	</div>
{/snippet}

<style>
	@import url('https://fonts.googleapis.com/css2?family=DM+Mono:wght@400;500&family=Manrope:wght@400;500;600;700;800&display=swap');

	.pmr-shell {
		--bg: oklch(0.965 0.014 78);
		--surface: oklch(0.985 0.009 78);
		--text: oklch(0.23 0.025 63);
		--subtle: oklch(0.55 0.025 72);
		--border: oklch(0.88 0.028 75);
		--bronze: oklch(0.61 0.09 69);
		--green: oklch(0.55 0.1 150);
		--blue: oklch(0.56 0.11 245);
		--violet: oklch(0.58 0.11 300);
		--red: oklch(0.57 0.14 27);
		--shadow: 0 12px 30px oklch(0.3 0.02 70 / 0.06);
		min-height: 100vh;
		background: var(--bg);
		color: var(--text);
		font-family:
			'Manrope',
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			sans-serif;
		transition:
			background 180ms ease,
			color 180ms ease;
	}
	.pmr-shell.dark {
		--bg: #111827;
		--surface: #182334;
		--text: #f7f5f1;
		--subtle: #c4cfdf;
		--border: #304663;
		--bronze: #ff9b54;
		--green: #47b699;
		--blue: #78a5e5;
		--violet: #9c8de2;
		--red: #ff7d6b;
		--shadow: 0 12px 30px rgba(7, 10, 18, 0.32);
	}
	.pmr-shell :global(*) {
		box-sizing: border-box;
	}
	.pmr-shell button,
	.pmr-shell input,
	.pmr-shell select {
		font: inherit;
		color: inherit;
	}
	.pmr-shell button:focus-visible,
	.pmr-shell a:focus-visible,
	.pmr-shell input:focus-visible,
	.pmr-shell select:focus-visible {
		outline: 2px solid var(--bronze);
		outline-offset: 2px;
	}

	.kicker {
		margin: 0;
		text-transform: uppercase;
		letter-spacing: 0.13em;
		font:
			500 10px 'DM Mono',
			monospace;
		color: var(--bronze);
	}
	.mono {
		font-family: 'DM Mono', monospace;
	}
	.muted {
		color: var(--subtle);
		font-size: 12px;
	}
	.warn-text {
		color: var(--bronze);
	}

	.topbar {
		height: 62px;
		padding: 0 clamp(16px, 3vw, 40px);
		border-bottom: 1px solid var(--border);
		display: flex;
		justify-content: space-between;
		align-items: center;
		background: color-mix(in oklch, var(--surface) 82%, transparent);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 11px;
	}
	.brand-mark {
		width: 30px;
		height: 30px;
		border: 1px solid var(--bronze);
		color: var(--bronze);
		display: grid;
		place-items: center;
		font:
			500 11px 'DM Mono',
			monospace;
		letter-spacing: -0.06em;
	}
	.brand-name {
		margin: 2px 0 0;
		font-size: 13px;
		font-weight: 700;
	}
	.crumbs {
		display: flex;
		align-items: center;
		gap: 9px;
		color: var(--subtle);
		font-size: 12px;
	}
	.crumbs strong {
		color: var(--text);
	}
	.slash {
		color: var(--bronze);
	}
	.route-badge {
		margin-left: 6px;
		padding: 3px 8px;
		border: 1px solid var(--border);
		border-radius: 99px;
		font:
			500 10px 'DM Mono',
			monospace;
	}

	.content {
		max-width: 1720px;
		margin: 0 auto;
		padding: 26px clamp(16px, 2.4vw, 40px) 40px;
	}

	.note {
		display: flex;
		align-items: center;
		gap: 9px;
		margin: 12px 0;
		padding: 10px 12px;
		border: 1px solid var(--border);
		background: color-mix(in oklch, var(--surface) 60%, var(--bronze) 5%);
		font-size: 12px;
	}
	.note.error {
		color: var(--red);
		border-color: color-mix(in oklch, var(--red) 45%, var(--border));
	}
	.note.warn {
		color: var(--bronze);
		border-color: color-mix(in oklch, var(--bronze) 45%, var(--border));
	}
	.linky {
		border: 0;
		background: transparent;
		color: var(--bronze);
		cursor: pointer;
		font-size: 11px;
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 0 0 0 6px;
	}
	.linky:disabled {
		opacity: 0.5;
		cursor: wait;
	}

	.empty-hero {
		margin: 60px auto;
		max-width: 460px;
		text-align: center;
		color: var(--subtle);
	}
	.empty-hero h1 {
		margin: 14px 0 6px;
		font-size: 22px;
		letter-spacing: -0.03em;
		color: var(--text);
	}
	.empty-hero p {
		margin: 0 0 18px;
		font-size: 13px;
		line-height: 1.6;
	}
	.run-open {
		display: flex;
		gap: 8px;
		justify-content: center;
	}
	.run-open input {
		width: 150px;
		padding: 9px 11px;
		border: 1px solid var(--border);
		background: var(--surface);
	}

	.title-row {
		display: flex;
		justify-content: space-between;
		gap: 24px;
		align-items: end;
		margin: 22px 0 18px;
	}
	.title-row h1 {
		margin: 6px 0 8px;
		font-size: clamp(24px, 3vw, 38px);
		letter-spacing: -0.05em;
		line-height: 1.04;
	}
	.lede {
		margin: 0;
		max-width: 640px;
		color: var(--subtle);
		font-size: 13px;
		line-height: 1.6;
	}
	.title-actions {
		display: flex;
		gap: 8px;
		flex-shrink: 0;
	}
	.ghost {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 9px 12px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text);
		font-size: 12px;
		text-decoration: none;
		cursor: pointer;
	}
	.ghost:hover {
		background: var(--surface);
	}
	.ghost:disabled {
		opacity: 0.5;
		cursor: wait;
	}
	.ghost.wide {
		justify-content: center;
		width: 100%;
		margin-top: 14px;
	}
	.primary {
		padding: 9px 14px;
		border: 1px solid var(--bronze);
		background: var(--bronze);
		color: var(--bg);
		cursor: pointer;
		font-size: 12px;
	}

	.tabs {
		display: flex;
		gap: 2px;
		border-bottom: 1px solid var(--border);
		margin-bottom: 16px;
	}
	.tabs button {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 10px 14px 11px;
		border: 0;
		border-bottom: 2px solid transparent;
		background: transparent;
		color: var(--subtle);
		cursor: pointer;
		font-size: 12px;
	}
	.tabs button:hover,
	.tabs button.active {
		color: var(--text);
		border-color: var(--bronze);
	}
	.tabs button span {
		color: var(--bronze);
		font:
			500 10px 'DM Mono',
			monospace;
	}

	.layout {
		display: grid;
		grid-template-columns: minmax(280px, 340px) 1fr;
		gap: 16px;
		align-items: start;
	}
	.layout.report-mode {
		grid-template-columns: minmax(260px, 300px) 1fr;
	}
	.pane-head {
		display: flex;
		align-items: center;
		gap: 7px;
		padding-bottom: 9px;
		border-bottom: 1px solid var(--border);
		margin-bottom: 8px;
		font:
			500 10px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--bronze);
	}
	.pane-head span {
		margin-right: auto;
	}
	.scope-pane,
	.main-pane {
		border: 1px solid var(--border);
		background: var(--surface);
		box-shadow: var(--shadow);
		padding: 14px;
	}
	.scope-pane {
		position: sticky;
		top: 14px;
		max-height: calc(100vh - 40px);
		overflow: auto;
	}

	/* tree */
	.tnode-row {
		display: flex;
		align-items: center;
		gap: 2px;
		padding-left: calc(var(--d) * 13px);
		border-radius: 3px;
	}
	.tnode-row.sel {
		background: color-mix(in oklch, var(--bronze) 12%, transparent);
	}
	.tnode-row.rejected {
		opacity: 0.42;
		text-decoration: line-through;
	}
	.twist,
	.mini,
	.icon {
		border: 0;
		background: transparent;
		color: var(--subtle);
		cursor: pointer;
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}
	.twist {
		width: 16px;
		height: 22px;
		flex-shrink: 0;
	}
	.twist-spacer {
		width: 16px;
		flex-shrink: 0;
	}
	.tnode-label {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		gap: 5px;
		border: 0;
		background: transparent;
		cursor: pointer;
		padding: 4px 2px;
		text-align: left;
	}
	.t-name {
		font-size: 12px;
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.badge {
		flex-shrink: 0;
		padding: 1px 5px;
		border: 1px solid currentColor;
		border-radius: 2px;
		font:
			500 8.5px 'DM Mono',
			monospace;
		text-transform: uppercase;
	}
	.k-product {
		color: var(--bronze);
	}
	.k-module {
		color: var(--violet);
	}
	.k-part {
		color: var(--blue);
	}
	.k-aspect {
		color: var(--green);
	}
	.origin {
		flex-shrink: 0;
		color: var(--subtle);
		font:
			500 8.5px 'DM Mono',
			monospace;
	}
	.ground {
		flex-shrink: 0;
		display: inline-flex;
	}
	.g-object_node {
		color: var(--green);
	}
	.g-keyword_concept {
		color: var(--blue);
	}
	.g-ontology_term {
		color: var(--violet);
	}
	.g-ungrounded {
		color: var(--subtle);
		opacity: 0.5;
	}
	.flag {
		flex-shrink: 0;
		color: var(--red);
		display: inline-flex;
	}
	.t-count {
		flex-shrink: 0;
		margin-left: auto;
		padding: 0 5px;
		border: 1px solid var(--border);
		border-radius: 99px;
		font:
			500 9px 'DM Mono',
			monospace;
		color: var(--subtle);
	}
	.tnode-actions {
		display: flex;
		gap: 1px;
		flex-shrink: 0;
		opacity: 0;
		transition: opacity 120ms;
	}
	.tnode-row:hover .tnode-actions {
		opacity: 1;
	}
	.mini {
		width: 20px;
		height: 20px;
		font-size: 11px;
	}
	.mini:hover {
		color: var(--text);
	}
	.mini:disabled {
		opacity: 0.4;
	}
	.add-row {
		display: flex;
		gap: 5px;
		margin: 4px 0 6px;
		padding-left: calc(var(--d) * 13px + 18px);
	}
	.add-row input {
		flex: 1;
		min-width: 0;
		padding: 5px 7px;
		border: 1px solid var(--border);
		background: var(--bg);
		font-size: 11px;
	}
	.add-row select {
		padding: 5px;
		border: 1px solid var(--border);
		background: var(--bg);
		font-size: 11px;
	}
	.add-row .mini {
		width: auto;
		padding: 0 8px;
		border: 1px solid var(--border);
		font-size: 11px;
	}

	/* filters + results */
	.filters {
		display: flex;
		gap: 8px;
		margin-bottom: 12px;
		flex-wrap: wrap;
	}
	.filters select,
	.filters input {
		padding: 7px 9px;
		border: 1px solid var(--border);
		background: var(--bg);
		font-size: 11px;
	}
	.filters input {
		flex: 1;
		min-width: 120px;
	}

	.result-group {
		margin-bottom: 10px;
	}
	.group-head {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 7px 8px;
		border: 0;
		border-left: 2px solid var(--bronze);
		background: color-mix(in oklch, var(--bronze) 6%, transparent);
		cursor: pointer;
		font-size: 11px;
		text-align: left;
	}
	.group-head.sel {
		background: color-mix(in oklch, var(--bronze) 14%, transparent);
	}
	.scope-group .group-head {
		border-left-color: var(--subtle);
		background: color-mix(in oklch, var(--subtle) 8%, transparent);
	}
	.g-label {
		font-weight: 700;
	}
	.g-count {
		margin-left: auto;
		font:
			500 10px 'DM Mono',
			monospace;
		color: var(--subtle);
	}
	.rrow {
		width: 100%;
		display: grid;
		grid-template-columns: 78px 1fr 56px minmax(0, 90px) 58px;
		gap: 8px;
		align-items: center;
		padding: 8px;
		border: 0;
		border-bottom: 1px solid var(--border);
		background: transparent;
		cursor: pointer;
		text-align: left;
		font-size: 11px;
	}
	.rrow:hover {
		background: color-mix(in oklch, var(--bronze) 5%, transparent);
	}
	.r-tier {
		padding: 2px 5px;
		border: 1px solid currentColor;
		border-radius: 2px;
		font:
			500 8.5px 'DM Mono',
			monospace;
		text-align: center;
	}
	.t-direct {
		color: var(--bronze);
	}
	.t-part {
		color: var(--blue);
	}
	.t-aspect {
		color: var(--green);
	}
	.t-document_scope {
		color: var(--subtle);
	}
	.r-label {
		font-weight: 600;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.r-doc,
	.r-paths {
		color: var(--subtle);
		font:
			500 10px 'DM Mono',
			monospace;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.r-score {
		text-align: right;
		color: var(--subtle);
	}

	.empty-state {
		padding: 40px 0;
		text-align: center;
		color: var(--subtle);
		font-size: 12px;
	}

	/* report */
	.report h3 {
		margin: 22px 0 10px;
		font-size: 14px;
		letter-spacing: -0.02em;
	}
	.report h4 {
		margin: 0 0 6px;
		font:
			500 10px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--bronze);
	}
	.metrics-strip {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
		gap: 8px;
	}
	.metrics-strip > div {
		padding: 12px 13px;
		border: 1px solid var(--border);
		border-top: 2px solid var(--bronze);
		background: var(--bg);
	}
	.metrics-strip > div.warn-tone {
		border-top-color: var(--red);
	}
	.metrics-strip strong {
		display: block;
		font:
			700 22px 'DM Mono',
			monospace;
		letter-spacing: -0.06em;
	}
	.metrics-strip span {
		font-size: 10px;
		color: var(--subtle);
	}
	.tbl-wrap {
		overflow-x: auto;
		border: 1px solid var(--border);
	}
	.report table {
		width: 100%;
		border-collapse: collapse;
		font-size: 11px;
		min-width: 460px;
	}
	.report th {
		padding: 9px 12px;
		text-align: left;
		color: var(--subtle);
		font:
			500 9px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		border-bottom: 1px solid var(--border);
	}
	.report td {
		padding: 9px 12px;
		border-bottom: 1px solid var(--border);
	}
	.report th.num,
	.report td.num {
		text-align: right;
		font-family: 'DM Mono', monospace;
	}
	.report tr.zero td {
		color: var(--subtle);
	}
	.gaps {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.gaps li {
		padding: 7px 0;
		border-bottom: 1px solid var(--border);
		font-size: 12px;
	}
	.gaps .mono {
		color: var(--subtle);
		font-size: 10px;
		margin-left: 6px;
	}
	.diff-cols {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 14px;
	}
	.diff-cols ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.diff-cols li {
		padding: 4px 0;
		font-size: 10px;
	}
	.diff-cols em {
		color: var(--bronze);
		font-style: normal;
	}

	/* drawer */
	.drawer-scrim {
		position: fixed;
		inset: 0;
		background: oklch(0.2 0.02 60 / 0.28);
		z-index: 40;
	}
	.drawer {
		position: fixed;
		top: 0;
		right: 0;
		width: min(420px, 92vw);
		height: 100vh;
		z-index: 41;
		background: var(--surface);
		border-left: 1px solid var(--border);
		box-shadow: -18px 0 40px oklch(0.2 0.02 60 / 0.18);
		padding: 18px;
		overflow: auto;
		animation: slidein 160ms ease;
	}
	@keyframes slidein {
		from {
			transform: translateX(24px);
			opacity: 0;
		}
	}
	.drawer-head {
		display: flex;
		justify-content: space-between;
		gap: 12px;
		align-items: start;
		border-bottom: 1px solid var(--border);
		padding-bottom: 12px;
	}
	.drawer-head h2 {
		margin: 5px 0 0;
		font-size: 16px;
		letter-spacing: -0.02em;
	}
	.icon {
		width: 28px;
		height: 28px;
	}
	.icon:hover {
		color: var(--text);
	}
	.drawer-fields {
		display: grid;
		grid-template-columns: 108px 1fr;
		gap: 9px 12px;
		margin: 14px 0 0;
		font-size: 12px;
	}
	.drawer-fields dt {
		color: var(--subtle);
		font:
			500 9px 'DM Mono',
			monospace;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		padding-top: 2px;
	}
	.drawer-fields dd {
		margin: 0;
		word-break: break-word;
	}
	.drawer-fields a {
		color: var(--bronze);
		text-decoration: none;
		display: inline-flex;
		align-items: center;
		gap: 3px;
	}

	@media (max-width: 1000px) {
		.layout,
		.layout.report-mode {
			grid-template-columns: 1fr;
		}
		.scope-pane {
			position: static;
			max-height: 340px;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.pmr-shell :global(*) {
			animation-duration: 0.01ms !important;
			transition-duration: 0.01ms !important;
		}
	}
</style>
