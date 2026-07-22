<script lang="ts">
	import { onMount } from 'svelte';
	import { listImages, uploadImage, imageContentUrl, type ImageMeta } from '$lib/services/imageService';

	let {
		darkMode = true,
		onPick,
		onClose
	}: {
		darkMode?: boolean;
		onPick: (image: ImageMeta) => void;
		onClose: () => void;
	} = $props();

	let overlayBg = $derived(darkMode ? 'rgba(0,0,0,0.6)' : 'rgba(0,0,0,0.4)');
	let panelBg = $derived(darkMode ? '#1E2333' : '#FFFFFF');
	let borderColor = $derived(darkMode ? '#2D3348' : '#E4E6EB');
	let accent = $derived(darkMode ? '#818CF8' : '#6366F1');
	let textPrimary = $derived(darkMode ? '#E2E8F0' : '#111827');
	let textSecondary = $derived(darkMode ? '#94A3B8' : '#6B7280');

	let images = $state<ImageMeta[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let uploading = $state(false);
	let fileInput = $state<HTMLInputElement | null>(null);

	async function refresh() {
		loading = true;
		error = null;
		try {
			images = await listImages();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load images';
		} finally {
			loading = false;
		}
	}

	async function onUploadChosen(event: Event) {
		const file = (event.target as HTMLInputElement).files?.[0];
		if (!file) return;
		uploading = true;
		error = null;
		try {
			const img = await uploadImage(file);
			await refresh();
			onPick(img); // newly uploaded image becomes the selection
		} catch (e) {
			error = e instanceof Error ? e.message : 'Upload failed';
		} finally {
			uploading = false;
			if (fileInput) fileInput.value = '';
		}
	}

	onMount(refresh);
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="fixed inset-0 z-50 flex items-center justify-center p-4"
	style="background:{overlayBg};"
	onclick={onClose}
>
	<div
		class="w-full max-w-3xl rounded-2xl overflow-hidden flex flex-col"
		style="background:{panelBg}; border:1px solid {borderColor}; max-height:80vh;"
		onclick={(e) => e.stopPropagation()}
	>
		<div class="flex items-center justify-between px-5 py-4" style="border-bottom:1px solid {borderColor};">
			<h2 style="font-size:16px; font-weight:600; color:{textPrimary};">Pick an Image</h2>
			<div class="flex items-center gap-3">
				<input bind:this={fileInput} type="file" accept="image/*" class="hidden" onchange={onUploadChosen} />
				<button
					onclick={() => fileInput?.click()}
					disabled={uploading}
					class="rounded-lg px-3 py-1.5 cursor-pointer"
					style="background:{accent}; color:#fff; font-size:13px; border:none; opacity:{uploading ? 0.6 : 1};"
				>
					{uploading ? 'Uploading…' : 'Upload new'}
				</button>
				<button
					onclick={onClose}
					class="cursor-pointer"
					style="background:none; border:none; color:{textSecondary}; font-size:14px;"
				>
					✕
				</button>
			</div>
		</div>

		<div class="p-5 overflow-y-auto">
			{#if error}
				<div class="mb-3" style="color:#F87171; font-size:13px;">{error}</div>
			{/if}
			{#if loading}
				<div class="py-8 text-center" style="color:{textSecondary}; font-size:14px;">Loading…</div>
			{:else if images.length === 0}
				<div class="py-8 text-center" style="color:{textSecondary}; font-size:14px;">
					No images yet. Upload one to start your library.
				</div>
			{:else}
				<div class="grid gap-3" style="grid-template-columns:repeat(auto-fill,minmax(120px,1fr));">
					{#each images as img (img.id)}
						<button
							onclick={() => onPick(img)}
							class="cursor-pointer rounded-lg overflow-hidden"
							style="border:1px solid {borderColor}; background:#000; aspect-ratio:1/1; padding:0;"
							title={img.filename}
						>
							<img
								src={imageContentUrl(img.id)}
								alt={img.filename}
								style="width:100%; height:100%; object-fit:cover; display:block;"
							/>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
