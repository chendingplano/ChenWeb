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

export default defineConfig({
	envDir: '..',
	plugins: [
		tailwindcss(),
		sveltekit(),
		devtoolsJson(),
		paraglideVitePlugin({
			project: './project.inlang',
			outdir: './src/lib/paraglide'
		})
	],
	worker: {
		format: 'es'
	},
	server: {
		port: 5173,
		allowedHosts: ['macmini.deepdocs.me'],
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
