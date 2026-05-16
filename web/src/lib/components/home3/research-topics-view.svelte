<script lang="ts">
	import { onMount } from 'svelte';
	import PlusIcon         from '@lucide/svelte/icons/plus';
	import SearchIcon       from '@lucide/svelte/icons/search';
	import TagIcon          from '@lucide/svelte/icons/tag';
	import FileTextIcon     from '@lucide/svelte/icons/file-text';
	import BookOpenIcon     from '@lucide/svelte/icons/book-open';
	import PencilIcon       from '@lucide/svelte/icons/pencil';
	import LightbulbIcon    from '@lucide/svelte/icons/lightbulb';
	import CodeIcon         from '@lucide/svelte/icons/code-2';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import UsersIcon        from '@lucide/svelte/icons/users';
	import FolderIcon       from '@lucide/svelte/icons/folder';
	import XIcon            from '@lucide/svelte/icons/x';
	import LoaderIcon       from '@lucide/svelte/icons/loader';
	import AlertCircleIcon  from '@lucide/svelte/icons/alert-circle';

	let { darkMode = true }: { darkMode: boolean } = $props();

	// ── Design tokens ─────────────────────────────────────────────────────────
	let pageBg        = $derived(darkMode ? '#171B26' : '#F2F4F7');
	let cardBg        = $derived(darkMode ? '#1F2333' : '#FFFFFF');
	let surface2      = $derived(darkMode ? '#252A3A' : '#ECEEF2');
	let surface3      = $derived(darkMode ? '#2C3246' : '#E0E3EA');
	let borderColor   = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent        = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint    = $derived(darkMode ? 'rgba(129,140,248,0.12)' : 'rgba(99,102,241,0.08)');
	let textPrimary   = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');
	let textMuted     = $derived(darkMode ? '#64748B' : '#9CA3AF');
	let inputBg       = $derived(darkMode ? '#161B28' : '#F9FAFB');
	let danger        = $derived(darkMode ? '#F87171' : '#DC2626');
	let dangerTint    = $derived(darkMode ? 'rgba(248,113,113,0.10)' : 'rgba(220,38,38,0.06)');
	let overlayBg     = $derived(darkMode ? 'rgba(0,0,0,0.6)' : 'rgba(0,0,0,0.35)');

	// ── Constants ─────────────────────────────────────────────────────────────
	type BeanType = 'Thoughts' | 'Design' | 'Spec' | 'Implementation' | 'Reading';
	type FileType = 'Markdown' | 'Typst' | 'Text' | 'HTML';

	const BEAN_TYPES: BeanType[] = ['Thoughts', 'Design', 'Spec', 'Implementation', 'Reading'];
	const FILE_TYPES: FileType[] = ['Markdown', 'Typst', 'Text', 'HTML'];

	const beanTypeColors: Record<BeanType, { bg: string; text: string }> = {
		Thoughts:       { bg: 'rgba(251,191,36,0.15)',  text: '#FBBF24' },
		Design:         { bg: 'rgba(129,140,248,0.15)', text: '#818CF8' },
		Spec:           { bg: 'rgba(52,211,153,0.15)',  text: '#34D399' },
		Implementation: { bg: 'rgba(248,113,113,0.15)', text: '#F87171' },
		Reading:        { bg: 'rgba(56,189,248,0.15)',  text: '#38BDF8' }
	};

	const beanTypeIcons: Record<BeanType, any> = {
		Thoughts:       LightbulbIcon,
		Design:         PencilIcon,
		Spec:           FileTextIcon,
		Implementation: CodeIcon,
		Reading:        BookOpenIcon
	};

	const fileTypeExtension: Record<FileType, string> = {
		Markdown: '.md',
		Typst:    '.typ',
		Text:     '.text',
		HTML:     '.html'
	};

	// ── RTB type ──────────────────────────────────────────────────────────────
	interface ResearchTopicBean {
		bean_name: string;
		bean_desc?: string;
		bean_keywords?: string[];
		bean_type: BeanType;
		related_topics?: string[];
		research_title: string;
		research_subtitle: string;
		file_type: FileType;
		bean_category: string;
		authors?: string[];
		content?: string;
		created_at?: string;
	}

	// ── List state ────────────────────────────────────────────────────────────
	let beans        = $state<ResearchTopicBean[]>([]);
	let loading      = $state(false);
	let loadError    = $state('');
	let selectedBean = $state<ResearchTopicBean | null>(null);
	let searchQuery  = $state('');
	let activeTypeFilter = $state<BeanType | 'All'>('All');

	let filteredBeans = $derived(
		beans.filter(b => {
			const matchesType = activeTypeFilter === 'All' || b.bean_type === activeTypeFilter;
			const q = searchQuery.toLowerCase();
			const matchesSearch = !q ||
				b.bean_name.toLowerCase().includes(q) ||
				b.research_title.toLowerCase().includes(q) ||
				(b.bean_desc ?? '').toLowerCase().includes(q) ||
				(b.bean_keywords ?? []).some(k => k.toLowerCase().includes(q));
			return matchesType && matchesSearch;
		})
	);

	async function loadBeans() {
		loading = true;
		loadError = '';
		try {
			const res = await fetch('/api/v1/ke/research-topics');
			if (!res.ok) throw new Error(`Server error: ${res.status}`);
			beans = await res.json();
		} catch (e: any) {
			loadError = e.message ?? 'Failed to load beans';
		} finally {
			loading = false;
		}
	}

	onMount(loadBeans);

	// ── New RTB modal state ───────────────────────────────────────────────────
	let modalOpen   = $state(false);
	let submitting  = $state(false);
	let submitError = $state('');

	// Form fields
	let fBeanName        = $state('');
	let fResearchTitle   = $state('');
	let fResearchSubtitle = $state('');
	let fBeanType        = $state<BeanType>('Thoughts');
	let fFileType        = $state<FileType>('Markdown');
	let fBeanCategory    = $state('');
	let fBeanDesc        = $state('');
	let fKeywordsRaw     = $state('');   // comma-separated input
	let fAuthorsRaw      = $state('');
	let fRelatedRaw      = $state('');
	let fContent         = $state('');

	function openModal() {
		fBeanName = '';
		fResearchTitle = '';
		fResearchSubtitle = '';
		fBeanType = 'Thoughts';
		fFileType = 'Markdown';
		fBeanCategory = '';
		fBeanDesc = '';
		fKeywordsRaw = '';
		fAuthorsRaw = '';
		fRelatedRaw = '';
		fContent = '';
		submitError = '';
		modalOpen = true;
	}

	function closeModal() {
		if (!submitting) modalOpen = false;
	}

	function splitTrimmed(raw: string): string[] {
		return raw.split(',').map(s => s.trim()).filter(Boolean);
	}

	async function handleCreate() {
		submitError = '';
		submitting = true;
		try {
			const payload: ResearchTopicBean = {
				bean_name:         fBeanName.trim(),
				research_title:    fResearchTitle.trim(),
				research_subtitle: fResearchSubtitle.trim(),
				bean_type:         fBeanType,
				file_type:         fFileType,
				bean_category:     fBeanCategory.trim(),
				bean_desc:         fBeanDesc.trim() || undefined,
				bean_keywords:     splitTrimmed(fKeywordsRaw),
				authors:           splitTrimmed(fAuthorsRaw),
				related_topics:    splitTrimmed(fRelatedRaw),
				content:           fContent,
			};

			const res = await fetch('/api/v1/ke/research-topics', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload),
			});

			if (!res.ok) {
				const body = await res.json().catch(() => ({}));
				throw new Error(body.error ?? `Server error: ${res.status}`);
			}

			const created: ResearchTopicBean = await res.json();
			beans = [created, ...beans];
			modalOpen = false;
		} catch (e: any) {
			submitError = e.message ?? 'Failed to create bean';
		} finally {
			submitting = false;
		}
	}
