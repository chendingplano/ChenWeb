<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listVideos,
		uploadVideo,
		deleteVideo,
		videoStreamUrl,
		videoDownloadUrl,
		type VideoMeta
	} from '$lib/services/videoService';

	let { darkMode = true }: { darkMode?: boolean } = $props();

	// --- Design tokens (match the Dashboard app shell) ---
	let surface = $derived(darkMode ? '#1E2333' : '#FFFFFF');
	let cardBg = $derived(darkMode ? '#252A3A' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let accentTint = $derived(darkMode ? 'rgba(129,140,248,0.15)' : 'rgba(99,102,241,0.10)');
	let dangerColor = $derived(darkMode ? '#F87171' : '#DC2626');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');

	let videos = $state<VideoMeta[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let info = $state<string | null>(null);
	let uploading = $state(false);
	let uploadProgress = $state(0);
	let selected = $state<VideoMeta | null>(null);
	let fileInput = $state<HTMLInputElement | null>(null);

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
			videos = await listVideos();
			// Keep the selection valid after a refresh.
			if (selected && !videos.some((v) => v.id === selected!.id)) selected = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load videos';
		} finally {
			loading = false;
		}
	}

	async function onFileChosen(event: Event) {
		const target = event.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;

		uploading = true;
		uploadProgress = 0;
		error = null;
		info = null;
		try {
			const meta = await uploadVideo(file, (f) => (uploadProgress = f));
			info = `Uploaded "${meta.filename}"`;
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Upload failed';
		} finally {
			uploading = false;
			uploadProgress = 0;
			if (fileInput) fileInput.value = ''; // allow re-uploading the same file
		}
	}

	async function onDelete(video: VideoMeta) {
		if (!confirm(`Delete "${video.filename}"? This cannot be undone.`)) return;
		error = null;
		info = null;
		try {
			await deleteVideo(video.id);
			if (selected?.id === video.id) selected = null;
			info = `Deleted "${video.filename}"`;
			await refresh();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Delete failed';
		}
	}

	onMount(refresh);
</script>

<div class="p-6" style="color:{textPrimary};">
	<!-- Header + upload -->
	<div class="flex items-start justify-between gap-4 mb-5 flex-wrap">
		<div>
			<h1 style="font-size:20px; font-weight:600; margin-bottom:4px;">Training Videos</h1>
			<p style="font-size:14px; color:{textSecondary};">
				Upload, view, download, and delete training videos.
			</p>
		</div>
		<div class="flex flex-col items-end gap-2">
			<input
				bind:this={fileInput}
				type="file"
				accept="video/*"
				class="hidden"
				onchange={onFileChosen}
			/>
			<button
				onclick={() => fileInput?.click()}
				disabled={uploading}
				class="rounded-lg px-4 py-2 cursor-pointer transition-opacity"
				style="background:{accent}; color:#fff; font-size:14px; font-weight:500; border:none; opacity:{uploading
					? 0.6
					: 1};"
			>
				{uploading ? `Uploading… ${Math.round(uploadProgress * 100)}%` : 'Upload video'}
			</button>
			{#if uploading}
				<div style="width:180px; height:6px; background:{borderColor}; border-radius:999px; overflow:hidden;">
					<div
						style="width:{Math.round(uploadProgress * 100)}%; height:100%; background:{accent}; transition:width 120ms;"
					></div>
				</div>
			{/if}
		</div>
	</div>

	{#if error}
		<div
			class="mb-4 rounded-lg px-4 py-3"
			style="background:{dangerColor}1A; color:{dangerColor}; font-size:13px; border:1px solid {dangerColor}40;"
		>
			{error}
		</div>
	{/if}
	{#if info}
		<div
			class="mb-4 rounded-lg px-4 py-3"
			style="background:{accentTint}; color:{accent}; font-size:13px; border:1px solid {accent}40;"
		>
			{info}
		</div>
	{/if}

	<!-- Inline player -->
	{#if selected}
		<div
			class="mb-6 rounded-xl p-4"
			style="background:{cardBg}; border:1px solid {borderColor};"
		>
			<div class="flex items-center justify-between mb-3">
				<span style="font-size:14px; font-weight:500;">{selected.filename}</span>
				<button
					onclick={() => (selected = null)}
					class="cursor-pointer"
					style="background:none; border:none; color:{textSecondary}; font-size:13px;"
				>
					Close
				</button>
			</div>
			<!-- svelte-ignore a11y_media_has_caption -->
			<video
				src={videoStreamUrl(selected.id)}
				controls
				style="width:100%; max-height:60vh; border-radius:8px; background:#000;"
			></video>
		</div>
	{/if}

	<!-- Video list -->
	<div class="rounded-xl overflow-hidden" style="border:1px solid {borderColor}; background:{surface};">
		{#if loading}
			<div class="p-6 text-center" style="color:{textSecondary}; font-size:14px;">Loading…</div>
		{:else if videos.length === 0}
			<div class="p-8 text-center" style="color:{textSecondary}; font-size:14px;">
				No videos yet. Upload one to get started.
			</div>
		{:else}
			<table style="width:100%; border-collapse:collapse; font-size:13px;">
				<thead>
					<tr style="text-align:left; color:{textSecondary};">
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Name</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Size</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Uploaded by</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500;">Date</th>
						<th style="padding:10px 14px; border-bottom:1px solid {borderColor}; font-weight:500; text-align:right;">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each videos as video (video.id)}
						<tr>
							<td style="padding:10px 14px; border-bottom:1px solid {borderColor}; color:{textPrimary};">
								{video.filename}
							</td>
							<td style="padding:10px 14px; border-bottom:1px solid {borderColor}; color:{textSecondary};">
								{formatBytes(video.size_bytes)}
							</td>
							<td style="padding:10px 14px; border-bottom:1px solid {borderColor}; color:{textSecondary};">
								{video.uploaded_by || '—'}
							</td>
							<td style="padding:10px 14px; border-bottom:1px solid {borderColor}; color:{textSecondary};">
								{formatDate(video.created_at)}
							</td>
							<td style="padding:10px 14px; border-bottom:1px solid {borderColor}; text-align:right; white-space:nowrap;">
								<button
									onclick={() => (selected = video)}
									class="cursor-pointer"
									style="background:none; border:none; color:{accent}; font-size:13px; margin-left:8px;"
								>
									View
								</button>
								<a
									href={videoDownloadUrl(video.id)}
									style="color:{accent}; font-size:13px; margin-left:12px; text-decoration:none;"
								>
									Download
								</a>
								<button
									onclick={() => onDelete(video)}
									class="cursor-pointer"
									style="background:none; border:none; color:{dangerColor}; font-size:13px; margin-left:12px;"
								>
									Delete
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>
