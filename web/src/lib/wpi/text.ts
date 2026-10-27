// Translated messages for save error codes returned by the server, and text
// written in descriptors.
import { m } from '$lib/paraglide/messages.js';
import { getLocale } from '$lib/paraglide/runtime';
import { textIn } from './entries';
import type { Text } from './types';

// A descriptor's text in the current language, else English. getLocale is
// reactive (stores/locale.svelte.ts), so markup using this follows switches.
export function localized(text: Text | undefined): string {
	return textIn(text, getLocale());
}

export function fieldErrorText(code: string): string {
	switch (code) {
		case 'required':
			return m.wpi_err_required();
		case 'max_length':
			return m.wpi_err_max_length();
		case 'pattern':
			return m.wpi_err_pattern();
		case 'format':
			return m.wpi_err_format();
	}
	return m.wpi_err_unknown({ code });
}
