// Light/dark mode for the SemOS frontend. Uses the existing `.dark` class
// contract from app.css (ADR 2026071102: site-wide toggle, both modes v1).

const STORAGE_KEY = 'semos-theme';

class SemosTheme {
	mode = $state<'light' | 'dark'>('light');

	/**
	 * Call once from the /semos layout (browser only). The `.dark` class
	 * itself is already applied by the blocking inline script in
	 * app.html (before hydration, to avoid a flash) — this only syncs the
	 * reactive `mode` state so the header's sun/moon icon matches on
	 * first paint.
	 */
	init() {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored === 'dark' || stored === 'light') {
			this.mode = stored;
		} else {
			this.mode = window.matchMedia('(prefers-color-scheme: dark)').matches
				? 'dark'
				: 'light';
		}
	}

	toggle() {
		this.mode = this.mode === 'dark' ? 'light' : 'dark';
		localStorage.setItem(STORAGE_KEY, this.mode);
		this.apply();
	}

	private apply() {
		document.documentElement.classList.toggle('dark', this.mode === 'dark');
	}
}

export const semosTheme = new SemosTheme();
