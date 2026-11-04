import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { fileURLToPath } from 'node:url';
export default defineConfig({
	root: fileURLToPath(new URL('.', import.meta.url)),
	plugins: [svelte()],
	resolve: { alias: { $lib: fileURLToPath(new URL('../../src/lib', import.meta.url)) } },
	server: {
		host: '127.0.0.1',
		port: 5198,
		strictPort: true,
		fs: { allow: [fileURLToPath(new URL('../../..', import.meta.url))] }
	}
});
