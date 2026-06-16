import type { KbSearchHistorySnapshot } from '$lib/services/kbArtifactSearch';

// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		interface PageState {
			kbSearchResults?: KbSearchHistorySnapshot;
		}
		// interface Platform {}
	}
}

export {};
