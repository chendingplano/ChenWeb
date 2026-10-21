import { browser } from '$app/environment';
import {
	getLocale,
	setLocale,
	overwriteGetLocale,
	overwriteSetLocale,
	type Locale
} from '$lib/paraglide/runtime';

let locale = $state<Locale>();

// Paraglide messages read getLocale() inside Svelte render effects. Making that
// getter reactive updates mounted components without discarding their UI state.
export function initializeClientLocale() {
	if (!browser || locale !== undefined) return;
	locale = getLocale();
	const persistLocale = setLocale;
	overwriteGetLocale(() => locale!);
	overwriteSetLocale(async (next) => {
		await persistLocale(next, { reload: false });
		locale = next;
		document.documentElement.lang = next;
	});
	document.documentElement.lang = locale;
}
