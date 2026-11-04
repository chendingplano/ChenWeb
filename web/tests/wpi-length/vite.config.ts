import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'node:path';
export default defineConfig({root:'tests/wpi-length', plugins:[svelte()], resolve:{alias:{'$lib':resolve('src/lib')}}, server:{port:5187}});
