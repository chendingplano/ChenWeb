// Whitespace, line breaks, lengths and patterns for the validators, the twin of
// server/api/wpi/validators/text.go (OpenSpec wpi-primitive-validators, D11, D12).

// The one whitespace set both languages use: the Unicode White_Space characters
// and U+FEFF. String.prototype.trim and Go's strings.TrimSpace differ, so
// neither is used.
const spaces = new Set([0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x20, 0x85, 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff]);

export function isSpace(ch: string): boolean {
	const cp = ch.codePointAt(0)!;
	return spaces.has(cp) || (cp >= 0x2000 && cp <= 0x200a);
}

export const isLineBreak = (ch: string) => ch === '\n' || ch === '\r' || ch === ' ' || ch === ' ';

// Removes whitespace at the start, end or both ("none" keeps it).
export function trimSpace(s: string, mode: string): string {
	if (mode !== 'both' && mode !== 'start' && mode !== 'end') return s;
	const chars = Array.from(s);
	let start = 0;
	let end = chars.length;
	if (mode !== 'end') while (start < end && isSpace(chars[start])) start++;
	if (mode !== 'start') while (end > start && isSpace(chars[end - 1])) end--;
	return chars.slice(start, end).join('');
}

export function normalize(s: string, form: string): string {
	const forms: Record<string, string> = { nfc: 'NFC', nfd: 'NFD', nfkc: 'NFKC', nfkd: 'NFKD' };
	return forms[form] ? s.normalize(forms[form]) : s;
}

// Code points (the default, §2.6.7), UTF-16 code units or UTF-8 bytes.
export function textLength(s: string, unit: string): number {
	if (unit === 'utf8_bytes') return new TextEncoder().encode(s).length;
	if (unit === 'utf16_units') return s.length;
	return Array.from(s).length;
}

// The longest value, in code points, a pattern is matched against.
export const PATTERN_LIMIT = 10000;

// Syntax RE2 and ECMAScript read differently, as the server's PortablePattern.
const nonPortable = /\(\?[a-zA-Z<]|\\[AzQEpP]/;

const compiled = new Map<string, RegExp | null>();

function wholePattern(p: string): RegExp | null {
	if (!compiled.has(p)) {
		let re: RegExp | null = null;
		try {
			re = new RegExp(`^(?:${p})$`, 'u');
		} catch {
			re = null;
		}
		compiled.set(p, re);
	}
	return compiled.get(p)!;
}

// Whether the whole of s matches p; a pattern that does not compile matches nothing.
export function matchesWhole(p: string, s: string): boolean {
	return wholePattern(p)?.test(s) ?? false;
}

// Why a pattern cannot be used in the browser, or null; the server also checks
// that Go reads it.
export function patternProblem(p: string): 'invalid' | 'non_portable' | null {
	if (!wholePattern(p)) return 'invalid';
	return nonPortable.test(p) ? 'non_portable' : null;
}