</script>

<!-- ── Root ──────────────────────────────────────────────────────────────── -->
<div class="flex flex-col h-full overflow-hidden" style="background:{pageBg};">

	<!-- Header -->
	<div class="flex-shrink-0 px-6 pt-6 pb-4">
		<div class="flex items-center justify-between mb-4">
			<div>
				<h1 style="font-size:20px; font-weight:700; color:{textPrimary}; margin-bottom:2px;">
					Research Topics
				</h1>
				<p style="font-size:13px; color:{textSecondary};">
					Research Topic Beans — articles, thoughts, designs, specs, and readings
				</p>
			</div>
			<button
				onclick={openModal}
				class="flex items-center gap-2 rounded-lg px-4 py-2 cursor-pointer transition-opacity duration-150"
				style="background:{accent}; color:white; font-size:13px; font-weight:600; border:none;"
				onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.opacity = '0.85'; }}
				onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.opacity = '1'; }}
			>
				<PlusIcon style="width:15px; height:15px;" />
				New RTB
			</button>
		</div>

		<!-- Search + type filters -->
		<div class="flex items-center gap-3 flex-wrap">
			<div
				class="flex items-center gap-2 rounded-lg px-3 flex-1 min-w-48"
				style="background:{inputBg}; border:1px solid {borderColor}; height:36px;"
			>
				<SearchIcon style="width:14px; height:14px; color:{textMuted}; flex-shrink:0;" />
				<input
					type="text"
					placeholder="Search beans…"
					bind:value={searchQuery}
					style="background:transparent; border:none; outline:none; font-size:13px; color:{textPrimary}; width:100%;"
				/>
			</div>
			<div class="flex items-center gap-1.5">
				{#each ['All', ...BEAN_TYPES] as t}
					{@const isActive = activeTypeFilter === t}
					<button
						onclick={() => { activeTypeFilter = t as BeanType | 'All'; }}
						class="rounded-full px-3 py-1 text-xs font-medium cursor-pointer transition-colors duration-150"
						style="border:1px solid {isActive ? accent : borderColor}; background:{isActive ? accentTint : 'transparent'}; color:{isActive ? accent : textMuted};"
					>
						{t}
					</button>
				{/each}
			</div>
		</div>
	</div>

	<!-- Body -->
	<div class="flex flex-1 min-h-0">

		<!-- Bean list -->
		<div
			class="flex flex-col overflow-y-auto px-6 pb-6"
			style="width:{selectedBean ? '420px' : '100%'}; flex-shrink:0; scrollbar-width:thin; scrollbar-color:{borderColor} transparent; transition:width 200ms ease;"
		>
			{#if loading}
				<div class="flex items-center justify-center py-16">
					<LoaderIcon style="width:24px; height:24px; color:{accent}; animation:spin 1s linear infinite;" />
				</div>
			{:else if loadError}
				<div
					class="flex items-center gap-3 rounded-xl p-4 mt-2"
					style="background:{dangerTint}; border:1px solid {danger}40;"
				>
					<AlertCircleIcon style="width:16px; height:16px; color:{danger}; flex-shrink:0;" />
					<span style="font-size:13px; color:{danger};">{loadError}</span>
					<button
						onclick={loadBeans}
						class="ml-auto text-xs px-3 py-1 rounded-lg cursor-pointer"
						style="background:{dangerTint}; color:{danger}; border:1px solid {danger}40;"
					>Retry</button>
				</div>
			{:else if filteredBeans.length === 0}
				<div
					class="flex flex-col items-center justify-center rounded-xl p-12 mt-2"
					style="background:{cardBg}; border:1px solid {borderColor};"
				>
					<BookOpenIcon style="width:40px; height:40px; color:{accent}; opacity:0.3; margin-bottom:12px;" />
					<p style="font-size:15px; font-weight:500; color:{textSecondary};">
						{beans.length === 0 ? 'No beans yet' : 'No beans found'}
					</p>
					<p style="font-size:13px; color:{textMuted}; margin-top:4px;">
						{beans.length === 0 ? 'Click "New RTB" to create your first research topic bean.' : 'Try adjusting your search or filter.'}
					</p>
				</div>
			{:else}
				<div class="flex flex-col gap-3 mt-2">
					{#each filteredBeans as bean (bean.bean_name + bean.bean_category)}
						{@const typeConf = beanTypeColors[bean.bean_type]}
						{@const TypeIcon = beanTypeIcons[bean.bean_type]}
						{@const isSelected = selectedBean?.bean_name === bean.bean_name && selectedBean?.bean_category === bean.bean_category}
						<button
							onclick={() => { selectedBean = isSelected ? null : bean; }}
							class="text-left rounded-xl p-4 cursor-pointer transition-colors duration-150 w-full"
							style="background:{isSelected ? surface2 : cardBg}; border:1px solid {isSelected ? accent + '60' : borderColor};"
							onmouseenter={(e) => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = surface2; }}
							onmouseleave={(e) => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = cardBg; }}
						>
							<div class="flex items-start gap-3">
								<div class="flex items-center justify-center rounded-lg flex-shrink-0" style="width:36px; height:36px; background:{typeConf.bg};">
									<TypeIcon style="width:18px; height:18px; color:{typeConf.text};" />
								</div>
								<div class="flex-1 min-w-0">
									<div class="flex items-center gap-2 flex-wrap mb-0.5">
										<span style="font-size:14px; font-weight:600; color:{textPrimary};">{bean.research_title}</span>
										<span class="rounded-full px-2 py-0.5 text-xs font-medium flex-shrink-0" style="background:{typeConf.bg}; color:{typeConf.text};">{bean.bean_type}</span>
										<span class="rounded-md px-1.5 py-0.5 text-xs font-mono flex-shrink-0" style="background:{surface3}; color:{textMuted};">{fileTypeExtension[bean.file_type]}</span>
									</div>
									<p style="font-size:12px; color:{textSecondary}; margin-bottom:6px;">{bean.research_subtitle}</p>
									<div class="flex items-center gap-3 flex-wrap">
										<div class="flex items-center gap-1">
											<FolderIcon style="width:11px; height:11px; color:{textMuted};" />
											<span style="font-size:11px; color:{textMuted}; font-family:monospace;">{bean.bean_category}</span>
										</div>
										{#if bean.authors?.length}
											<div class="flex items-center gap-1">
												<UsersIcon style="width:11px; height:11px; color:{textMuted};" />
												<span style="font-size:11px; color:{textMuted};">{bean.authors.join(', ')}</span>
											</div>
										{/if}
									</div>
									{#if bean.bean_keywords?.length}
										<div class="flex items-center gap-1.5 flex-wrap mt-2">
											<TagIcon style="width:10px; height:10px; color:{textMuted}; flex-shrink:0;" />
											{#each (bean.bean_keywords ?? []) as kw}
												<span class="rounded-md px-1.5 py-0.5 text-xs" style="background:{surface3}; color:{textSecondary};">{kw}</span>
											{/each}
										</div>
									{/if}
								</div>
								<ChevronRightIcon style="width:14px; height:14px; color:{textMuted}; flex-shrink:0; transform:rotate({isSelected ? '90deg' : '0deg'}); transition:transform 200ms ease;" />
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Detail panel -->
		{#if selectedBean}
			{@const bean = selectedBean}
			{@const typeConf = beanTypeColors[bean.bean_type]}
			{@const TypeIcon = beanTypeIcons[bean.bean_type]}
			<div
				class="flex-1 overflow-y-auto border-l p-6"
				style="background:{surface2}; border-color:{borderColor}; scrollbar-width:thin; scrollbar-color:{borderColor} transparent;"
			>
				<div class="flex items-start gap-3 mb-5">
					<div class="flex items-center justify-center rounded-xl flex-shrink-0" style="width:44px; height:44px; background:{typeConf.bg};">
						<TypeIcon style="width:22px; height:22px; color:{typeConf.text};" />
					</div>
					<div class="flex-1 min-w-0">
						<h2 style="font-size:16px; font-weight:700; color:{textPrimary}; margin-bottom:2px;">{bean.research_title}</h2>
						<p style="font-size:13px; color:{textSecondary};">{bean.research_subtitle}</p>
					</div>
				</div>

				<div class="rounded-xl p-4 mb-4 grid gap-3" style="background:{cardBg}; border:1px solid {borderColor}; grid-template-columns:1fr 1fr;">
					{#each [
						{ label: 'Bean Name',  value: bean.bean_name,      mono: true },
						{ label: 'Bean Type',  value: bean.bean_type,      mono: false },
						{ label: 'File Type',  value: bean.file_type + ' (' + fileTypeExtension[bean.file_type] + ')', mono: false },
						{ label: 'Category',   value: bean.bean_category,  mono: true },
						{ label: 'Authors',    value: (bean.authors ?? []).join(', ') || '—', mono: false },
					] as row}
						<div>
							<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.05em; margin-bottom:2px;">{row.label}</div>
							<div style="font-size:13px; color:{textPrimary}; {row.mono ? 'font-family:monospace;' : ''}">{row.value}</div>
						</div>
					{/each}
				</div>

				{#if bean.bean_desc}
					<div class="rounded-xl p-4 mb-4" style="background:{cardBg}; border:1px solid {borderColor};">
						<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.05em; margin-bottom:6px;">Description</div>
						<p style="font-size:13px; color:{textSecondary}; line-height:1.6;">{bean.bean_desc}</p>
					</div>
				{/if}

				{#if bean.bean_keywords?.length}
					<div class="rounded-xl p-4 mb-4" style="background:{cardBg}; border:1px solid {borderColor};">
						<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.05em; margin-bottom:8px;">Keywords</div>
						<div class="flex flex-wrap gap-2">
							{#each (bean.bean_keywords ?? []) as kw}
								<span class="rounded-full px-3 py-1 text-xs" style="background:{accentTint}; color:{accent}; border:1px solid {accent}30;">{kw}</span>
							{/each}
						</div>
					</div>
				{/if}

				{#if bean.related_topics?.length}
					<div class="rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
						<div style="font-size:11px; font-weight:600; color:{textMuted}; text-transform:uppercase; letter-spacing:0.05em; margin-bottom:8px;">Related Topics</div>
						<div class="flex flex-col gap-1.5">
							{#each (bean.related_topics ?? []) as rel}
								<div class="flex items-center gap-2 rounded-lg px-3 py-2" style="background:{surface3}; font-size:13px; color:{textSecondary}; font-family:monospace;">
									<ChevronRightIcon style="width:12px; height:12px; color:{textMuted};" />
									{rel}
								</div>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</div>

<!-- ── New RTB Modal ──────────────────────────────────────────────────────── -->
{#if modalOpen}
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 flex items-center justify-center z-50"
		style="background:{overlayBg};"
		onclick={(e) => { if (e.target === e.currentTarget) closeModal(); }}
	>
		<div
			class="flex flex-col rounded-2xl shadow-2xl w-full max-w-2xl mx-4 max-h-[90vh] overflow-hidden"
			style="background:{cardBg}; border:1px solid {borderColor};"
		>
			<!-- Modal header -->
			<div class="flex items-center justify-between px-6 py-4 flex-shrink-0" style="border-bottom:1px solid {borderColor};">
				<h2 style="font-size:16px; font-weight:700; color:{textPrimary};">New Research Topic Bean</h2>
				<button
					onclick={closeModal}
					disabled={submitting}
					class="flex items-center justify-center w-7 h-7 rounded-lg cursor-pointer"
					style="color:{textMuted}; background:transparent; border:none;"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.color = textPrimary; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.color = textMuted; }}
				>
					<XIcon style="width:16px; height:16px;" />
				</button>
			</div>

			<!-- Modal body -->
			<div class="overflow-y-auto px-6 py-5 flex flex-col gap-4" style="scrollbar-width:thin; scrollbar-color:{borderColor} transparent;">

				<!-- Research Title -->
				<div>
					<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
						Research Title <span style="color:{accent};">*</span>
					</label>
					<input
						type="text"
						bind:value={fResearchTitle}
						placeholder="e.g. Document Chunking Strategy"
						style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none;"
						onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
						onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
					/>
				</div>

				<!-- Research Subtitle -->
				<div>
					<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
						Research Subtitle <span style="color:{accent};">*</span>
					</label>
					<input
						type="text"
						bind:value={fResearchSubtitle}
						placeholder="e.g. Specification for Semantic and Structural Chunking Approaches"
						style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none;"
						onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
						onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
					/>
				</div>

				<!-- Bean Name + Bean Type (2-col) -->
				<div class="grid gap-4" style="grid-template-columns:1fr 1fr;">
					<div>
						<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
							Bean Name <span style="color:{accent};">*</span>
						</label>
						<input
							type="text"
							bind:value={fBeanName}
							placeholder="e.g. chunking-strategy-spec"
							style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none; font-family:monospace;"
							onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
							onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
						/>
						<p style="font-size:11px; color:{textMuted}; margin-top:3px;">Letters, digits, hyphens, underscores only</p>
					</div>
					<div>
						<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
							Bean Type <span style="color:{accent};">*</span>
						</label>
						<select
							bind:value={fBeanType}
							style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none; cursor:pointer;"
						>
							{#each BEAN_TYPES as t}
								<option value={t}>{t}</option>
							{/each}
						</select>
					</div>
				</div>

				<!-- File Type + Bean Category (2-col) -->
				<div class="grid gap-4" style="grid-template-columns:1fr 1fr;">
					<div>
						<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
							File Type <span style="color:{accent};">*</span>
						</label>
						<select
							bind:value={fFileType}
							style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none; cursor:pointer;"
						>
							{#each FILE_TYPES as ft}
								<option value={ft}>{ft} ({fileTypeExtension[ft]})</option>
							{/each}
						</select>
					</div>
					<div>
						<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
							Bean Category <span style="color:{accent};">*</span>
						</label>
						<input
							type="text"
							bind:value={fBeanCategory}
							placeholder="e.g. knowledge-engineering/ingestion"
							style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none; font-family:monospace;"
							onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
							onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
						/>
						<p style="font-size:11px; color:{textMuted}; margin-top:3px;">Maps to directory path under Topics/</p>
					</div>
				</div>

				<!-- Description -->
				<div>
					<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">Description</label>
					<textarea
						bind:value={fBeanDesc}
						placeholder="Brief description of this research topic bean…"
						rows="2"
						style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none; resize:vertical;"
						onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
						onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
					></textarea>
				</div>

				<!-- Keywords / Authors / Related (3 compact rows) -->
				{#each [
					{ label: 'Keywords',       bind: 'keywords', placeholder: 'ontology, schema, RTB' },
					{ label: 'Authors',        bind: 'authors',  placeholder: 'Chen Ding' },
					{ label: 'Related Topics', bind: 'related',  placeholder: 'bean-name-a, bean-name-b' },
				] as row}
					<div>
						<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
							{row.label} <span style="font-weight:400;">(comma-separated)</span>
						</label>
						{#if row.bind === 'keywords'}
							<input type="text" bind:value={fKeywordsRaw} placeholder={row.placeholder}
								style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none;"
								onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
								onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }} />
						{:else if row.bind === 'authors'}
							<input type="text" bind:value={fAuthorsRaw} placeholder={row.placeholder}
								style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none;"
								onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
								onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }} />
						{:else}
							<input type="text" bind:value={fRelatedRaw} placeholder={row.placeholder}
								style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none;"
								onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
								onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }} />
						{/if}
					</div>
				{/each}

				<!-- Content -->
				<div>
					<label style="display:block; font-size:12px; font-weight:600; color:{textMuted}; margin-bottom:5px;">
						Content <span style="font-weight:400;">(written to the artifact file)</span>
					</label>
					<textarea
						bind:value={fContent}
						placeholder="Paste or write the bean's content here…"
						rows="5"
						style="width:100%; background:{inputBg}; border:1px solid {borderColor}; border-radius:8px; padding:8px 12px; font-size:13px; color:{textPrimary}; outline:none; resize:vertical; font-family:monospace;"
						onfocus={(e) => { (e.currentTarget as HTMLElement).style.borderColor = accent; }}
						onblur={(e) => { (e.currentTarget as HTMLElement).style.borderColor = borderColor; }}
					></textarea>
				</div>

				<!-- Error -->
				{#if submitError}
					<div class="flex items-center gap-2 rounded-lg px-4 py-3" style="background:{dangerTint}; border:1px solid {danger}40;">
						<AlertCircleIcon style="width:14px; height:14px; color:{danger}; flex-shrink:0;" />
						<span style="font-size:13px; color:{danger};">{submitError}</span>
					</div>
				{/if}
			</div>

			<!-- Modal footer -->
			<div class="flex items-center justify-end gap-3 px-6 py-4 flex-shrink-0" style="border-top:1px solid {borderColor};">
				<button
					onclick={closeModal}
					disabled={submitting}
					class="rounded-lg px-4 py-2 cursor-pointer text-sm font-medium transition-colors duration-150"
					style="background:transparent; color:{textSecondary}; border:1px solid {borderColor};"
					onmouseenter={(e) => { (e.currentTarget as HTMLElement).style.background = surface2; }}
					onmouseleave={(e) => { (e.currentTarget as HTMLElement).style.background = 'transparent'; }}
				>Cancel</button>
				<button
					onclick={handleCreate}
					disabled={submitting || !fBeanName.trim() || !fResearchTitle.trim() || !fResearchSubtitle.trim() || !fBeanCategory.trim()}
					class="flex items-center gap-2 rounded-lg px-5 py-2 cursor-pointer text-sm font-semibold transition-opacity duration-150"
					style="background:{accent}; color:white; border:none; opacity:{submitting || !fBeanName.trim() || !fResearchTitle.trim() || !fResearchSubtitle.trim() || !fBeanCategory.trim() ? '0.5' : '1'}; cursor:{submitting ? 'wait' : 'pointer'};"
				>
					{#if submitting}
						<LoaderIcon style="width:14px; height:14px; animation:spin 1s linear infinite;" />
						Creating…
					{:else}
						<PlusIcon style="width:14px; height:14px;" />
						Create Bean
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	@keyframes spin {
		from { transform: rotate(0deg); }
		to   { transform: rotate(360deg); }
	}
</style>
