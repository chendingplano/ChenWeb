import { paraglideVitePlugin } from '@inlang/paraglide-js';
import devtoolsJson from 'vite-plugin-devtools-json';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Read from .env (or process.env)
const APP_BASE_URL = process.env.APP_BASE_URL;
if (typeof APP_BASE_URL !== "string" || APP_BASE_URL.length <= 0) {
	console.error("missing APP_BASE_URL")
}
const APP_HOST = APP_BASE_URL ? new URL(APP_BASE_URL).hostname : undefined;

export default defineConfig({
	envDir: '..',
	plugins: [
		tailwindcss(),
		sveltekit(),
		devtoolsJson(),
		paraglideVitePlugin({
			project: './project.inlang',
			outdir: './src/lib/paraglide',
			// Thousands of per-message modules exhaust Rollup's heap during the
			// combined SSR/client build. Match the dev server's locale modules.
			outputStructure: 'locale-modules',
			// Builds can run alongside the dev server. Keep its generated imports
			// available while Paraglide rewrites the output.
			cleanOutdir: false,
			// "cookie" persists a visitor's explicit language choice (set via
			// setLocale() in the language switcher) across reloads and future
			// visits. New visitors with no cookie yet still land on baseLocale
			// (zh-cn, from project.inlang/settings.json) until they pick one.
			strategy: ['cookie', 'baseLocale']
		})
	],
	worker: {
		format: 'es'
	},
	build: {
		rollupOptions: {
			// onLog, not onwarn: SvelteKit sets its own onwarn, which replaces ours.
			onLog(level, log, handler) {
				// Svelte's server compiler drops actions and other browser-only code,
				// so a Svelte library's imports such as `use:clickOutside` from
				// @svar-ui/lib-dom are unused in the SSR bundle. Rollup reports each
				// one; they are not actionable here. Our own code is still reported.
				if (
					log.code === 'UNUSED_EXTERNAL_IMPORT' &&
					log.ids?.every((id) => id.includes('/node_modules/'))
				) {
					return;
				}
				handler(level, log);
			}
		}
	},
	server: {
		port: 5173,
		allowedHosts: APP_HOST ? [APP_HOST] : [],
    	hmr: {
      		// Connect HMR WebSocket to :5173 directly
      		host: 'localhost',
      		port: 5173,
      		protocol: 'ws'
    	},
    	proxy: {
      		'/api': {
        		target: APP_BASE_URL,
        		changeOrigin: true,
        		secure: false,
      		},
			'/auth': {
				target: APP_BASE_URL,
				changeOrigin: true,
				secure: false,
	  		},
			'/shared_api': {
        		target: APP_BASE_URL,
        		changeOrigin: true,
			},
			'/kratos': {
				target: 'http://127.0.0.1:4433',
				changeOrigin: true,
				secure: false,
				rewrite: (path) => path.replace(/^\/kratos/, ''),
			}
    	}
  	},
});
