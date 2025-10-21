import adapter from '@sveltejs/adapter-static';
import { resolve } from 'node:path';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://svelte.dev/docs/kit/integrations
	// for more information about preprocessors
	preprocess: vitePreprocess(),
	kit: { 
		adapter: adapter(),
		alias: {
			'@svar/core': resolve('../libs/svelte-core/svelte/src'),
			'@svar/editor': resolve('../libs/svelte-editor/svelte/src'),
			'@svar/grid': resolve('../libs/svelte-grid/svelte/src')
		}
	}
};

export default config;
