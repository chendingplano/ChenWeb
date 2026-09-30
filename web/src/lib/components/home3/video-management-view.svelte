<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { onMount } from 'svelte';
	import {
		listVideos,
		uploadVideo,
		updateVideo,
		deleteVideo,
		videoStreamUrl,
		videoDownloadUrl,
		type VideoMeta,
		type VideoUploadFields
	} from '$lib/services/videoService';
	import { generateImage, type ImageMeta } from '$lib/services/imageService';
	import ImageLibraryPicker from '$lib/components/home3/image-library-picker.svelte';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (match the Dashboard app shell) ---
	let surface = $derived(darkMode ? '#1E2333' : '#FFFFFF');
	let cardBg = $derived(darkMode ? '#252A3A' : '#FFFFFF');
	let inputBg = $derived(darkMode ? '#171B26' : '#F9FAFB');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let dangerColor = $derived(darkMode ? '#F87171' : '#DC2626');
	let overlayBg = $derived(darkMode ? 'rgba(0,0,0,0.6)' : 'rgba(0,0,0,0.4)');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');

	let videos = $state<VideoMeta[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let selected = $state<VideoMeta | null>(null);

	// --- Sort / filter controls ---
	type SortField = 'name' | 'created_at' | 'size_bytes';
	type SortDirection = 'asc' | 'desc';
	const sortableColumns: { field: SortField; label: string }[] = [
		{ field: 'name', label: m.video_management_name() },
		{ field: 'size_bytes', label: m.video_management_size() },
		{ field: 'created_at', label: m.video_management_uploaded() }
	];
	let sorts = $state<{ field: SortField; direction: SortDirection }[]>([]);
	let filterName = $state('');
	let filterTimeFrom = $state('');
	let filterTimeTo = $state('');

	// --- Upload/edit dialog state ---
	let editingId = $state<number | null>(null);
	let dialogOpen = $state(false);
	let uploading = $state(false);
	let uploadProgress = $state(0);
	let generating = $state(false);
	let showPicker = $state(false);
	let ignoreDialogBackdropUntil = 0;
	let dialogError = $state<string | null>(null);

	let file = $state<File | null>(null);
	let currentFilename = $state(''); // edit mode: the file already stored, shown until replaced
	let name = $state('');
	let description = $state('');
	let source = $state('Recording');
	let url = $state('');
	let coverImage = $state<ImageMeta | null>(null);
	let dialogFileInput = $state<HTMLInputElement | null>(null);

	// --- Classification metadata (all optional) ---
	let keywords = $state('');
	let category = $state('');
	let subcategory = $state('');
	let container = $state('');
	let status = $state('draft');
	let notes = $state('');
	let videoType = $state('');

	// Distinct existing values feed the category/subcategory comboboxes.
	let categoryOptions = $derived([
		...new Set(videos.map((v) => v.category).filter((c) => c && c.trim()))
	].sort());
	let subcategoryOptions = $derived([
		...new Set(videos.map((v) => v.subcategory).filter((c) => c && c.trim()))
	].sort());

	function formatBytes(n: number): string {
		if (n < 1024) return `${n} B`;
		const units = ['KB', 'MB', 'GB', 'TB'];
		let value = n / 1024;
		let i = 0;
		while (value >= 1024 && i < units.length - 1) {
			value /= 1024;
			i++;
		}
		return `${value.toFixed(1)} ${units[i]}`;
	}

	function formatDate(iso: string): string {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
	}

	async function refresh() {
		loading = true;
		error = null;
		try {
			videos = await listVideos({
				sorts,
				name: filterName.trim() || undefined,
				timeFrom: filterTimeFrom || undefined,
				timeTo: filterTimeTo || undefined
			});
			if (selected && !videos.some((v) => v.id === selected!.id)) selected = null;
		} catch (e) {
			error = e instanceof Error ? e.message : m.video_management_failed_to_load_videos();
		} finally {
			loading = false;
		}
	}

	function setSort(field: SortField, direction: SortDirection | 'none') {
		const existingIndex = sorts.findIndex((sort) => sort.field === field);
		if (direction === 'none') {
			if (existingIndex >= 0) sorts.splice(existingIndex, 1);
		} else if (existingIndex >= 0) {
			sorts[existingIndex] = { field, direction };
		} else {
			sorts.push({ field, direction });
		}
		void refresh();
	}

	function sortDirection(field: SortField): SortDirection | 'none' {
		return sorts.find((sort) => sort.field === field)?.direction ?? 'none';
	}

	function sortArrow(field: SortField) {
		const direction = sortDirection(field);
		return direction === 'asc' ? '↑' : direction === 'desc' ? '↓' : '';
	}

	function openDialog() {
		editingId = null;
		file = null;
		currentFilename = '';
		name = '';
		description = '';
		source = 'Recording';
		url = '';
		coverImage = null;
		keywords = '';
		category = '';
		subcategory = '';
		container = '';
		status = 'draft';
		notes = '';
		videoType = '';
		dialogError = null;
		uploadProgress = 0;
		dialogOpen = true;
	}

	function onDialogBackdropClick() {
		if (Date.now() < ignoreDialogBackdropUntil) return;
		if (!uploading) dialogOpen = false;
	}

	function openEditDialog(video: VideoMeta) {
		editingId = video.id;
		file = null;
		currentFilename = video.filename;
		name = video.name;
		description = video.description;
		source = video.source || 'Recording';
		url = video.url;
		coverImage = video.image_id != null
			? { id: video.image_id, content_url: video.image_url, filename: '', size_bytes: 0, content_type: '', origin: '', created_at: '' }
			: null;
		keywords = video.keywords;
		category = video.category;
		subcategory = video.subcategory;
		container = video.container;
		status = video.status || 'draft';
		notes = video.notes;
		videoType = video.video_type;
		dialogError = null;
		uploadProgress = 0;
		dialogOpen = true;
	}

	function onDialogFileChosen(event: Event) {
		const chosen = (event.target as HTMLInputElement).files?.[0] ?? null;
		file = chosen;
		if (chosen && !name) name = chosen.name.replace(/\.[^.]+$/, '');
		// Auto-detect the video type from the file extension (editable fallback).
		if (chosen) {
			const match = /\.([^.]+)$/.exec(chosen.name);
			videoType = match ? match[1].toLowerCase() : '';
		}
	}

	async function onAutoGenerate() {
		if (!name.trim() || !description.trim()) {
			dialogError = m.video_management_enter_a_name_and_description();
			return;
		}
		generating = true;
		dialogError = null;
		try {
			// Prompt reflects the description (the subject), with the name for context.
			// The generated image is saved into the image library server-side.
			const prompt = `Cover image for a training video titled "${name.trim()}". ${description.trim()}`;
			coverImage = await generateImage(prompt); // each click yields a new image
		} catch (e) {
			dialogError = e instanceof Error ? e.message : m.video_management_auto_generate_failed();
		} finally {
			generating = false;
		}
	}

	async function submitDialog() {
		if (editingId === null && !file) {
			dialogError = m.video_management_please_choose_a_video_file();
			return;
		}
		if (!name.trim()) {
			dialogError = m.video_management_name_is_required();
			return;
		}
		if (!description.trim()) {
			dialogError = m.video_management_description_is_required();
			return;
		}
		if (source === 'Web' && !url.trim()) {
			dialogError = m.video_management_a_url_is_required_when();
			return;
		}
		uploading = true;
		uploadProgress = 0;
		dialogError = null;
		try {
			const fields: VideoUploadFields = {
				name: name.trim(),
				description: description.trim(),
				source,
				url: url.trim(),
				image_id: coverImage?.id ?? null,
				keywords: keywords.trim(),
				category: category.trim(),
				subcategory: subcategory.trim(),
				container: container.trim(),
				status,
				notes: notes.trim(),
				video_type: videoType.trim()
			};
			const meta =
				editingId === null
					? await uploadVideo(file!, fields, (f) => (uploadProgress = f))
					: await updateVideo(editingId, fields, file, (f) => (uploadProgress = f));
			info = editingId === null ? m.video_management_uploaded_2({ name: meta.name }) : m.video_management_updated({ name: meta.name });
			error = null;
			dialogOpen = false;
			await refresh();
		} catch (e) {
			dialogError = e instanceof Error ? e.message : (editingId === null ? m.video_management_upload_failed() : m.video_management_update_failed());
		} finally {
			uploading = false;
			uploadProgress = 0;
		}
	}

	async function onDelete(video: VideoMeta) {
		if (!confirm(m.video_management_delete_this_cannot_be_undone({ name: video.name }))) return;
		error = null;
		info = null;
		try {
			await deleteVideo(video.id);
			if (selected?.id === video.id) selected = null;
			info = m.video_management_deleted({ name: video.name });
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : m.video_management_delete_failed();
		}
	}

	onMount(refresh);
</script>

<div class="p-6" style="color:{textPrimary};">
	<!-- Header -->
	<div class="flex items-start justify-between gap-4 mb-5 flex-wrap">
		<div>
			<h1 style="font-size:20px; font-weight:600; margin-bottom:4px;">{m.video_management_videos()}</h1>
			<p style="font-size:14px; color:{textSecondary};">
				{m.video_management_manage_training_videos_upload_with()}
			</p>
		</div>
		<button
			onclick={openDialog}
			class="rounded-lg px-4 py-2 cursor-pointer"
			style="background:{accent}; color:#fff; font-size:14px; font-weight:500; border:none;"
		>
			{m.video_management_upload_video()}
		</button>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg px-4 py-3" style="background:{dangerColor}1A; color:{dangerColor}; font-size:13px; border:1px solid {dangerColor}40;">{error}</div>
	{/if}
	{#if info}
		<div class="mb-4 rounded-lg px-4 py-3" style="background:{accentTint}; color:{accent}; font-size:13px; border:1px solid {accent}40;">{info}</div>
	{/if}

	<!-- Inline player -->
	{#if selected}
		<div class="mb-6 rounded-xl p-4" style="background:{cardBg}; border:1px solid {borderColor};">
			<div class="flex items-center justify-between mb-3">
				<span style="font-size:14px; font-weight:500;">{selected.name}</span>
				<button onclick={() => (selected = null)} class="cursor-pointer" style="background:none; border:none; color:{textSecondary}; font-size:13px;">{m.video_management_close()}</button>
			</div>
			<!-- svelte-ignore a11y_media_has_caption -->
			<video src={videoStreamUrl(selected.id)} controls style="width:100%; max-height:60vh; border-radius:8px; background:#000;"></video>
		</div>
	{/if}

	<!-- Sort / filter controls -->
	<div class="flex items-end gap-3 mb-4 flex-wrap">
		<div class="flex flex-col gap-1.5">
			<label for="v-filter-name" style="color:{textSecondary}; font-size:12px;">{m.video_management_filter_by_name()}</label>
			<input
				id="v-filter-name"
				bind:value={filterName}
				onkeydown={(e) => e.key === 'Enter' && refresh()}
				placeholder={m.video_management_search_name()}
				style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; font-size:13px; min-width:180px;"
			/>
		</div>
		<div class="flex flex-col gap-1.5">
			<label for="v-filter-from" style="color:{textSecondary}; font-size:12px;">{m.video_management_filter_by_time()}</label>
			<div class="flex items-center gap-2">
				<input
					id="v-filter-from"
					type="date"
					bind:value={filterTimeFrom}
					style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; font-size:13px;"
				/>
				<span style="color:{textSecondary};">–</span>
				<input
					id="v-filter-to"
					type="date"
					bind:value={filterTimeTo}
					style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; font-size:13px;"
				/>
			</div>
		</div>
		<button
			onclick={refresh}
			class="rounded-lg px-4 py-2 cursor-pointer"
			style="background:{accentTint}; color:{accent}; font-size:13px; font-weight:500; border:none;"
		>
			{m.video_management_apply()}
		</button>
	</div>

	<!-- Video list -->
	<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor}; background:{surface};">
		{#if loading}
			<div class="p-6 text-center" style="color:{textSecondary}; font-size:14px;">{m.video_management_loading()}</div>
		{:else if videos.length === 0}
			<div class="p-8 text-center" style="color:{textSecondary}; font-size:14px;">{m.video_management_no_videos_yet_upload_one()}</div>
		{:else}
			<table style="width:100%; border-collapse:collapse; font-size:13px;">
				<thead>
					<tr style="text-align:left; color:{textSecondary};">
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">{m.video_management_cover()}</th>
						<th style="padding:6px 10px; border-bottom:1px solid {borderColor}; font-weight:500;">
							<label style="display:flex; align-items:center; gap:6px; white-space:nowrap;">
								<span>{m.video_management_name()}</span>
								{#if sortArrow('name')}
									<span aria-label={sortDirection('name') === 'asc' ? m.video_management_sorted_ascending() : m.video_management_sorted_descending()} style="color:{accent}; font-size:16px; font-weight:700;">{sortArrow('name')}</span>
								{/if}
								<select aria-label={m.video_management_sort_name()} value={sortDirection('name')} onchange={(event) => setSort('name', event.currentTarget.value as SortDirection | 'none')} style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:4px 6px; font-size:12px;">
									<option value="none">{m.video_management_no_sort()}</option>
									<option value="asc">{m.video_management_ascending()}</option>
									<option value="desc">{m.video_management_descending()}</option>
								</select>
							</label>
						</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">{m.video_management_source()}</th>
						{#each sortableColumns.slice(1) as column (column.field)}
							<th style="padding:6px 10px; border-bottom:1px solid {borderColor}; font-weight:500;">
								<label style="display:flex; align-items:center; gap:6px; white-space:nowrap;">
									<span>{column.label}</span>
									{#if sortArrow(column.field)}
										<span aria-label={sortDirection(column.field) === 'asc' ? m.video_management_sorted_ascending() : m.video_management_sorted_descending()} style="color:{accent}; font-size:16px; font-weight:700;">{sortArrow(column.field)}</span>
									{/if}
									<select aria-label={m.video_management_sort({ label: column.label })} value={sortDirection(column.field)} onchange={(event) => setSort(column.field, event.currentTarget.value as SortDirection | 'none')} style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:6px; padding:4px 6px; font-size:12px;">
										<option value="none">{m.video_management_no_sort()}</option>
										<option value="asc">{m.video_management_ascending()}</option>
										<option value="desc">{m.video_management_descending()}</option>
									</select>
								</label>
							</th>
						{/each}
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500; text-align:right;">{m.video_management_actions()}</th>
					</tr>
				</thead>
				<tbody>
					{#each videos as video (video.id)}
						<tr>
							<td style="padding:8px 14px; border-bottom:1px solid {borderColor};">
								{#if video.image_url}
									<img src={video.image_url} alt="" style="width:56px; height:36px; object-fit:cover; border-radius:6px; display:block;" />
								{:else}
									<div style="width:56px; height:36px; border-radius:6px; background:{accentTint};"></div>
								{/if}
							</td>
							<td style="padding:8px 14px; border-bottom:1px solid {borderColor}; color:{textPrimary};">
								<div style="font-weight:500;">{video.name}</div>
								{#if video.description}
									<div style="color:{textSecondary}; font-size:12px; max-width:320px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;">{video.description}</div>
								{/if}
							</td>
							<td style="padding:8px 14px; border-bottom:1px solid {borderColor}; color:{textSecondary};">{video.source}</td>
							<td style="padding:8px 14px; border-bottom:1px solid {borderColor}; color:{textSecondary};">{formatBytes(video.size_bytes)}</td>
							<td style="padding:8px 14px; border-bottom:1px solid {borderColor}; color:{textSecondary};">{formatDate(video.created_at)}</td>
							<td style="padding:8px 14px; border-bottom:1px solid {borderColor}; text-align:right; white-space:nowrap;">
								<button onclick={() => (selected = video)} class="cursor-pointer" style="background:none; border:none; color:{accent}; font-size:13px; margin-left:8px;">{m.video_management_view()}</button>
								<button onclick={() => openEditDialog(video)} class="cursor-pointer" style="background:none; border:none; color:{accent}; font-size:13px; margin-left:12px;">{m.video_management_edit()}</button>
								<a href={videoDownloadUrl(video.id)} style="color:{accent}; font-size:13px; margin-left:12px; text-decoration:none;">{m.video_management_download()}</a>
								<button onclick={() => onDelete(video)} class="cursor-pointer" style="background:none; border:none; color:{dangerColor}; font-size:13px; margin-left:12px;">{m.video_management_delete()}</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<!-- Upload dialog -->
{#if dialogOpen}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="fixed inset-0 z-40 flex items-center justify-center p-4" style="background:{overlayBg};" onclick={onDialogBackdropClick}>
		<div
			class="w-full max-w-lg rounded-2xl overflow-hidden flex flex-col"
			style="background:{surface}; border:1px solid {borderColor}; max-height:88vh;"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="flex items-center justify-between px-5 py-4" style="border-bottom:1px solid {borderColor};">
				<h2 style="font-size:16px; font-weight:600; color:{textPrimary};">{editingId === null ? m.video_management_upload_video() : m.video_management_edit_video()}</h2>
				<button onclick={() => !uploading && (dialogOpen = false)} class="cursor-pointer" style="background:none; border:none; color:{textSecondary}; font-size:16px;">✕</button>
			</div>

			<div class="p-5 overflow-y-auto flex flex-col gap-4" style="font-size:13px;">
				{#if dialogError}
					<div style="color:{dangerColor}; font-size:13px;">{dialogError}</div>
				{/if}

				<!-- Video file (optional when editing — leave empty to keep the current file) -->
				<div class="flex flex-col gap-1.5">
					<label for="v-file-display" style="color:{textSecondary};">
						{m.video_management_video_file_name()}{#if editingId === null}<span style="color:{dangerColor};"> *</span>{/if}
					</label>
					<div class="flex items-center gap-2">
						<input id="v-file-display" type="text" readonly value={file ? file.name : currentFilename} placeholder={m.video_management_no_file_chosen()}
							style="flex:1; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						<button type="button" onclick={() => dialogFileInput?.click()} class="cursor-pointer rounded-lg px-3 py-2"
							style="background:{accentTint}; color:{accent}; border:none; font-size:13px; white-space:nowrap;">{m.video_management_pick_file()}</button>
					</div>
					<input bind:this={dialogFileInput} id="v-file" type="file" accept="video/*" onchange={onDialogFileChosen}
						style="display:none;" />
				</div>

				<!-- Name -->
				<div class="flex flex-col gap-1.5">
					<label for="v-name" style="color:{textSecondary};">{m.video_management_name()} <span style="color:{dangerColor};">*</span></label>
					<input id="v-name" bind:value={name} placeholder={m.video_management_video_name()} required
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
				</div>

				<!-- Description -->
				<div class="flex flex-col gap-1.5">
					<label for="v-desc" style="color:{textSecondary};">{m.video_management_description()} <span style="color:{dangerColor};">*</span></label>
					<textarea id="v-desc" bind:value={description} rows="2" placeholder={m.video_management_short_description()} required
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; resize:vertical;"></textarea>
				</div>

				<!-- Source + URL -->
				<div class="flex gap-3 flex-wrap">
					<div class="flex flex-col gap-1.5" style="min-width:140px;">
						<label for="v-source" style="color:{textSecondary};">{m.video_management_source()}</label>
						<select id="v-source" bind:value={source}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;">
							<option value="Recording">{m.video_management_recording()}</option>
							<option value="Web">{m.video_management_web()}</option>
						</select>
					</div>
					{#if source === 'Web'}
						<div class="flex flex-col gap-1.5 flex-1" style="min-width:200px;">
							<label for="v-url" style="color:{textSecondary};">{m.video_management_url()}</label>
							<input id="v-url" bind:value={url} placeholder={m.video_management_https()}
								style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						</div>
					{/if}
				</div>

				<!-- Keywords -->
				<div class="flex flex-col gap-1.5">
					<label for="v-keywords" style="color:{textSecondary};">{m.video_management_keywords()}</label>
					<input id="v-keywords" bind:value={keywords} placeholder={m.video_management_comma_separated_tags()}
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
				</div>

				<!-- Category + Subcategory (editable comboboxes) -->
				<div class="flex gap-3 flex-wrap">
					<div class="flex flex-col gap-1.5 flex-1" style="min-width:160px;">
						<label for="v-category" style="color:{textSecondary};">{m.video_management_category()}</label>
						<input id="v-category" list="v-category-options" bind:value={category} placeholder={m.video_management_pick_or_type()}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						<datalist id="v-category-options">
							{#each categoryOptions as opt (opt)}<option value={opt}></option>{/each}
						</datalist>
					</div>
					<div class="flex flex-col gap-1.5 flex-1" style="min-width:160px;">
						<label for="v-subcategory" style="color:{textSecondary};">{m.video_management_subcategory()}</label>
						<input id="v-subcategory" list="v-subcategory-options" bind:value={subcategory} placeholder={m.video_management_pick_or_type()}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						<datalist id="v-subcategory-options">
							{#each subcategoryOptions as opt (opt)}<option value={opt}></option>{/each}
						</datalist>
					</div>
				</div>

				<!-- Container + Status + Video type -->
				<div class="flex gap-3 flex-wrap">
					<div class="flex flex-col gap-1.5 flex-1" style="min-width:160px;">
						<label for="v-container" style="color:{textSecondary};">{m.video_management_container()}</label>
						<input id="v-container" bind:value={container} placeholder={m.video_management_collection_group()}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
					</div>
					<div class="flex flex-col gap-1.5" style="min-width:140px;">
						<label for="v-status" style="color:{textSecondary};">{m.video_management_status()}</label>
						<select id="v-status" bind:value={status}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;">
							<option value="draft">{m.video_management_draft()}</option>
							<option value="published">{m.video_management_published()}</option>
							<option value="archived">{m.video_management_archived()}</option>
						</select>
					</div>
					<div class="flex flex-col gap-1.5" style="min-width:120px;">
						<label for="v-video-type" style="color:{textSecondary};">{m.video_management_video_type()}</label>
						<input id="v-video-type" bind:value={videoType} placeholder={m.video_management_e_g_mp4()}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
					</div>
				</div>

				<!-- Notes -->
				<div class="flex flex-col gap-1.5">
					<label for="v-notes" style="color:{textSecondary};">{m.video_management_notes()}</label>
					<textarea id="v-notes" bind:value={notes} rows="2" placeholder={m.video_management_internal_notes()}
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; resize:vertical;"></textarea>
				</div>

				<!-- Cover image -->
				<div class="flex flex-col gap-1.5">
					<span style="color:{textSecondary};">{m.video_management_cover_image()}</span>
					<div class="flex items-center gap-3">
						<div style="width:96px; height:60px; border-radius:8px; border:1px solid {borderColor}; background:{accentTint}; overflow:hidden; flex-shrink:0;">
							{#if coverImage}
								<img src={coverImage.content_url} alt={m.video_management_cover_2()} style="width:100%; height:100%; object-fit:cover; display:block;" />
							{/if}
						</div>
						<div class="flex flex-col gap-2">
							<button onclick={() => (showPicker = true)} class="cursor-pointer rounded-lg px-3 py-1.5"
								style="background:{accentTint}; color:{accent}; border:none; font-size:13px;">{m.video_management_pick_an_image()}</button>
							<button onclick={onAutoGenerate} disabled={generating} class="cursor-pointer rounded-lg px-3 py-1.5"
								style="background:{accentTint}; color:{accent}; border:none; font-size:13px; opacity:{generating ? 0.6 : 1};">
								{generating ? m.video_management_generating() : m.video_management_auto_generate()}
							</button>
						</div>
					</div>
				</div>

				{#if uploading && (editingId === null || file)}
					<div style="width:100%; height:6px; background:{borderColor}; border-radius:999px; overflow:hidden;">
						<div style="width:{Math.round(uploadProgress * 100)}%; height:100%; background:{accent}; transition:width 120ms;"></div>
					</div>
				{/if}
			</div>

			<div class="flex items-center justify-end gap-3 px-5 py-4" style="border-top:1px solid {borderColor};">
				<button onclick={() => !uploading && (dialogOpen = false)} class="cursor-pointer rounded-lg px-4 py-2"
					style="background:none; border:1px solid {borderColor}; color:{textSecondary}; font-size:14px;">{m.video_management_cancel()}</button>
				<button onclick={submitDialog} disabled={uploading} class="cursor-pointer rounded-lg px-4 py-2"
					style="background:{accent}; color:#fff; border:none; font-size:14px; opacity:{uploading ? 0.6 : 1};">
					{#if editingId === null}
						{uploading ? m.video_management_uploading({ value: Math.round(uploadProgress * 100) }) : m.video_management_upload()}
					{:else}
						{uploading ? (file ? m.video_management_saving({ value: Math.round(uploadProgress * 100) }) : m.video_management_saving_2()) : m.video_management_save()}
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}

{#if showPicker}
	<ImageLibraryPicker
		{darkMode}
		onPick={(img) => {
			coverImage = img;
			ignoreDialogBackdropUntil = Date.now() + 500;
			showPicker = false;
		}}
		onClose={() => (showPicker = false)}
	/>
{/if}
