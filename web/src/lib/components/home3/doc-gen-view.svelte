<script lang="ts">
	import { onMount, onDestroy } from 'svelte';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// --- Design tokens (match home3 palette) ---
	let pageBg      = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg      = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2    = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent      = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint  = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted   = $derived(darkMode ? '#64748B' : '#9CA3AF');

	type Tab = 'generate' | 'history' | 'queries';
	let activeTab = $state<Tab>('generate');
	let isAdmin = $state(false);

	// --- Generate tab state ---
	let requestName  = $state('');
	let purpose      = $state('');
	let remarks      = $state('');
	let sqlSearch    = $state('');
	let sqlQueryID   = $state<number | null>(null);
	let sqlStatement = $state('');
	let templateType = $state('word');
	let templateName = $state('');
	let converterStr = $state('{}');
	let outputDir    = $state('');
	let outputFormat = $state('docx');
	let submitError  = $state('');
	let submitSuccess = $state('');
	let submitting   = $state(false);

	// --- Query search state ---
	let queryResults = $state<{id:number,name:string,description:string,sql_statement:string}[]>([]);
	let querySearchLoading = $state(false);

	// --- Template state ---
	let templates = $state<string[]>([]);

	// --- History tab state ---
	type Job = {
		job_id: number; request_name: string; purpose: string; status: string;
		total_count: number; success_count: number; fail_count: number;
		created_by: string; created_at: string;
	};
	type LogEntry = {
		id: number; filename: string; customer_name: string;
		status: string; error_msg: string;
	};
	let jobs = $state<Job[]>([]);
	let jobTotal = $state(0);
	let jobPage = $state(1);
	let jobStatusFilter = $state('');
	let jobNameFilter = $state('');
	let historyLoading = $state(false);
	let expandedJobID = $state<number | null>(null);
	let jobLogs = $state<Record<number, LogEntry[]>>({});
	let historyRefreshInterval: ReturnType<typeof setInterval> | null = null;

	// --- SQL Queries tab state ---
	type SQLQuery = {id:number;name:string;description:string;sql_statement:string;created_by:string;created_at:string};
	let queryList = $state<SQLQuery[]>([]);
	let queryListSearch = $state('');
	let queryListLoading = $state(false);
	let showAddQuery = $state(false);
	let newQueryName = $state('');
	let newQueryDesc = $state('');
	let newQuerySQL  = $state('');
	let addQueryError = $state('');
	let deleteQueryError = $state('');

	onMount(async () => {
		await checkAdmin();
		await loadTemplates();
		await loadHistory();
		startAutoRefresh();
	});

	onDestroy(() => {
		if (historyRefreshInterval !== null) {
			clearInterval(historyRefreshInterval);
			historyRefreshInterval = null;
		}
	});

	async function checkAdmin() {
		try {
			const res = await fetch('/api/v1/ai-assistant/user-info', { credentials: 'same-origin' });
			if (res.ok) {
				const data = await res.json();
				isAdmin = data?.user?.role === 'admin' || data?.role === 'admin';
			}
		} catch { /* ignore */ }
	}

	async function loadTemplates() {
		try {
			const res = await fetch('/api/v1/docgen/templates', { credentials: 'same-origin' });
			if (res.ok) {
				const data = await res.json();
				templates = data.templates ?? [];
			}
		} catch { /* ignore */ }
	}

	async function searchQueries() {
		if (!sqlSearch.trim()) { queryResults = []; return; }
		querySearchLoading = true;
		try {
			const res = await fetch(`/api/v1/docgen/queries?q=${encodeURIComponent(sqlSearch)}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); queryResults = data.queries ?? []; }
		} finally { querySearchLoading = false; }
	}

	function pickQuery(q: {id:number;name:string;sql_statement:string}) {
		sqlQueryID = q.id;
		sqlStatement = q.sql_statement;
		sqlSearch = q.name;
		queryResults = [];
	}

	async function handleFileUpload(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files?.length) return;
		const formData = new FormData();
		formData.append('file', input.files[0]);
		const res = await fetch('/api/v1/docgen/templates', { method: 'POST', body: formData, credentials: 'same-origin' });
		if (res.ok) {
			const data = await res.json();
			templateName = data.filename;
			await loadTemplates();
		}
	}

	async function submitJob() {
		submitError = ''; submitSuccess = ''; submitting = true;
		try {
			let converter: Record<string,string> = {};
			try { converter = JSON.parse(converterStr); } catch {
				submitError = 'Converter must be valid JSON.'; return;
			}
			const body: Record<string,unknown> = {
				request_name: requestName, purpose, remarks,
				template_type: templateType, template_name: templateName,
				converter, output_dir: outputDir, output_format: outputFormat
			};
			if (sqlQueryID !== null) body.sql_query_id = sqlQueryID;
			else body.sql_statement = sqlStatement;

			const res = await fetch('/api/v1/docgen/jobs', {
				method: 'POST', credentials: 'same-origin',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			const data = await res.json();
			if (!res.ok) { submitError = data.error_msg ?? 'Submission failed.'; return; }
			submitSuccess = `Job created! ID: ${data.job_id}`;
			requestName = ''; purpose = ''; remarks = ''; sqlStatement = '';
			sqlQueryID = null; sqlSearch = ''; converterStr = '{}';
		} finally { submitting = false; }
	}

	async function loadHistory() {
		historyLoading = true;
		try {
			const params = new URLSearchParams({ page: String(jobPage), page_size: '20' });
			if (jobStatusFilter) params.set('status', jobStatusFilter);
			if (jobNameFilter) params.set('request_name', jobNameFilter);
			const res = await fetch(`/api/v1/docgen/jobs?${params}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); jobs = data.jobs ?? []; jobTotal = data.total ?? 0; }
		} finally { historyLoading = false; }
	}

	function startAutoRefresh() {
		historyRefreshInterval = setInterval(() => {
			const hasActive = jobs.some(j => j.status === 'pending' || j.status === 'processing');
			if (hasActive) loadHistory();
		}, 5000);
	}

	async function toggleJobExpand(jobID: number) {
		if (expandedJobID === jobID) { expandedJobID = null; return; }
		expandedJobID = jobID;
		if (!jobLogs[jobID]) {
			const res = await fetch(`/api/v1/docgen/jobs/${jobID}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); jobLogs = { ...jobLogs, [jobID]: data.logs ?? [] }; }
		}
	}

	async function loadQueryList() {
		queryListLoading = true;
		try {
			const res = await fetch(`/api/v1/docgen/queries?q=${encodeURIComponent(queryListSearch)}`, { credentials: 'same-origin' });
			if (res.ok) { const data = await res.json(); queryList = data.queries ?? []; }
		} finally { queryListLoading = false; }
	}

	async function addQuery() {
		addQueryError = '';
		if (!newQueryName || !newQuerySQL) { addQueryError = 'Name and SQL are required.'; return; }
		const res = await fetch('/api/v1/docgen/queries', {
			method: 'POST', credentials: 'same-origin',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name: newQueryName, description: newQueryDesc, sql_statement: newQuerySQL })
		});
		if (!res.ok) { const d = await res.json(); addQueryError = d.error_msg ?? 'Failed.'; return; }
		showAddQuery = false; newQueryName = ''; newQueryDesc = ''; newQuerySQL = '';
		await loadQueryList();
	}

	async function deleteQuery(id: number) {
		if (!confirm('Delete this query?')) return;
		deleteQueryError = '';
		const res = await fetch(`/api/v1/docgen/queries/${id}`, { method: 'DELETE', credentials: 'same-origin' });
		if (!res.ok) {
			const d = await res.json().catch(() => ({}));
			deleteQueryError = d.error_msg ?? 'Failed to delete query.';
			return;
		}
		await loadQueryList();
	}

	function statusBadgeStyle(status: string): string {
		const map: Record<string, string> = {
			pending:    'background:#6B7280;color:white',
			processing: `background:${accent};color:white`,
			completed:  'background:#10B981;color:white',
			failed:     'background:#EF4444;color:white',
			generated:  'background:#10B981;color:white',
		};
		return map[status] ?? 'background:#6B7280;color:white';
	}
</script>

<div class="flex flex-col h-full overflow-y-auto p-6" style="background:{pageBg};">
	<!-- Tab bar -->
	<div class="flex gap-1 mb-6 p-1 rounded-xl flex-shrink-0" style="background:{surface2}; border:1px solid {borderColor}; width:fit-content;">
		{#each ([['generate','Generate'],['history','History'],isAdmin ? ['queries','SQL Queries'] : null] as const).filter(Boolean) as [id, label]}
			<button
				onclick={() => { activeTab = id as Tab; if (id === 'history') loadHistory(); if (id === 'queries') loadQueryList(); }}
				class="px-4 py-1.5 rounded-lg text-sm font-medium transition-colors duration-150"
				style="background:{activeTab === id ? accent : 'transparent'}; color:{activeTab === id ? 'white' : textSecondary};"
			>{label}</button>
		{/each}
	</div>

	<!-- ===== GENERATE TAB ===== -->
	{#if activeTab === 'generate'}
		<div class="rounded-xl p-6 max-w-2xl" style="background:{cardBg}; border:1px solid {borderColor};">
			<h2 class="text-lg font-semibold mb-5" style="color:{textPrimary};">New Document Generation Job</h2>

			{#if submitSuccess}
				<div class="mb-4 p-3 rounded-lg text-sm" style="background:#10B981; color:white;">{submitSuccess}</div>
			{/if}
			{#if submitError}
				<div class="mb-4 p-3 rounded-lg text-sm" style="background:#EF4444; color:white;">{submitError}</div>
			{/if}

			<div class="space-y-4">
				<!-- Request Name -->
				<div>
					<label for="docgen-request-name" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Request Name *</label>
					<input id="docgen-request-name" bind:value={requestName} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Unique identifier" />
				</div>

				<!-- Purpose -->
				<div>
					<label for="docgen-purpose" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Purpose *</label>
					<input id="docgen-purpose" bind:value={purpose} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Brief description of this doc run" />
				</div>

				<!-- SQL Query search-and-pick -->
				<div>
					<label for="docgen-sql-query" class="block text-sm font-medium mb-1" style="color:{textSecondary};">SQL Query *</label>
					<div class="relative">
						<input
							id="docgen-sql-query"
							bind:value={sqlSearch}
							oninput={searchQueries}
							class="w-full px-3 py-2 rounded-lg text-sm"
							style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};"
							placeholder="Search predefined queries by name…"
						/>
						{#if queryResults.length > 0}
							<div class="absolute z-10 left-0 right-0 mt-1 rounded-lg overflow-hidden" style="background:{cardBg}; border:1px solid {borderColor}; box-shadow:0 8px 24px rgba(0,0,0,0.15);">
								{#each queryResults as q}
									<button onclick={() => pickQuery(q)} class="w-full text-left px-3 py-2 text-sm hover:opacity-80 transition-opacity" style="background:transparent; color:{textPrimary}; border-bottom:1px solid {borderColor};">
										<div class="font-medium">{q.name}</div>
										{#if q.description}<div class="text-xs" style="color:{textMuted};">{q.description}</div>{/if}
									</button>
								{/each}
							</div>
						{/if}
					</div>
					{#if sqlStatement}
						<pre class="mt-2 p-2 rounded text-xs overflow-x-auto" style="background:{surface2}; color:{textSecondary}; border:1px solid {borderColor};">{sqlStatement}</pre>
					{/if}
				</div>

				<!-- Template Type -->
				<div>
					<label for="docgen-template-type" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Template Type *</label>
					<select id="docgen-template-type" bind:value={templateType} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
						<option value="word">Word (.docx)</option>
						<option value="typst">Typst (not yet supported)</option>
					</select>
				</div>

				<!-- Template Name -->
				<div>
					<label for="docgen-template-name" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Template *</label>
					<div class="flex gap-2">
						<select id="docgen-template-name" bind:value={templateName} class="flex-1 px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
							<option value="">Select a template…</option>
							{#each templates as t}<option value={t}>{t}</option>{/each}
						</select>
						<label class="flex items-center px-3 py-2 rounded-lg text-sm cursor-pointer transition-opacity hover:opacity-80" style="background:{accentTint}; color:{accent}; border:1px solid {accent}30;">
							Upload
							<input type="file" class="hidden" accept=".docx,.typ" onchange={handleFileUpload} />
						</label>
					</div>
				</div>

				<!-- Converter -->
				<div>
					<label for="docgen-converter-json" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Converter JSON * <span class="font-normal text-xs" style="color:{textMuted};">(sql_column → template_token; must include customer_id, customer_name, email as values)</span></label>
					<textarea id="docgen-converter-json" bind:value={converterStr} rows={4} class="w-full px-3 py-2 rounded-lg text-sm font-mono" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder={'{"customer_id_col":"customer_id","name_col":"customer_name","email_col":"email"}'}></textarea>
				</div>

				<!-- Output Dir -->
				<div>
					<label for="docgen-output-dir" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Output Directory *</label>
					<input id="docgen-output-dir" bind:value={outputDir} class="w-full px-3 py-2 rounded-lg text-sm font-mono" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Data/docgen/output" />
				</div>

				<!-- Output Format -->
				<div>
					<label for="docgen-output-format" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Output Format *</label>
					<select id="docgen-output-format" bind:value={outputFormat} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
						<option value="docx">DOCX</option>
						<option value="pdf">PDF (not yet supported)</option>
					</select>
				</div>

				<!-- Remarks -->
				<div>
					<label for="docgen-remarks" class="block text-sm font-medium mb-1" style="color:{textSecondary};">Remarks</label>
					<textarea id="docgen-remarks" bind:value={remarks} rows={2} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};"></textarea>
				</div>

				<button
					onclick={submitJob}
					disabled={submitting}
					class="w-full py-2.5 rounded-lg text-sm font-semibold transition-opacity hover:opacity-88 disabled:opacity-50"
					style="background:{accent}; color:white; border:none;"
				>{submitting ? 'Submitting…' : 'Generate Documents'}</button>
			</div>
		</div>
	{/if}

	<!-- ===== HISTORY TAB ===== -->
	{#if activeTab === 'history'}
		<div class="space-y-4">
			<!-- Filters -->
			<div class="flex gap-3 flex-wrap">
				<select bind:value={jobStatusFilter} onchange={loadHistory} class="px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">
					<option value="">All statuses</option>
					<option value="pending">Pending</option>
					<option value="processing">Processing</option>
					<option value="completed">Completed</option>
					<option value="failed">Failed</option>
				</select>
				<input bind:value={jobNameFilter} oninput={loadHistory} class="px-3 py-2 rounded-lg text-sm flex-1 min-w-40" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Filter by request name…" />
			</div>

			{#if historyLoading}
				<div class="text-sm" style="color:{textMuted};">Loading…</div>
			{:else if jobs.length === 0}
				<div class="text-sm" style="color:{textMuted};">No jobs found.</div>
			{:else}
				<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor};">
					<!-- Header -->
					<div class="grid text-xs font-semibold px-4 py-2" style="grid-template-columns:2fr 1fr 1fr 1fr 1fr; background:{surface2}; color:{textMuted};">
						<span>Request Name</span><span>Status</span><span>Results</span><span>Created By</span><span>Created At</span>
					</div>
					{#each jobs as job}
						<div style="border-top:1px solid {borderColor};">
							<button
								onclick={() => toggleJobExpand(job.job_id)}
								class="grid w-full text-left px-4 py-3 text-sm hover:opacity-80 transition-opacity"
								style="grid-template-columns:2fr 1fr 1fr 1fr 1fr; background:{expandedJobID === job.job_id ? accentTint : cardBg}; color:{textPrimary};"
							>
								<span class="font-medium truncate">{job.request_name}</span>
								<span><span class="px-2 py-0.5 rounded-full text-xs font-semibold" style="{statusBadgeStyle(job.status)}">{job.status}</span></span>
								<span style="color:{textSecondary};">{job.success_count}/{job.total_count}</span>
								<span class="truncate" style="color:{textSecondary};">{job.created_by}</span>
								<span style="color:{textMuted};">{new Date(job.created_at).toLocaleString()}</span>
							</button>
							{#if expandedJobID === job.job_id}
								<div class="px-4 pb-3" style="background:{surface2};">
									{#if !jobLogs[job.job_id]}
										<div class="text-xs py-2" style="color:{textMuted};">Loading log…</div>
									{:else if jobLogs[job.job_id].length === 0}
										<div class="text-xs py-2" style="color:{textMuted};">No log entries yet.</div>
									{:else}
										<table class="w-full text-xs mt-2">
											<thead><tr style="color:{textMuted};"><th class="text-left pb-1">Filename</th><th class="text-left pb-1">Customer</th><th class="text-left pb-1">Status</th><th class="text-left pb-1">Error</th></tr></thead>
											<tbody>
												{#each jobLogs[job.job_id] as entry}
													<tr style="border-top:1px solid {borderColor}; color:{textSecondary};">
														<td class="py-1 pr-2 truncate max-w-xs">{entry.filename}</td>
														<td class="py-1 pr-2">{entry.customer_name}</td>
														<td class="py-1 pr-2"><span class="px-1.5 py-0.5 rounded-full text-xs font-semibold" style="{statusBadgeStyle(entry.status)}">{entry.status}</span></td>
														<td class="py-1" style="color:#EF4444;">{entry.error_msg}</td>
													</tr>
												{/each}
											</tbody>
										</table>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>

				<!-- Pagination -->
				<div class="flex items-center gap-3 text-sm" style="color:{textSecondary};">
					<button disabled={jobPage <= 1} onclick={() => { jobPage--; loadHistory(); }} class="px-3 py-1 rounded-lg disabled:opacity-40" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">Prev</button>
					<span>Page {jobPage} · {jobTotal} total</span>
					<button disabled={jobPage * 20 >= jobTotal} onclick={() => { jobPage++; loadHistory(); }} class="px-3 py-1 rounded-lg disabled:opacity-40" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};">Next</button>
				</div>
			{/if}
		</div>
	{/if}

	<!-- ===== SQL QUERIES TAB (admin only) ===== -->
	{#if activeTab === 'queries' && isAdmin}
		<div class="space-y-4">
			<div class="flex items-center gap-3">
				<input bind:value={queryListSearch} oninput={loadQueryList} class="flex-1 px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Search queries…" />
				<button onclick={() => showAddQuery = !showAddQuery} class="px-4 py-2 rounded-lg text-sm font-semibold" style="background:{accent}; color:white; border:none;">+ Add Query</button>
			</div>

			{#if showAddQuery}
				<div class="rounded-xl p-4 space-y-3" style="background:{cardBg}; border:1px solid {borderColor};">
					<h3 class="text-sm font-semibold" style="color:{textPrimary};">New Predefined Query</h3>
					{#if addQueryError}<div class="text-xs p-2 rounded" style="background:#EF4444; color:white;">{addQueryError}</div>{/if}
					<input bind:value={newQueryName} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Name *" />
					<input bind:value={newQueryDesc} class="w-full px-3 py-2 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="Description" />
						<textarea bind:value={newQuerySQL} rows={4} class="w-full px-3 py-2 rounded-lg text-sm font-mono" style="background:{surface2}; border:1px solid {borderColor}; color:{textPrimary};" placeholder="SELECT ... *"></textarea>
					<div class="flex gap-2">
						<button onclick={addQuery} class="px-4 py-1.5 rounded-lg text-sm font-semibold" style="background:{accent}; color:white; border:none;">Save</button>
						<button onclick={() => showAddQuery = false} class="px-4 py-1.5 rounded-lg text-sm" style="background:{surface2}; border:1px solid {borderColor}; color:{textSecondary};">Cancel</button>
					</div>
				</div>
			{/if}

			{#if deleteQueryError}<div class="text-xs p-2 rounded mb-2" style="background:#EF4444; color:white;">{deleteQueryError}</div>{/if}

			{#if queryListLoading}
				<div class="text-sm" style="color:{textMuted};">Loading…</div>
			{:else if queryList.length === 0}
				<div class="text-sm" style="color:{textMuted};">No queries found.</div>
			{:else}
				<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor};">
					<div class="grid text-xs font-semibold px-4 py-2" style="grid-template-columns:2fr 3fr 1fr 1fr; background:{surface2}; color:{textMuted};">
						<span>Name</span><span>Description</span><span>Created By</span><span></span>
					</div>
					{#each queryList as q}
						<div class="grid items-center px-4 py-3 text-sm" style="grid-template-columns:2fr 3fr 1fr 1fr; border-top:1px solid {borderColor}; background:{cardBg}; color:{textPrimary};">
							<span class="font-medium">{q.name}</span>
							<span class="truncate" style="color:{textSecondary};">{q.description}</span>
							<span style="color:{textMuted};">{q.created_by}</span>
							<button onclick={() => deleteQuery(q.id)} class="text-xs px-2 py-1 rounded" style="background:#EF444420; color:#EF4444; border:none;">Delete</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
