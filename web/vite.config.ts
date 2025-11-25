import { paraglideVitePlugin } from '@inlang/paraglide-js';
import devtoolsJson from 'vite-plugin-devtools-json';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

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
        		target: 'http://localhost:8080',
        		changeOrigin: true,
        		secure: false,
      		},
			'/auth': {
				target: 'http://localhost:8080',
				changeOrigin: true,
				secure: false,
	  		},
			'/shared_api': {
        		target: 'http://localhost:8080',
        		changeOrigin: true,
			}
    	}
  	},
});
