// Light/dark mode for the whole ChenWeb frontend — one source of truth, so the
// mode a user picks on any page (e.g. /semos) is the mode every other page
// (e.g. /home3) opens in. Uses the `.dark` class contract from app.css
// (ADR 2026071102: site-wide toggle, both modes v1).

import { browser } from '$app/environment';

const STORAGE_KEY = 'chenweb-theme';

export type ThemeMode = 'light' | 'dark';

function storedMode(): ThemeMode {
	if (!browser) return 'light';
	const stored = localStorage.getItem(STORAGE_KEY);
	if (stored === 'dark' || stored === 'light') return stored;
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

class Theme {
	// Seeded at module load so the first paint already matches the stored mode,
	// the same value the blocking script in app.html used for the `.dark` class.
	mode = $state<ThemeMode>(storedMode());

	get isDark() {
		return this.mode === 'dark';
	}

	/** Sync after hydration and follow changes from other same-origin frames/tabs. */
	init() {
		this.mode = storedMode();
		this.apply();
		const sync = (event: StorageEvent) => {
			if (event.storageArea !== localStorage || (event.key !== STORAGE_KEY && event.key !== null)) return;
			this.mode = storedMode();
			this.apply();
		};
		window.addEventListener('storage', sync);
		return () => window.removeEventListener('storage', sync);
	}

	toggle() {
		this.set(this.isDark ? 'light' : 'dark');
	}

	set(mode: ThemeMode) {
		this.mode = mode;
		localStorage.setItem(STORAGE_KEY, mode);
		this.apply();
	}

	private apply() {
		document.documentElement.classList.toggle('dark', this.isDark);
	}
}

export const theme = new Theme();
