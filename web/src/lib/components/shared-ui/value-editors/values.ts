// How the value editors read typed text and change structured values. Pure, so
// the rules are tested without a browser (OpenSpec wpi-property-editors, D4).
import type { Json, JsonObject } from '$lib/wpi/types';

function parseDefault(text: string): Json {
	const t = text.trim();
	try {
		return JSON.parse(t) as Json;
	} catch {
		return t;
	}
}

// The text a value is shown as; reading it back gives the same value, so a
// string that would read as something else ("200", "true") is shown quoted.
export function jsonText(value: Json | undefined): string {
	if (value === undefined) return '';
	if (typeof value === 'string' && parseDefault(value) === value) return value;
	return JSON.stringify(value);
}

// What typed text means: a value (undefined unsets) or nothing valid.
export type Parsed = { ok: true; value: Json | undefined } | { ok: false };

const invalid: Parsed = { ok: false };

// A length: an allowed keyword, or a non-negative number with an allowed unit;
// a number without a unit means px. A percentage is at most 100.
export function parseLength(text: string, keywords: string[], units: string[]): Parsed {
	const t = text.trim();
	if (t === '') return { ok: true, value: undefined };
	if (keywords.includes(t)) return { ok: true, value: t };
	const m = /^(\d+(?:\.\d+)?)\s*(px|rem|em|%)?$/.exec(t);
	if (!m) return invalid;
	const unit = m[2] ?? 'px';
	if (!units.includes(unit) || (unit === '%' && Number(m[1]) > 100)) return invalid;
	return { ok: true, value: `${m[1]}${unit}` };
}

export function lengthParts(value: Json | undefined): { amount: string; type: string } {
	if (typeof value !== 'string') return { amount: '', type: '' };
	const match = /^(\d+(?:\.\d+)?)(px|rem|em|%)$/.exec(value);
	return match ? { amount: match[1], type: match[2] } : { amount: '', type: value };
}

export function lengthWithType(value: Json | undefined, type: string, units: string[]): string | undefined {
	if (type === 'default' || type === '') return undefined;
	return units.includes(type) ? `${lengthParts(value).amount || '0'}${type}` : type;
}

export function parseNumber(text: string, opts: { min?: number; max?: number; integer?: boolean }): Parsed {
	const t = text.trim();
	if (t === '') return { ok: true, value: undefined };
	if (!/^-?\d+(\.\d+)?$/.test(t)) return invalid;
	const n = Number(t);
	if ((opts.integer && !Number.isInteger(n)) || (opts.min !== undefined && n < opts.min) || (opts.max !== undefined && n > opts.max))
		return invalid;
	return { ok: true, value: n };
}

// Typed text: empty unsets, anything else is kept as typed.
export function parseText(text: string): Parsed {
	return { ok: true, value: text === '' ? undefined : text };
}

// JSON when it parses, otherwise the text itself; empty unsets.
export function parseJsonText(text: string): Parsed {
	return { ok: true, value: text.trim() === '' ? undefined : parseDefault(text) };
}

// Comma-separated words, kept in order, blanks dropped.
export function parseStringList(text: string): Parsed {
	const items = text.split(',').map((s) => s.trim()).filter((s) => s !== '');
	return { ok: true, value: items.length ? items : undefined };
}

export function stringListText(value: Json | undefined): string {
	return Array.isArray(value) ? value.map(String).join(', ') : value === undefined ? '' : String(value);
}

// A colour is unset, one CSS colour, or a light and a dark one.
export type ColorMode = 'none' | 'single' | 'theme';
export type ColorValue = string | { light: string; dark: string } | undefined;

export function colorMode(value: Json | undefined): ColorMode {
	if (value === undefined || value === null || value === '') return 'none';
	return typeof value === 'object' ? 'theme' : 'single';
}

export function colorPart(value: Json | undefined, variant: 'single' | 'light' | 'dark'): string {
	if (variant === 'single') return typeof value === 'string' ? value : '';
	return value && typeof value === 'object' && !Array.isArray(value) && typeof value[variant] === 'string' ? (value[variant] as string) : '';
}

// The value after switching mode, keeping the colour already chosen.
export function withColorMode(value: Json | undefined, mode: ColorMode): ColorValue {
	const first = typeof value === 'string' ? (value === 'auto' ? '' : value) : colorPart(value, 'light');
	if (mode === 'none') return undefined;
	if (mode === 'single') return first || '#000000';
	return { light: first || '#ffffff', dark: colorPart(value, 'dark') || first || '#000000' };
}

export function withColorPart(value: Json | undefined, variant: 'single' | 'light' | 'dark', color: string): ColorValue {
	if (variant === 'single') return color || undefined;
	return {
		light: variant === 'light' ? color : colorPart(value, 'light'),
		dark: variant === 'dark' ? color : colorPart(value, 'dark')
	};
}

// Localized text is one string, or one text per language.
export function isLanguageMap(value: Json | undefined): value is JsonObject {
	return !!value && typeof value === 'object' && !Array.isArray(value);
}

export function withLanguages(value: Json | undefined, localized: boolean): Json | undefined {
	if (localized) return { en: typeof value === 'string' ? value : '' };
	return isLanguageMap(value) && typeof value.en === 'string' ? value.en || undefined : value;
}

// One language's text set; an empty one is left out, so the other languages can be
// filled in one at a time without storing empty translations.
export function withTranslation(value: Json | undefined, lang: string, text: string): JsonObject {
	const map: JsonObject = isLanguageMap(value) ? { ...value } : { en: typeof value === 'string' ? value : '' };
	if (text === '' && lang !== 'en') delete map[lang];
	else map[lang] = text;
	return map;
}

// The list with the item at `at` moved by one place up (-1) or down (1).
export function moved<T>(list: T[], at: number, by: -1 | 1): T[] {
	const to = at + by;
	if (to < 0 || to >= list.length) return list;
	const out = [...list];
	[out[at], out[to]] = [out[to], out[at]];
	return out;
}
