import sveltePreprocess from 'svelte-preprocess';

export default {
    preprocess: sveltePreprocess(),
    kit: {
        // Specify the adapter for deployment
        adapter: {
            name: '@sveltejs/adapter-auto',
            options: {}
        },
        // Other kit options can be added here
    }
};