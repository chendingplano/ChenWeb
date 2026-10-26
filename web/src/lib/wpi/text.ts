// Translated messages for save error codes returned by the server.
import { m } from '$lib/paraglide/messages.js';

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
