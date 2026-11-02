import { mount } from 'svelte';
const Harness = new URLSearchParams(location.search).has('full')
	? (await import('./FullHarness.svelte')).default
	: (await import('./Harness.svelte')).default;
mount(Harness, { target: document.getElementById('app')! });
