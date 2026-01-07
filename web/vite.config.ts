import { paraglideVitePlugin } from '@inlang/paraglide-js';
import devtoolsJson from 'vite-plugin-devtools-json';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Read from .env (or process.env)
const API_BASE_URL = process.env.API_BASE_URL || 'http://localhost:8088';

export default defineConfig({
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
    	hmr: {
      		// Connect HMR WebSocket to :5173 directly
      		host: 'localhost',
      		port: 5173,
      		protocol: 'ws'
    	},
    	proxy: {
      		'/api': {
        		target: API_BASE_URL,
        		changeOrigin: true,
        		secure: false,
      		},
			'/auth': {
				target: API_BASE_URL,
				changeOrigin: true,
				secure: false,
	  		},
			'/shared_api': {
        		target: API_BASE_URL,
        		changeOrigin: true,
			}
    	}
  	},
});
