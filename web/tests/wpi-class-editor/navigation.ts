// The standalone harness has no SvelteKit router. Keep editor registration real.
export function beforeNavigate(callback: (event: { cancel: () => void }) => void) {
	(window as unknown as { testBeforeNavigate: typeof callback }).testBeforeNavigate = callback;
}
