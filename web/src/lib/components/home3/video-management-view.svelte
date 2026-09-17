<script lang="ts">
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
	const sortOptions = [
		{ value: 'name-asc', label: 'By Name ASC', sortBy: 'name', sortDir: 'asc' },
		{ value: 'name-desc', label: 'By Name DESC', sortBy: 'name', sortDir: 'desc' },
		{ value: 'created_at-asc', label: 'By Time ASC', sortBy: 'created_at', sortDir: 'asc' },
		{ value: 'created_at-desc', label: 'By Time DESC', sortBy: 'created_at', sortDir: 'desc' },
		{ value: 'size_bytes-asc', label: 'By Size ASC', sortBy: 'size_bytes', sortDir: 'asc' },
		{ value: 'size_bytes-desc', label: 'By Size DESC', sortBy: 'size_bytes', sortDir: 'desc' }
	] as const;
	let sortOption = $state<(typeof sortOptions)[number]['value']>('created_at-desc');
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
		const chosen = sortOptions.find((o) => o.value === sortOption)!;
		try {
			videos = await listVideos({
				sortBy: chosen.sortBy,
				sortDir: chosen.sortDir,
				name: filterName.trim() || undefined,
				timeFrom: filterTimeFrom || undefined,
				timeTo: filterTimeTo || undefined
			});
			if (selected && !videos.some((v) => v.id === selected!.id)) selected = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load videos';
		} finally {
			loading = false;
		}
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
			dialogError = 'Enter a name and description first — the cover is generated from them.';
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
			dialogError = e instanceof Error ? e.message : 'Auto-generate failed';
		} finally {
			generating = false;
		}
	}

	async function submitDialog() {
		if (editingId === null && !file) {
			dialogError = 'Please choose a video file.';
			return;
		}
		if (!name.trim()) {
			dialogError = 'Name is required.';
			return;
		}
		if (!description.trim()) {
			dialogError = 'Description is required.';
			return;
		}
		if (source === 'Web' && !url.trim()) {
			dialogError = 'A URL is required when source is Web.';
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
			info = editingId === null ? `Uploaded "${meta.name}"` : `Updated "${meta.name}"`;
			error = null;
			dialogOpen = false;
			await refresh();
		} catch (e) {
			dialogError = e instanceof Error ? e.message : (editingId === null ? 'Upload failed' : 'Update failed');
		} finally {
			uploading = false;
			uploadProgress = 0;
		}
	}

	async function onDelete(video: VideoMeta) {
		if (!confirm(`Delete "${video.name}"? This cannot be undone.`)) return;
		error = null;
		info = null;
		try {
			await deleteVideo(video.id);
			if (selected?.id === video.id) selected = null;
			info = `Deleted "${video.name}"`;
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		}
	}

	onMount(refresh);
</script>

