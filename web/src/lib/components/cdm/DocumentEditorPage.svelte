<script lang="ts">
	// Task 8.1: fetch-by-key wrapper behind /home3/cdm/[key]. Kept separate
	// from DocumentEditor.svelte itself (which takes an already-loaded
	// Document and has no route awareness) so DocumentEditor stays testable
	// with a plain injected fixture, matching how it was built and unit/
	// browser-tested in task group 7.
	import { getDocument, CdmApiError } from './cdm-client.js';
	import type { Document } from './types.js';
	import DocumentEditor from './DocumentEditor.svelte';

	let { documentKey }: { documentKey: string } = $props();

	// $state.raw, not $state: this component only ever replaces the whole
	// reference (on load, or on a later re-fetch after a key change), never
	// mutates a field inside it -- DocumentEditor owns the document from here
	// on and takes its own private copy. $state would deep-proxy the fetched
	// object, and DocumentEditor's `$state(structuredClone(initialDocument))`
	// then fails at runtime: structuredClone cannot clone a Svelte 5 reactive
	// proxy ("could not be cloned"), confirmed live -- the group 7 scratch
	// route never caught this because it seeded DocumentEditor from a plain
	// JSON import, never from a value that had passed through $state first.
	let doc = $state.raw<Document | null>(null);
	let loading = $state(true);
	let loadError = $state('');

	// Keyed on documentKey, not onMount: navigating client-side from one
	// document to another re-runs this without a full page reload, and the
	// key-vs-documentKey guards below discard a response that resolves after
	// a newer navigation already started a different fetch.
	$effect(() => {
		const key = documentKey;
		loading = true;
		loadError = '';
		doc = null;
		getDocument(key)
			.then((d) => {
				if (key === documentKey) doc = d;
			})
			.catch((err) => {
				if (key !== documentKey) return;
				loadError =
					err instanceof CdmApiError
						? err.message
						: err instanceof Error
							? err.message
							: String(err);
			})
			.finally(() => {
				if (key === documentKey) loading = false;
			});
	});
</script>

{#if loading}
	<p>Loading document…</p>
{:else if loadError}
	<p class="cdm-load-error">{loadError}</p>
{:else if doc}
	<DocumentEditor initialDocument={doc} />
{/if}

<style>
	.cdm-load-error {
		color: #b91c1c;
	}
</style>
