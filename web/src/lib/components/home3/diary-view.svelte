<script lang="ts">
	import { onMount } from 'svelte';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import CheckIcon from '@lucide/svelte/icons/check';
	import XIcon from '@lucide/svelte/icons/x';
	import Link2Icon from '@lucide/svelte/icons/link-2';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import TagIcon from '@lucide/svelte/icons/tag';
	import RadioIcon from '@lucide/svelte/icons/radio';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import BookMarkedIcon from '@lucide/svelte/icons/book-marked';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import LoaderIcon from '@lucide/svelte/icons/loader';

	let { darkMode = true }: { darkMode: boolean } = $props();

	interface DiaryEntry {
		id: string;
		createdAt: string;
		topic: string;
		urls: string[];
		keywords: string[];
		source: string;
		content: string;
	}

	// Design tokens — mirrors home3 palette
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let surface3      = $derived(darkMode ? '#2C3246' : '#E0E3EA');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.12)' : 'rgba(99,102,241,0.08)');
	let accentBright  = $derived(darkMode ? 'rgba(129,140,248,0.22)' : 'rgba(99,102,241,0.18)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg       = $derived(darkMode ? '#161B28' : '#F9FAFB');
	let danger        = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint    = $derived(darkMode ? 'rgba(248,113,113,0.12)' : 'rgba(220,38,38,0.06)');
	let successColor  = $derived(darkMode ? '#34D399' : '#059669');

	// --- State ---
	let entries     = $state<DiaryEntry[]>([]);
	let loading     = $state(false);
	let saving      = $state(false);
	let error       = $state('');
	let selectedId  = $state<string | null>(null);
	let isEditing   = $state(false);

	// Form fields
	let formTopic    = $state('');
	let formUrls     = $state<string[]>(['']);
	let formSource   = $state('');
	let formContent  = $state('');
	let formKeywords = $state<string[]>([]);
	let kwInput      = $state('');

	// Collapsed date groups in the list
	let collapsed = $state<Record<string, boolean>>({});

	// --- Derived ---
	let selectedEntry = $derived(entries.find((e) => e.id === selectedId) ?? null);
	let isNew         = $derived(selectedId === '__new__');
	let isFormMode    = $derived(isNew || isEditing);

	interface DayGroup  { key: string; label: string; entries: DiaryEntry[] }
	interface MonthGroup { key: string; label: string; days: DayGroup[] }
	interface YearGroup  { year: string; months: MonthGroup[] }

	const MONTHS = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];

	let grouped: YearGroup[] = $derived((() => {
		const yearMap = new Map<string, Map<string, Map<string, DiaryEntry[]>>>();
		for (const e of entries) {
			const d  = new Date(e.createdAt);
			const yr = d.getFullYear().toString();
			const mo = String(d.getMonth() + 1).padStart(2, '0');
			const dy = String(d.getDate()).padStart(2, '0');
			if (!yearMap.has(yr)) yearMap.set(yr, new Map());
			const mMap = yearMap.get(yr)!;
			if (!mMap.has(mo)) mMap.set(mo, new Map());
			const dMap = mMap.get(mo)!;
			if (!dMap.has(dy)) dMap.set(dy, []);
			dMap.get(dy)!.push(e);
		}

		const result: YearGroup[] = [];
		for (const [yr, mMap] of [...yearMap].sort((a, b) => b[0].localeCompare(a[0]))) {
			const months: MonthGroup[] = [];
			for (const [mo, dMap] of [...mMap].sort((a, b) => b[0].localeCompare(a[0]))) {
				const days: DayGroup[] = [];
				for (const [dy, dayEntries] of [...dMap].sort((a, b) => b[0].localeCompare(a[0]))) {
					const dt = new Date(parseInt(yr), parseInt(mo) - 1, parseInt(dy));
					days.push({
						key: `${yr}-${mo}-${dy}`,
						label: dt.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' }),
						entries: dayEntries.sort(
							(a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
						)
					});
				}
				months.push({ key: `${yr}-${mo}`, label: `${MONTHS[parseInt(mo)-1]} ${yr}`, days });
			}
			result.push({ year: yr, months });
		}
		return result;
	})());

	// --- API ---
	async function loadEntries() {
		loading = true;
		error = '';
		try {
			const res = await fetch('/api/v1/diary');
			if (!res.ok) throw new Error(`Server error: ${res.status}`);
			entries = await res.json();
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function saveEntry() {
		saving = true;
		error = '';
		const payload = {
			topic:    formTopic.trim(),
			urls:     formUrls.map((u) => u.trim()).filter(Boolean),
			keywords: formKeywords,
			source:   formSource.trim(),
			content:  formContent
		};
		try {
			if (isNew) {
				const res = await fetch('/api/v1/diary', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(payload)
				});
				if (!res.ok) throw new Error(await res.text());
				const created: DiaryEntry = await res.json();
				await loadEntries();
				selectEntry(created);
				isEditing = false;
			} else if (selectedId) {
				const res = await fetch(`/api/v1/diary/${selectedId}`, {
					method: 'PUT',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(payload)
				});
				if (!res.ok) throw new Error(await res.text());
				const updated: DiaryEntry = await res.json();
				await loadEntries();
				selectEntry(updated);
				isEditing = false;
			}
		} catch (e: any) {
			error = e.message;
		} finally {
			saving = false;
		}
	}

	async function deleteEntry() {
		if (!selectedId || isNew) return;
		if (!confirm('Delete this diary entry? This cannot be undone.')) return;
		try {
			const res = await fetch(`/api/v1/diary/${selectedId}`, { method: 'DELETE' });
			if (!res.ok) throw new Error(await res.text());
			await loadEntries();
			resetForm();
		} catch (e: any) {
			error = e.message;
		}
	}

	// --- Form helpers ---
	function resetForm() {
		selectedId   = null;
		isEditing    = false;
		formTopic    = '';
		formUrls     = [''];
		formSource   = '';
		formContent  = '';
		formKeywords = [];
		kwInput      = '';
	}

	function startNew() {
		resetForm();
		selectedId = '__new__';
	}

	function selectEntry(entry: DiaryEntry) {
		selectedId   = entry.id;
		isEditing    = false;
		formTopic    = entry.topic;
		formUrls     = entry.urls.length ? [...entry.urls] : [''];
		formSource   = entry.source;
		formContent  = entry.content;
		formKeywords = [...entry.keywords];
		kwInput      = '';
	}

	function addUrlField()    { formUrls = [...formUrls, '']; }
	function removeUrl(i: number) {
		formUrls = formUrls.filter((_, idx) => idx !== i);
		if (formUrls.length === 0) formUrls = [''];
	}

	function handleKwKey(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			commitKw();
		} else if (e.key === 'Backspace' && kwInput === '' && formKeywords.length > 0) {
			formKeywords = formKeywords.slice(0, -1);
		}
	}

	function commitKw() {
		const kw = kwInput.replace(/,/g, '').trim();
		if (kw && !formKeywords.includes(kw)) formKeywords = [...formKeywords, kw];
		kwInput = '';
	}

	function removeKw(kw: string) {
		formKeywords = formKeywords.filter((k) => k !== kw);
	}

	function toggleGroup(key: string) {
		collapsed[key] = !collapsed[key];
	}

	function formatTime(iso: string): string {
		return new Date(iso).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
	}

	function formatDate(iso: string): string {
		return new Date(iso).toLocaleDateString('en-US', {
			weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'
		});
	}

	onMount(loadEntries);
</script>

<!-- Root: two-column layout filling the content panel -->
<div
	class="flex overflow-hidden"
	style="height: calc(100vh - 260px); min-height: 520px;"
>
	<!-- ── Left: Entry List ── -->
	<aside
		class="flex-shrink-0 flex flex-col overflow-hidden"
		style="width: 276px; background: {surface2}; border-right: 1px solid {borderColor};"
	>
		<!-- Header -->
		<div
			class="flex items-center justify-between px-4 flex-shrink-0"
			style="height: 52px; border-bottom: 1px solid {borderColor};"
		>
			<span style="font-size: 13px; font-weight: 600; color: {accent}; letter-spacing: 0.02em;">
				Diary
			</span>
			<button
				onclick={startNew}
				class="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 cursor-pointer transition-opacity duration-150"
				style="background: {accent}; color: #fff; font-size: 12px; font-weight: 600; border: none;"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.85'; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
				title="New diary entry"
			>
				<PlusIcon style="width: 13px; height: 13px;" />
				New
			</button>
		</div>

		<!-- List body -->
		<div class="flex-1 overflow-y-auto" style="scrollbar-width: thin; scrollbar-color: {borderColor} transparent;">
			{#if loading && entries.length === 0}
				<div class="flex items-center justify-center p-8">
					<LoaderIcon style="width: 20px; height: 20px; color: {textMuted}; animation: spin 1s linear infinite;" />
				</div>
			{:else if entries.length === 0}
				<div class="p-6 text-center">
					<BookMarkedIcon style="width: 32px; height: 32px; color: {textMuted}; opacity: 0.5; margin: 0 auto 8px;" />
					<p style="font-size: 13px; color: {textMuted};">No entries yet.</p>
					<p style="font-size: 12px; color: {textMuted}; margin-top: 4px;">Click New to start.</p>
				</div>
			{:else}
				{#each grouped as yearGroup (yearGroup.year)}
					{#each yearGroup.months as month (month.key)}
						<!-- Month header -->
						<button
							onclick={() => toggleGroup(month.key)}
							class="flex items-center gap-2 w-full px-4 py-2 cursor-pointer"
							style="background: {surface3}; border-bottom: 1px solid {borderColor}; text-align: left;"
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.85'; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
						>
							{#if collapsed[month.key]}
								<ChevronRightIcon style="width: 12px; height: 12px; color: {textMuted}; flex-shrink: 0;" />
							{:else}
								<ChevronDownIcon style="width: 12px; height: 12px; color: {textMuted}; flex-shrink: 0;" />
							{/if}
							<span style="font-size: 11px; font-weight: 700; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.06em;">
								{month.label}
							</span>
						</button>

						{#if !collapsed[month.key]}
							{#each month.days as day (day.key)}
								<!-- Day label -->
								<div
									class="px-4 pt-2 pb-0.5"
									style="font-size: 11px; font-weight: 600; color: {textMuted}; letter-spacing: 0.03em;"
								>
									{day.label}
								</div>
								{#each day.entries as entry (entry.id)}
									<button
										onclick={() => selectEntry(entry)}
										class="flex flex-col w-full px-4 py-2.5 cursor-pointer transition-colors duration-100"
										style="
											text-align: left;
											background: {selectedId === entry.id ? accentTint : 'transparent'};
											border-left: 2px solid {selectedId === entry.id ? accent : 'transparent'};
										"
										onmouseenter={(e) => {
											const el = e.currentTarget as HTMLElement;
											if (selectedId !== entry.id) el.style.background = surface3;
										}}
										onmouseleave={(e) => {
											const el = e.currentTarget as HTMLElement;
											if (selectedId !== entry.id) el.style.background = 'transparent';
										}}
									>
										<span
											class="block truncate"
											style="font-size: 13px; font-weight: 500; color: {selectedId === entry.id ? accent : textPrimary};"
										>
											{entry.topic || '(no topic)'}
										</span>
										<div class="flex items-center gap-2 mt-0.5">
											<span style="font-size: 11px; color: {textMuted};">{formatTime(entry.createdAt)}</span>
											{#if entry.source}
												<span
													class="truncate"
													style="font-size: 11px; color: {textMuted}; max-width: 120px;"
												>· {entry.source}</span>
											{/if}
										</div>
									</button>
								{/each}
							{/each}
						{/if}
					{/each}
				{/each}
			{/if}
		</div>
	</aside>

	<!-- ── Right: Detail / Form ── -->
	<div
		class="flex-1 overflow-y-auto flex flex-col"
		style="background: {pageBg}; scrollbar-width: thin; scrollbar-color: {borderColor} transparent;"
	>
		{#if !selectedId}
			<!-- Empty state -->
			<div class="flex flex-col items-center justify-center flex-1 p-12">
				<BookMarkedIcon style="width: 48px; height: 48px; color: {accent}; opacity: 0.35; margin-bottom: 16px;" />
				<p style="font-size: 16px; font-weight: 500; color: {textSecondary}; margin-bottom: 6px;">
					Select an entry or create a new one
				</p>
				<p style="font-size: 13px; color: {textMuted}; margin-bottom: 20px;">
					Capture topics, links, and notes as you research.
				</p>
				<button
					onclick={startNew}
					class="flex items-center gap-2 rounded-lg px-4 py-2 cursor-pointer"
					style="background: {accent}; color: #fff; font-size: 13px; font-weight: 600; border: none;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.85'; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
				>
					<PlusIcon style="width: 15px; height: 15px;" />
					New Entry
				</button>
			</div>

		{:else if isFormMode}
			<!-- ── Entry Form (new or edit) ── -->
			<div class="flex flex-col flex-1 p-6 gap-5" style="max-width: 800px;">
				<!-- Form header -->
				<div class="flex items-center justify-between">
					<h2 style="font-size: 17px; font-weight: 600; color: {textPrimary};">
						{isNew ? 'New Entry' : 'Edit Entry'}
					</h2>
					<div class="flex items-center gap-2">
						{#if !isNew}
							<button
								onclick={() => { isEditing = false; selectedEntry && selectEntry(selectedEntry); }}
								class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer transition-opacity duration-150"
								style="background: {surface2}; color: {textSecondary}; font-size: 12px; font-weight: 500; border: 1px solid {borderColor};"
								onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.75'; }}
								onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
							>
								<XIcon style="width: 13px; height: 13px;" />
								Cancel
							</button>
						{/if}
						<button
							onclick={saveEntry}
							disabled={saving}
							class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer transition-opacity duration-150"
							style="background: {accent}; color: #fff; font-size: 12px; font-weight: 600; border: none; opacity: {saving ? 0.6 : 1};"
							onmouseenter={(e) => { if (!saving) (e.currentTarget as HTMLElement).style.opacity = '0.85'; }}
							onmouseleave={(e) => { if (!saving) (e.currentTarget as HTMLElement).style.opacity = '1'; }}
						>
							{#if saving}
								<LoaderIcon style="width: 13px; height: 13px; animation: spin 1s linear infinite;" />
								Saving…
							{:else}
								<CheckIcon style="width: 13px; height: 13px;" />
								Save
							{/if}
						</button>
					</div>
				</div>

				{#if error}
					<div
						class="flex items-center gap-2 rounded-lg px-4 py-3"
						style="background: {dangerTint}; border: 1px solid {danger}40; color: {danger}; font-size: 13px;"
					>
						<AlertCircleIcon style="width: 15px; height: 15px; flex-shrink: 0;" />
						{error}
					</div>
				{/if}

				<!-- Topic -->
				<div class="flex flex-col gap-1.5">
					<label style="font-size: 12px; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;">
						Topic
					</label>
					<input
						type="text"
						bind:value={formTopic}
						placeholder="What is this about?"
						style="
							background: {inputBg};
							border: 1px solid {borderColor};
							color: {textPrimary};
							border-radius: 8px;
							padding: 9px 12px;
							font-size: 14px;
							outline: none;
							transition: border-color 150ms;
						"
						onfocus={(e) => { (e.target as HTMLInputElement).style.borderColor = accent; }}
						onblur={(e) => { (e.target as HTMLInputElement).style.borderColor = borderColor; }}
					/>
				</div>

				<!-- Source -->
				<div class="flex flex-col gap-1.5">
					<label
						class="flex items-center gap-1.5"
						style="font-size: 12px; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;"
					>
						<RadioIcon style="width: 13px; height: 13px;" />
						Source
					</label>
					<input
						type="text"
						bind:value={formSource}
						placeholder="Where did you hear about this? (e.g., HN, Twitter, colleague)"
						style="
							background: {inputBg};
							border: 1px solid {borderColor};
							color: {textPrimary};
							border-radius: 8px;
							padding: 9px 12px;
							font-size: 14px;
							outline: none;
							transition: border-color 150ms;
						"
						onfocus={(e) => { (e.target as HTMLInputElement).style.borderColor = accent; }}
						onblur={(e) => { (e.target as HTMLInputElement).style.borderColor = borderColor; }}
					/>
				</div>

				<!-- URLs -->
				<div class="flex flex-col gap-1.5">
					<div class="flex items-center justify-between">
						<label
							class="flex items-center gap-1.5"
							style="font-size: 12px; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;"
						>
							<Link2Icon style="width: 13px; height: 13px;" />
							URLs
						</label>
						<button
							onclick={addUrlField}
							class="flex items-center gap-1 cursor-pointer rounded-md px-2 py-1 transition-colors duration-100"
							style="font-size: 11px; color: {accent}; background: {accentTint}; border: none;"
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = accentBright; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = accentTint; }}
						>
							<PlusIcon style="width: 11px; height: 11px;" />
							Add
						</button>
					</div>
					<div class="flex flex-col gap-2">
						{#each formUrls as url, i (i)}
							<div class="flex items-center gap-2">
								<input
									type="url"
									bind:value={formUrls[i]}
									placeholder="https://"
									style="
										flex: 1;
										background: {inputBg};
										border: 1px solid {borderColor};
										color: {textPrimary};
										border-radius: 8px;
										padding: 8px 12px;
										font-size: 13px;
										font-family: 'Fira Code', monospace;
										outline: none;
										transition: border-color 150ms;
									"
									onfocus={(e) => { (e.target as HTMLInputElement).style.borderColor = accent; }}
									onblur={(e) => { (e.target as HTMLInputElement).style.borderColor = borderColor; }}
								/>
								{#if formUrls.length > 1}
									<button
										onclick={() => removeUrl(i)}
										class="flex items-center justify-center w-7 h-7 rounded-md cursor-pointer transition-colors duration-100 flex-shrink-0"
										style="color: {textMuted}; background: transparent; border: none;"
										onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = danger; (e.currentTarget as HTMLElement).style.background = dangerTint; }}
										onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
									>
										<XIcon style="width: 14px; height: 14px;" />
									</button>
								{/if}
							</div>
						{/each}
					</div>
				</div>

				<!-- Keywords -->
				<div class="flex flex-col gap-1.5">
					<label
						class="flex items-center gap-1.5"
						style="font-size: 12px; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;"
					>
						<TagIcon style="width: 13px; height: 13px;" />
						Keywords
					</label>
					<!-- Chip input container -->
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<!-- svelte-ignore a11y_no_static_element_interactions -->
					<div
						class="flex flex-wrap items-center gap-1.5 rounded-lg px-3 py-2 cursor-text"
						style="
							background: {inputBg};
							border: 1px solid {borderColor};
							min-height: 42px;
							transition: border-color 150ms;
						"
						onclick={(e) => {
							const input = (e.currentTarget as HTMLElement).querySelector('input');
							input?.focus();
						}}
						onfocusin={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
						onfocusout={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; commitKw(); }}
					>
						{#each formKeywords as kw (kw)}
							<span
								class="flex items-center gap-1 rounded-md px-2 py-0.5"
								style="background: {accentTint}; color: {accent}; font-size: 12px; font-weight: 500; border: 1px solid {accent}25;"
							>
								{kw}
								<button
									onclick={() => removeKw(kw)}
									style="display: flex; align-items: center; background: none; border: none; padding: 0; cursor: pointer; color: {accent}; opacity: 0.7;"
									onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
									onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.7'; }}
								>
									<XIcon style="width: 11px; height: 11px;" />
								</button>
							</span>
						{/each}
						<input
							type="text"
							bind:value={kwInput}
							onkeydown={handleKwKey}
							placeholder={formKeywords.length === 0 ? 'Type keyword, press Enter or comma' : ''}
							style="
								flex: 1;
								min-width: 120px;
								background: transparent;
								border: none;
								outline: none;
								color: {textPrimary};
								font-size: 13px;
								padding: 2px 0;
							"
						/>
					</div>
				</div>

				<!-- Content -->
				<div class="flex flex-col gap-1.5 flex-1">
					<label style="font-size: 12px; font-weight: 600; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;">
						Notes
					</label>
					<textarea
						bind:value={formContent}
						placeholder="Write your notes here…"
						rows={12}
						style="
							flex: 1;
							background: {inputBg};
							border: 1px solid {borderColor};
							color: {textPrimary};
							border-radius: 8px;
							padding: 12px;
							font-size: 14px;
							line-height: 1.65;
							resize: vertical;
							outline: none;
							transition: border-color 150ms;
							font-family: inherit;
						"
						onfocus={(e) => { (e.target as HTMLTextAreaElement).style.borderColor = accent; }}
						onblur={(e) => { (e.target as HTMLTextAreaElement).style.borderColor = borderColor; }}
					></textarea>
				</div>
			</div>

		{:else if selectedEntry}
			<!-- ── Detail View ── -->
			<div class="flex flex-col flex-1 p-6 gap-5" style="max-width: 800px;">
				<!-- Detail header -->
				<div class="flex items-start justify-between gap-4">
					<div class="flex-1 min-w-0">
						<h2 style="font-size: 20px; font-weight: 600; color: {textPrimary}; line-height: 1.3; margin-bottom: 4px;">
							{selectedEntry.topic || '(no topic)'}
						</h2>
						<p style="font-size: 13px; color: {textMuted};">
							{formatDate(selectedEntry.createdAt)}
							{#if selectedEntry.source}
								<span style="color: {borderColor}; margin: 0 6px;">·</span>
								<span style="color: {textSecondary};">{selectedEntry.source}</span>
							{/if}
						</p>
					</div>
					<div class="flex items-center gap-2 flex-shrink-0">
						<button
							onclick={() => { isEditing = true; }}
							class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer transition-opacity duration-150"
							style="background: {surface2}; color: {textSecondary}; font-size: 12px; font-weight: 500; border: 1px solid {borderColor};"
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.75'; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
						>
							<PencilIcon style="width: 13px; height: 13px;" />
							Edit
						</button>
						<button
							onclick={deleteEntry}
							class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 cursor-pointer transition-colors duration-150"
							style="color: {danger}; background: transparent; font-size: 12px; font-weight: 500; border: 1px solid {borderColor};"
							onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = dangerTint; (e.currentTarget as HTMLElement).style.borderColor = danger + '50'; }}
							onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
						>
							<Trash2Icon style="width: 13px; height: 13px;" />
							Delete
						</button>
					</div>
				</div>

				<!-- Keywords -->
				{#if selectedEntry.keywords.length > 0}
					<div class="flex flex-wrap gap-1.5">
						{#each selectedEntry.keywords as kw (kw)}
							<span
								class="flex items-center gap-1 rounded-md px-2.5 py-1"
								style="background: {accentTint}; color: {accent}; font-size: 12px; font-weight: 500; border: 1px solid {accent}25;"
							>
								<TagIcon style="width: 11px; height: 11px;" />
								{kw}
							</span>
						{/each}
					</div>
				{/if}

				<!-- URLs -->
				{#if selectedEntry.urls.length > 0}
					<div
						class="rounded-xl p-4 flex flex-col gap-2"
						style="background: {cardBg}; border: 1px solid {borderColor};"
					>
						<div
							class="flex items-center gap-1.5 mb-1"
							style="font-size: 11px; font-weight: 700; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;"
						>
							<Link2Icon style="width: 12px; height: 12px;" />
							Links
						</div>
						{#each selectedEntry.urls as url (url)}
							<a
								href={url}
								target="_blank"
								rel="noopener noreferrer"
								class="flex items-center gap-2 rounded-lg px-3 py-2 transition-colors duration-100"
								style="
									color: {accent};
									font-size: 13px;
									font-family: 'Fira Code', monospace;
									text-decoration: none;
									background: {inputBg};
									border: 1px solid {borderColor};
									word-break: break-all;
								"
								onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent + '60'; }}
								onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
							>
								<ExternalLinkIcon style="width: 13px; height: 13px; flex-shrink: 0;" />
								{url}
							</a>
						{/each}
					</div>
				{/if}

				<!-- Content -->
				{#if selectedEntry.content}
					<div
						class="rounded-xl p-5 flex-1"
						style="background: {cardBg}; border: 1px solid {borderColor};"
					>
						<div
							class="mb-3"
							style="font-size: 11px; font-weight: 700; color: {textMuted}; text-transform: uppercase; letter-spacing: 0.05em;"
						>
							Notes
						</div>
						<p
							style="
								font-size: 14px;
								color: {textSecondary};
								line-height: 1.75;
								white-space: pre-wrap;
								word-break: break-word;
							"
						>
							{selectedEntry.content}
						</p>
					</div>
				{:else}
					<div
						class="rounded-xl p-8 flex items-center justify-center"
						style="background: {cardBg}; border: 1px solid {borderColor}; border-style: dashed;"
					>
						<div class="text-center">
							<p style="font-size: 14px; color: {textMuted};">No notes written.</p>
							<button
								onclick={() => { isEditing = true; }}
								style="margin-top: 8px; font-size: 12px; color: {accent}; background: none; border: none; cursor: pointer; text-decoration: underline; text-underline-offset: 3px;"
							>
								Add notes
							</button>
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<style>
	@keyframes spin {
		from { transform: rotate(0deg); }
		to   { transform: rotate(360deg); }
	}
</style>
