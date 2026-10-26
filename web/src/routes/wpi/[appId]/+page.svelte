<script lang="ts">
	import { m } from '$lib/paraglide/messages.js';
	import { fetchApp, WpiError } from '$lib/wpi/api';
	import ErrorBox from '$lib/wpi/ErrorBox.svelte';
	import type { Bundle, Issue } from '$lib/wpi/types';
	import WpiApp from '$lib/wpi/WpiApp.svelte';

	let { data } = $props();

	type Loaded = { kind: 'loading' } | { kind: 'ready'; bundle: Bundle } | { kind: 'failed'; message: string; issues: Issue[] };
	let loaded = $state<Loaded>({ kind: 'loading' });

	$effect(() => {
		const appId = data.appId;
		loaded = { kind: 'loading' };
		fetchApp(appId)
			.then((bundle) => {
				if (bundle.warnings.length) console.warn('WPI descriptor warnings', bundle.warnings);
				loaded = { kind: 'ready', bundle };
			})
			.catch((err: unknown) => {
				if (err instanceof WpiError && err.status === 404) loaded = { kind: 'failed', message: m.wpi_app_not_found(), issues: [] };
				else if (err instanceof WpiError && err.issues.length) loaded = { kind: 'failed', message: m.wpi_app_invalid_intro(), issues: err.issues };
				else loaded = { kind: 'failed', message: m.wpi_app_load_failed({ message: err instanceof Error ? err.message : String(err) }), issues: [] };
			});
	});
</script>

<svelte:head><title>{data.appId}</title></svelte:head>

<main class="bg-background text-foreground h-screen w-screen overflow-hidden select-text">
	{#if loaded.kind === 'loading'}
		<p class="text-muted-foreground p-4 text-sm">{m.wpi_loading_app()}</p>
	{:else if loaded.kind === 'failed'}
		<div class="p-4">
			<ErrorBox
				title={m.wpi_app_invalid_title()}
				message={loaded.message}
				items={loaded.issues.map((i) => `${i.kind} ${i.descriptor_id}${i.path}: ${i.message}`)}
			/>
		</div>
	{:else}
		{#key data.appId}
			<WpiApp bundle={loaded.bundle} />
		{/key}
	{/if}
</main>
