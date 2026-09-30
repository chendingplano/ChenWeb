// i18n check for the ChenWeb frontend (ADR 2026093001, openspec change
// i18n-paraglide-standard). Two checks:
//
// 1. Message parity (hard failure): messages/en.json and messages/zh-cn.json must
//    have the same keys, and no value may be empty.
// 2. Hard-coded text (ratchet): user-visible text written directly in .svelte
//    markup (text nodes, and placeholder/title/aria-label/alt/label attributes),
//    and text-like string literals inside markup expressions
//    (`{busy ? 'Saving…' : 'Save'}`, `title={`Open ${x}`}`, `{#each ['Low', 'High']}`)
//    instead of through m.*(). Existing files are allowed their current count,
//    recorded in i18n-baseline.json; a new file with any hard-coded text, or an
//    existing file whose count grows, fails. Text inside <code>/<pre> is ignored.
//    Strings in <script> are not scanned.
//
// Usage (from web/):
//   bun scripts/check-i18n.ts            run both checks
//   bun scripts/check-i18n.ts --list F   print the hard-coded text found in file F
//   bun scripts/check-i18n.ts --update   rewrite the baseline to the current counts
//                                        (after converting pages; never to hide new text)

import { readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { parse } from 'svelte/compiler';

const ROOT = join(import.meta.dirname, '..');
const BASELINE = join(ROOT, 'i18n-baseline.json');
const LOCALES = ['en', 'zh-cn'];
const TEXT_ATTRS = new Set(['placeholder', 'title', 'aria-label', 'alt', 'label']);
const SKIP_ELEMENTS = new Set(['code', 'pre']);
// Two or more Latin letters in a row, or any CJK character.
const WORDY = /[A-Za-z]{2,}|[㐀-鿿]/;

// Language self-names are shown in their own language on purpose.
const ALLOWED_LITERALS = new Set(['English', '中文', '简体中文']);

type Finding = { line: number; text: string };
type JsNode = Record<string, unknown> & { type: string; start: number; end: number };

// Whether a string literal in code reads as display text rather than an id, key,
// CSS, path or format string. Stricter than isHardcodedText, because code is full
// of short identifiers.
export function looksLikeText(t: string): boolean {
	const v = t.trim();
	if (ALLOWED_LITERALS.has(v)) return false;
	if (!/[A-Za-z]{2,}|[\u3400-\u9fff]/.test(v)) return false;
	if (/^(https?:|\/|\.\/|#|\$lib)/.test(v)) return false;
	if (/(^|\s)(px|rem|em|rgba?|hsla?|var\(--)|#[0-9a-f]{3,8}\b|\b\d+(px|rem|ms|s)\b/i.test(v))
		return false;
	if (/[;{}]/.test(v)) return false;
	if (/^[a-z0-9_.:/@-]+$/.test(v)) return false; // ids, keys, enum codes, paths
	if (/^[a-z][a-zA-Z0-9]*$/.test(v)) return false; // camelCase identifiers
	if (/^[A-Z0-9_]+$/.test(v)) return false; // CONSTANTS, env names
	if (/^[a-z]{2,3}([-_][A-Za-z]{2,4})+$/.test(v)) return false; // locales: en-US, zh-Hans
	if (/^[YMDHhmsSaAZ]+([-/:. ][YMDHhmsSaAZ]+)+$/.test(v)) return false; // YYYY-MM-DD, HH:mm
	// CSS class lists: lowercase tokens, at least half of them with '-', ':' or a digit.
	const toks = v.split(/\s+/);
	if (
		toks.every((t) => /^[a-z0-9:/_.[\]!-]+$/.test(t)) &&
		toks.filter((t) => /[-:0-9]/.test(t)).length * 2 >= toks.length
	)
		return false;
	return true;
}

// Text-like string literals inside a markup expression. Skips comparisons
// (`=== 'done'`), object keys, member names and call arguments (format strings,
// keys, and the arguments of m.*() itself).
export function textLiteralsIn(expr: unknown): JsNode[] {
	const out: JsNode[] = [];
	const walk = (node: unknown, parent: JsNode | null, key: string): void => {
		if (Array.isArray(node)) return node.forEach((x) => walk(x, parent, key));
		if (!node || typeof node !== 'object') return;
		const n = node as JsNode;
		if (!n.type) return;
		if (parent) {
			if (
				parent.type === 'BinaryExpression' &&
				['===', '!==', '==', '!=', 'in'].includes(String(parent.operator))
			)
				return;
			if (parent.type === 'Property' && key === 'key') return;
			if (parent.type === 'MemberExpression' && key === 'property') return;
			if (
				(parent.type === 'CallExpression' || parent.type === 'NewExpression') &&
				key === 'arguments'
			)
				return;
			if (parent.type === 'TaggedTemplateExpression') return;
		}
		if (n.type === 'Literal') {
			if (typeof n.value === 'string' && looksLikeText(n.value)) out.push(n);
			return;
		}
		if (n.type === 'TemplateLiteral') {
			const quasiText = (n.quasis as JsNode[])
				.map((q) => String((q.value as Record<string, unknown>).cooked ?? ''))
				.join(' ');
			if (looksLikeText(quasiText)) out.push(n);
			else for (const e of n.expressions as JsNode[]) walk(e, n, 'expressions');
			return;
		}
		for (const [k, v] of Object.entries(n)) {
			if (k === 'metadata' || k === 'loc' || k === 'range') continue;
			if (v && typeof v === 'object') walk(v, n, k);
		}
	};
	walk(expr, null, '');
	return out;
}

// Attributes whose expression values are never display text.
const CODE_ATTR = (name: string) => name === 'class' || name === 'style' || name.startsWith('on');

export function isHardcodedText(raw: string): boolean {
	const t = raw.replace(/&[a-z]+;|&#\d+;/gi, ' ').trim();
	return t !== '' && WORDY.test(t);
}

export function findHardcodedText(source: string): Finding[] {
	const ast = parse(source, { modern: true }) as unknown as { fragment: unknown };
	const out: Finding[] = [];
	const lineOf = (pos: number) => source.slice(0, pos).split('\n').length;

	const visit = (node: unknown, skip: boolean): void => {
		if (Array.isArray(node)) {
			for (const n of node) visit(n, skip);
			return;
		}
		if (!node || typeof node !== 'object') return;
		const n = node as Record<string, unknown> & { type?: string; start?: number };
		if (n.type === 'Text') {
			if (!skip && isHardcodedText(String(n.data)))
				out.push({ line: lineOf(n.start ?? 0), text: String(n.data).trim() });
			return;
		}
		const literals = (expr: unknown) => {
			if (skip) return;
			for (const l of textLiteralsIn(expr))
				out.push({ line: lineOf(l.start), text: source.slice(l.start, l.end) });
		};
		if (n.type === 'ExpressionTag') {
			literals(n.expression);
			return;
		}
		if (n.type === 'Attribute') {
			const name = String(n.name);
			if (CODE_ATTR(name)) return;
			const parts = (
				Array.isArray(n.value) ? n.value : n.value && n.value !== true ? [n.value] : []
			) as Record<string, unknown>[];
			for (const part of parts) {
				if (part.type === 'ExpressionTag') literals(part.expression);
				else if (
					TEXT_ATTRS.has(name) &&
					part.type === 'Text' &&
					isHardcodedText(String(part.data))
				) {
					out.push({
						line: lineOf(Number(part.start ?? 0)),
						text: `${name}="${String(part.data).trim()}"`
					});
				}
			}
			return;
		}
		if (n.type === 'EachBlock') literals(n.expression);
		const childSkip = skip || (n.type === 'RegularElement' && SKIP_ELEMENTS.has(String(n.name)));
		for (const [k, v] of Object.entries(n)) {
			// JS expressions and compiler metadata are not markup text.
			if (k === 'expression' || k === 'metadata' || k === 'context' || k === 'key' || k === 'index')
				continue;
			if (v && typeof v === 'object') visit(v, childSkip);
		}
	};
	visit(ast.fragment, false);
	return out;
}

export function checkParity(messages: Record<string, Record<string, unknown>>): string[] {
	const errors: string[] = [];
	const all = new Set(
		Object.values(messages).flatMap((m) => Object.keys(m).filter((k) => k !== '$schema'))
	);
	for (const key of [...all].sort()) {
		for (const loc of Object.keys(messages)) {
			const v = messages[loc][key];
			if (v === undefined) errors.push(`message "${key}" is missing in ${loc}.json`);
			else if (typeof v === 'string' && v.trim() === '')
				errors.push(`message "${key}" is empty in ${loc}.json`);
		}
	}
	return errors;
}

function svelteFiles(dir: string): string[] {
	const out: string[] = [];
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) {
			if (name !== 'node_modules' && name !== 'paraglide') out.push(...svelteFiles(p));
		} else if (name.endsWith('.svelte')) out.push(p);
	}
	return out.sort();
}

function scan(): Record<string, Finding[]> {
	const found: Record<string, Finding[]> = {};
	for (const f of svelteFiles(join(ROOT, 'src'))) {
		const findings = findHardcodedText(readFileSync(f, 'utf8'));
		if (findings.length) found[relative(ROOT, f)] = findings;
	}
	return found;
}

function main(argv: string[]): number {
	if (argv[0] === '--list') {
		const file = argv[1];
		if (!file) {
			console.error('usage: check-i18n.ts --list <file.svelte>');
			return 2;
		}
		for (const f of findHardcodedText(readFileSync(file, 'utf8')))
			console.log(`${file}:${f.line}\t${f.text}`);
		return 0;
	}

	const found = scan();
	if (argv[0] === '--update') {
		const counts = Object.fromEntries(Object.entries(found).map(([f, v]) => [f, v.length]));
		writeFileSync(BASELINE, JSON.stringify(counts, null, '\t') + '\n');
		console.log(`i18n baseline updated: ${Object.keys(counts).length} files with hard-coded text`);
		return 0;
	}

	let failed = false;
	const messages = Object.fromEntries(
		LOCALES.map((l) => [l, JSON.parse(readFileSync(join(ROOT, 'messages', `${l}.json`), 'utf8'))])
	);
	for (const e of checkParity(messages)) {
		console.error(`i18n: ${e}`);
		failed = true;
	}

	const baseline: Record<string, number> = JSON.parse(readFileSync(BASELINE, 'utf8'));
	const improved: string[] = [];
	for (const [file, findings] of Object.entries(found)) {
		const allowed = baseline[file] ?? 0;
		if (findings.length > allowed) {
			failed = true;
			console.error(
				`i18n: ${file} has ${findings.length} hard-coded text item(s), baseline allows ${allowed}. ` +
					`Use m.*() with en + zh-cn messages. Found:`
			);
			for (const f of findings) console.error(`  ${file}:${f.line}\t${f.text}`);
		} else if (findings.length < allowed) improved.push(file);
	}
	for (const file of Object.keys(baseline)) if (!found[file]) improved.push(file);
	if (improved.length) {
		console.log(
			`i18n: ${improved.length} file(s) now have less hard-coded text than the baseline; run --update to lock that in.`
		);
	}
	if (!failed) console.log('i18n: ok');
	return failed ? 1 : 0;
}

if (import.meta.main) process.exit(main(process.argv.slice(2)));