<div class="p-6" style="color:{textPrimary};">
	<!-- Header -->
	<div class="flex items-start justify-between gap-4 mb-5 flex-wrap">
		<div>
			<h1 style="font-size:20px; font-weight:600; margin-bottom:4px;">Videos</h1>
			<p style="font-size:14px; color:{textSecondary};">
				Manage training videos — upload with metadata and a cover image, view, download, and delete.
			</p>
		</div>
		<button
			onclick={openDialog}
			class="rounded-lg px-4 py-2 cursor-pointer"
			style="background:{accent}; color:#fff; font-size:14px; font-weight:500; border:none;"
		>
			Upload video
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
				<button onclick={() => (selected = null)} class="cursor-pointer" style="background:none; border:none; color:{textSecondary}; font-size:13px;">Close</button>
			</div>
			<!-- svelte-ignore a11y_media_has_caption -->
			<video src={videoStreamUrl(selected.id)} controls style="width:100%; max-height:60vh; border-radius:8px; background:#000;"></video>
		</div>
	{/if}

	<!-- Sort / filter controls -->
	<div class="flex items-end gap-3 mb-4 flex-wrap">
		<div class="flex flex-col gap-1.5">
			<label for="v-sort" style="color:{textSecondary}; font-size:12px;">Sort</label>
			<select
				id="v-sort"
				bind:value={sortOption}
				onchange={refresh}
				style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; font-size:13px;"
			>
				{#each sortOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
		<div class="flex flex-col gap-1.5">
			<label for="v-filter-name" style="color:{textSecondary}; font-size:12px;">Filter by Name</label>
			<input
				id="v-filter-name"
				bind:value={filterName}
				onkeydown={(e) => e.key === 'Enter' && refresh()}
				placeholder="Search name…"
				style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; font-size:13px; min-width:180px;"
			/>
		</div>
		<div class="flex flex-col gap-1.5">
			<label for="v-filter-from" style="color:{textSecondary}; font-size:12px;">Filter by Time</label>
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
			Apply
		</button>
	</div>

	<!-- Video list -->
	<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor}; background:{surface};">
		{#if loading}
			<div class="p-6 text-center" style="color:{textSecondary}; font-size:14px;">Loading…</div>
		{:else if videos.length === 0}
			<div class="p-8 text-center" style="color:{textSecondary}; font-size:14px;">No videos yet. Upload one to get started.</div>
		{:else}
			<table style="width:100%; border-collapse:collapse; font-size:13px;">
				<thead>
					<tr style="text-align:left; color:{textSecondary};">
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Cover</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Name</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Source</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Size</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Uploaded</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500; text-align:right;">Actions</th>
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
								<button onclick={() => (selected = video)} class="cursor-pointer" style="background:none; border:none; color:{accent}; font-size:13px; margin-left:8px;">View</button>
								<button onclick={() => openEditDialog(video)} class="cursor-pointer" style="background:none; border:none; color:{accent}; font-size:13px; margin-left:12px;">Edit</button>
								<a href={videoDownloadUrl(video.id)} style="color:{accent}; font-size:13px; margin-left:12px; text-decoration:none;">Download</a>
								<button onclick={() => onDelete(video)} class="cursor-pointer" style="background:none; border:none; color:{dangerColor}; font-size:13px; margin-left:12px;">Delete</button>
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
	<div class="fixed inset-0 z-40 flex items-center justify-center p-4" style="background:{overlayBg};" onclick={() => !uploading && (dialogOpen = false)}>
		<div
			class="w-full max-w-lg rounded-2xl overflow-hidden flex flex-col"
			style="background:{surface}; border:1px solid {borderColor}; max-height:88vh;"
			onclick={(e) => e.stopPropagation()}
		>
			<div class="flex items-center justify-between px-5 py-4" style="border-bottom:1px solid {borderColor};">
				<h2 style="font-size:16px; font-weight:600; color:{textPrimary};">{editingId === null ? 'Upload video' : 'Edit video'}</h2>
				<button onclick={() => !uploading && (dialogOpen = false)} class="cursor-pointer" style="background:none; border:none; color:{textSecondary}; font-size:16px;">✕</button>
			</div>

			<div class="p-5 overflow-y-auto flex flex-col gap-4" style="font-size:13px;">
				{#if dialogError}
					<div style="color:{dangerColor}; font-size:13px;">{dialogError}</div>
				{/if}

				<!-- Video file (optional when editing — leave empty to keep the current file) -->
				<div class="flex flex-col gap-1.5">
					<label for="v-file-display" style="color:{textSecondary};">
						Video File Name{#if editingId === null}<span style="color:{dangerColor};"> *</span>{/if}
					</label>
					<div class="flex items-center gap-2">
						<input id="v-file-display" type="text" readonly value={file ? file.name : currentFilename} placeholder="No file chosen"
							style="flex:1; background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						<button type="button" onclick={() => dialogFileInput?.click()} class="cursor-pointer rounded-lg px-3 py-2"
							style="background:{accentTint}; color:{accent}; border:none; font-size:13px; white-space:nowrap;">Pick File</button>
					</div>
					<input bind:this={dialogFileInput} id="v-file" type="file" accept="video/*" onchange={onDialogFileChosen}
						style="display:none;" />
				</div>

				<!-- Name -->
				<div class="flex flex-col gap-1.5">
					<label for="v-name" style="color:{textSecondary};">Name <span style="color:{dangerColor};">*</span></label>
					<input id="v-name" bind:value={name} placeholder="Video name" required
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
				</div>

				<!-- Description -->
				<div class="flex flex-col gap-1.5">
					<label for="v-desc" style="color:{textSecondary};">Description <span style="color:{dangerColor};">*</span></label>
					<textarea id="v-desc" bind:value={description} rows="2" placeholder="Short description" required
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; resize:vertical;"></textarea>
				</div>

				<!-- Source + URL -->
				<div class="flex gap-3 flex-wrap">
					<div class="flex flex-col gap-1.5" style="min-width:140px;">
						<label for="v-source" style="color:{textSecondary};">Source</label>
						<select id="v-source" bind:value={source}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;">
							<option value="Recording">Recording</option>
							<option value="Web">Web</option>
						</select>
					</div>
					{#if source === 'Web'}
						<div class="flex flex-col gap-1.5 flex-1" style="min-width:200px;">
							<label for="v-url" style="color:{textSecondary};">URL</label>
							<input id="v-url" bind:value={url} placeholder="https://…"
								style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						</div>
					{/if}
				</div>

				<!-- Keywords -->
				<div class="flex flex-col gap-1.5">
					<label for="v-keywords" style="color:{textSecondary};">Keywords</label>
					<input id="v-keywords" bind:value={keywords} placeholder="Comma-separated tags"
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
				</div>

				<!-- Category + Subcategory (editable comboboxes) -->
				<div class="flex gap-3 flex-wrap">
					<div class="flex flex-col gap-1.5 flex-1" style="min-width:160px;">
						<label for="v-category" style="color:{textSecondary};">Category</label>
						<input id="v-category" list="v-category-options" bind:value={category} placeholder="Pick or type…"
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						<datalist id="v-category-options">
							{#each categoryOptions as opt (opt)}<option value={opt}></option>{/each}
						</datalist>
					</div>
					<div class="flex flex-col gap-1.5 flex-1" style="min-width:160px;">
						<label for="v-subcategory" style="color:{textSecondary};">Subcategory</label>
						<input id="v-subcategory" list="v-subcategory-options" bind:value={subcategory} placeholder="Pick or type…"
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
						<datalist id="v-subcategory-options">
							{#each subcategoryOptions as opt (opt)}<option value={opt}></option>{/each}
						</datalist>
					</div>
				</div>

				<!-- Container + Status + Video type -->
				<div class="flex gap-3 flex-wrap">
					<div class="flex flex-col gap-1.5 flex-1" style="min-width:160px;">
						<label for="v-container" style="color:{textSecondary};">Container</label>
						<input id="v-container" bind:value={container} placeholder="Collection / group"
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
					</div>
					<div class="flex flex-col gap-1.5" style="min-width:140px;">
						<label for="v-status" style="color:{textSecondary};">Status</label>
						<select id="v-status" bind:value={status}
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;">
							<option value="draft">Draft</option>
							<option value="published">Published</option>
							<option value="archived">Archived</option>
						</select>
					</div>
					<div class="flex flex-col gap-1.5" style="min-width:120px;">
						<label for="v-video-type" style="color:{textSecondary};">Video type</label>
						<input id="v-video-type" bind:value={videoType} placeholder="e.g. mp4"
							style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px;" />
					</div>
				</div>

				<!-- Notes -->
				<div class="flex flex-col gap-1.5">
					<label for="v-notes" style="color:{textSecondary};">Notes</label>
					<textarea id="v-notes" bind:value={notes} rows="2" placeholder="Internal notes"
						style="background:{inputBg}; border:1px solid {borderColor}; color:{textPrimary}; border-radius:8px; padding:8px 10px; resize:vertical;"></textarea>
				</div>

				<!-- Cover image -->
				<div class="flex flex-col gap-1.5">
					<span style="color:{textSecondary};">Cover image</span>
					<div class="flex items-center gap-3">
						<div style="width:96px; height:60px; border-radius:8px; border:1px solid {borderColor}; background:{accentTint}; overflow:hidden; flex-shrink:0;">
							{#if coverImage}
								<img src={coverImage.content_url} alt="cover" style="width:100%; height:100%; object-fit:cover; display:block;" />
							{/if}
						</div>
						<div class="flex flex-col gap-2">
							<button onclick={() => (showPicker = true)} class="cursor-pointer rounded-lg px-3 py-1.5"
								style="background:{accentTint}; color:{accent}; border:none; font-size:13px;">Pick an Image</button>
							<button onclick={onAutoGenerate} disabled={generating} class="cursor-pointer rounded-lg px-3 py-1.5"
								style="background:{accentTint}; color:{accent}; border:none; font-size:13px; opacity:{generating ? 0.6 : 1};">
								{generating ? 'Generating…' : 'Auto-Generate'}
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
					style="background:none; border:1px solid {borderColor}; color:{textSecondary}; font-size:14px;">Cancel</button>
				<button onclick={submitDialog} disabled={uploading} class="cursor-pointer rounded-lg px-4 py-2"
					style="background:{accent}; color:#fff; border:none; font-size:14px; opacity:{uploading ? 0.6 : 1};">
					{#if editingId === null}
						{uploading ? `Uploading… ${Math.round(uploadProgress * 100)}%` : 'Upload'}
					{:else}
						{uploading ? (file ? `Saving… ${Math.round(uploadProgress * 100)}%` : 'Saving…') : 'Save'}
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
			showPicker = false;
		}}
		onClose={() => (showPicker = false)}
	/>
{/if}
