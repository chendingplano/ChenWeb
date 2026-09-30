// Converts hard-coded text in .svelte files to Paraglide messages (ADR 2026093001).
// Companion to check-i18n.ts: it rewrites exactly what that check reports.
//
// For each file it:
//  - replaces runs of markup text (with any {expressions} inside them) by one
//    message call; expressions become named parameters, e.g.
//    `Reviewing {review.metrics_count} metrics…` -> {m.x_reviewing_metrics({ metrics_count: review.metrics_count })}
//  - replaces placeholder/title/aria-label/alt/label attribute text the same way;
//  - replaces text-like string literals inside markup expressions
//    (busy ? "Saving" : "Save"), as selected by check-i18n.ts textLiteralsIn;
//  - adds `import { m } from '$lib/paraglide/messages.js'`;
//  - adds each new key to messages/en.json and messages/zh-cn.json with the source
//    text in both, and appends it to a pending-translation list. The list must be
//    translated (translate the zh-cn copy of English text, or the en copy of
//    Chinese text) and applied with --apply before committing.
//
// A file that already uses `m` as a local name imports the messages as `msg`.
//
// Usage (from web/):
//   bun scripts/i18n-extract.ts --pending out.json [--dry-run] <file.svelte>...   markup text
//   bun scripts/i18n-extract.ts --script-candidates <file.svelte>... > cands.json  <script> strings
//   (review cands.json: delete entries that are not displayed text)
//   bun scripts/i18n-extract.ts --script-apply cands.json pending.json
//   bun scripts/i18n-extract.ts --suggest pending.json > tm.json   (translation memory)
//   bun scripts/i18n-extract.ts --fix-params   (after conversion: make message params non-null)
//   bun scripts/i18n-extract.ts --apply translations.json
//       translations.json: { "<key>": { "en"?: "...", "zh-cn"?: "..." } }

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { basename, join } from 'node:path';
import { parse } from 'svelte/compiler';
import { isHardcodedText, isTextAttr, looksLikeText, textLiteralsIn } from './check-i18n.ts';

const ROOT = join(import.meta.dirname, '..');
const MSG = (l: string) => join(ROOT, 'messages', `${l}.json`);
const SKIP_ELEMENTS = new Set(['code', 'pre']);
const CJK = /[㐀-鿿]/;
const importLine = (alias: string) =>
	`import { ${alias === 'm' ? 'm' : `m as ${alias}`} } from '$lib/paraglide/messages.js';`;

type Node = Record<string, unknown> & { type: string; start: number; end: number };
export type Pending = { key: string; text: string; source_lang: 'en' | 'zh-cn'; file: string };
type Edit = { start: number; end: number; text: string };

const ENTITIES: Record<string, string> = {
	nbsp: ' ',
	amp: '&',
	lt: '<',
	gt: '>',
	quot: '"',
	apos: "'",
	mdash: '—',
	ndash: '–',
	hellip: '…',
	rarr: '→',
	larr: '←',
	times: '×',
	middot: '·',
	bull: '•',
	copy: '©'
};

export function decodeEntities(s: string): string {
	return s.replace(/&(#\d+|#x[0-9a-f]+|[a-z]+);/gi, (all, e: string) => {
		if (e[0] === '#')
			return String.fromCodePoint(
				e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10)
			);
		return ENTITIES[e.toLowerCase()] ?? all;
	});
}

export function keyPrefix(file: string): string {
	return basename(file, '.svelte')
		.replace(/-view$/, '')
		.replace(/[^a-z0-9]+/gi, '_')
		.toLowerCase();
}

function slug(text: string): string {
	const words =
		text
			.toLowerCase()
			.replace(/\{[^}]*\}/g, ' ')
			.match(/[a-z0-9]+/g) ?? [];
	return words.slice(0, 5).join('_') || 'text';
}

type Expr = Record<string, unknown>;
const isStr = (e: unknown, v?: string) =>
	(e as Expr)?.type === 'Literal' &&
	typeof (e as Expr).value === 'string' &&
	(v === undefined || (e as Expr).value === v);

// Parameter name for an expression, named after the value it shows:
//   x -> x, a.b.name -> name, items.length -> itemsCount, fmtTime(t) -> t,
//   n.toLocaleString() -> n, Math.round(ms) -> ms, n === 1 ? '' : 's' -> plural.
function paramName(expr: Expr): string {
	switch (expr.type) {
		case 'Identifier':
			return String(expr.name);
		case 'MemberExpression': {
			if (expr.computed) return paramName(expr.object as Expr);
			const prop = String((expr.property as Expr).name);
			if (prop === 'length' || prop === 'size') {
				const base = paramName(expr.object as Expr);
				return base === 'value' ? 'count' : `${base}Count`;
			}
			return prop;
		}
		case 'CallExpression': {
			const args = expr.arguments as Expr[];
			if (args.length && args[0].type !== 'Literal') return paramName(args[0]);
			const callee = expr.callee as Expr;
			if (callee.type === 'MemberExpression') {
				const obj = paramName(callee.object as Expr);
				return obj === 'Math' ? 'value' : obj;
			}
			return 'value';
		}
		case 'ConditionalExpression':
			if (
				(isStr(expr.consequent, '') && isStr(expr.alternate, 's')) ||
				(isStr(expr.consequent, 's') && isStr(expr.alternate, ''))
			)
				return 'plural';
			return 'value';
		case 'LogicalExpression':
			return paramName(expr.left as Expr);
		case 'ChainExpression':
		case 'TSNonNullExpression':
			return paramName(expr.expression as Expr);
		default:
			return 'value';
	}
}

// Name under which a file calls the messages: its existing Paraglide import, else
// `m`, or `msg` when the file already uses `m` as a local name (e.g. for metrics).
export function messagesAlias(source: string): string {
	const existing = source.match(
		/import\s*\{\s*m(?:\s+as\s+(\w+))?\s*\}\s*from\s*['"]\$lib\/paraglide\/messages(?:\.js)?['"]/
	);
	if (existing) return existing[1] ?? 'm';
	if (!usesLocalM(source)) return 'm';
	return /\bmsg\b/.test(source) ? 'i18n' : 'msg';
}

export function usesLocalM(source: string): boolean {
	return /(\(|,\s*)m\s*(\)|,|=>)|\bm\s*=>|\bas\s+m\b|\b(const|let|var)\s+m\b|\{@const\s+m\b|\bfunction\s*\w*\s*\(\s*m\b/.test(
		source
	);
}

type Ctx = {
	alias: string;
	source: string;
	prefix: string;
	keys: Map<string, string>;
	used: Set<string>;
	existing: Record<string, string>;
};

function allocKey(ctx: Ctx, text: string): string {
	let key = ctx.keys.get(text);
	if (!key) {
		const base = `${ctx.prefix}_${slug(text)}`;
		key = base;
		for (
			let i = 2;
			ctx.used.has(key) || (ctx.existing[key] !== undefined && ctx.existing[key] !== text);
			i++
		)
			key = `${base}_${i}`;
		ctx.used.add(key);
		ctx.keys.set(text, key);
	}
	return key;
}

// Edits turning the text-like literals inside a markup expression into m.*() calls
// (same selection as the check: comparisons, keys and call arguments are left alone).
function literalEdits(ctx: Ctx, expr: unknown, dialogsOnly = false): Edit[] {
	return textLiteralsIn(expr, dialogsOnly).map((l) => {
		const node = l as unknown as Node;
		if (node.type === 'TemplateLiteral') {
			const { text, params } = templateText(ctx, node);
			const t = text.replace(/\s+/g, ' ').trim();
			const args = params.length
				? `{ ${params.map(([n, sv]) => (n === sv ? n : `${n}: ${sv}`)).join(', ')} }`
				: '';
			return {
				start: node.start,
				end: node.end,
				text: `${ctx.alias}.${allocKey(ctx, t)}(${args})`
			};
		}
		const t = String(node.value).replace(/\s+/g, ' ').trim();
		return { start: node.start, end: node.end, text: `${ctx.alias}.${allocKey(ctx, t)}()` };
	});
}

// Source of an expression with its text literals already converted.
function exprSource(ctx: Ctx, expr: Node): string {
	let src = ctx.source.slice(expr.start, expr.end);
	for (const e of literalEdits(ctx, expr).sort((a, b) => b.start - a.start)) {
		src = src.slice(0, e.start - expr.start) + e.text + src.slice(e.end - expr.start);
	}
	return src;
}

// Builds the message for a run of Text/ExpressionTag parts and returns the call.
function messageFor(ctx: Ctx, parts: Node[]): { call: string; key: string; text: string } {
	const params: [string, string][] = [];
	let text = '';
	for (const p of parts) {
		if (p.type === 'Text') text += decodeEntities(String(p.data));
		else {
			const expr = p.expression as Node;
			const src = exprSource(ctx, expr);
			let name = paramName(expr);
			const same = params.find(([, s]) => s === src);
			if (same) name = same[0];
			else {
				let n = name,
					i = 2;
				while (params.some(([pn]) => pn === n)) n = `${name}${i++}`;
				name = n;
				params.push([name, src]);
			}
			text += `{${name}}`;
		}
	}
	text = text.replace(/\s+/g, ' ').trim();
	const key = allocKey(ctx, text);
	const args = params.length
		? `{ ${params.map(([n, s]) => (n === s ? n : `${n}: ${s}`)).join(', ')} }`
		: '';
	return { call: `${ctx.alias}.${key}(${args})`, key, text };
}

export function convert(source: string, prefix: string, existing: Record<string, string>) {
	const ast = parse(source, { modern: true }) as unknown as {
		fragment: Node;
		instance?: Node & { content: Node };
	};
	const ctx: Ctx = {
		source,
		prefix,
		keys: new Map(),
		used: new Set(),
		existing,
		alias: messagesAlias(source)
	};
	const edits: Edit[] = [];

	const handleNodes = (nodes: Node[]) => {
		let run: Node[] = [];
		const flush = () => {
			if (run.some((p) => p.type === 'Text' && isHardcodedText(String(p.data)))) {
				const first = run[0],
					last = run[run.length - 1];
				const lead = first.type === 'Text' ? String(first.data).match(/^\s*/)![0] : '';
				const trail = last.type === 'Text' ? String(last.data).match(/\s*$/)![0] : '';
				const { call } = messageFor(ctx, run);
				edits.push({
					start: first.start + lead.length,
					end: last.end - trail.length,
					text: `{${call}}`
				});
			} else {
				for (const p of run)
					if (p.type === 'ExpressionTag') edits.push(...literalEdits(ctx, p.expression));
			}
			run = [];
		};
		for (const n of nodes) {
			if (n.type === 'Text' || n.type === 'ExpressionTag') run.push(n);
			else flush();
		}
		flush();
	};

	let onComponent = false;
	const visit = (node: unknown, skip: boolean): void => {
		if (Array.isArray(node)) return node.forEach((n) => visit(n, skip));
		if (!node || typeof node !== 'object') return;
		const n = node as Node;
		if (n.type === 'Fragment' && Array.isArray(n.nodes) && !skip) handleNodes(n.nodes as Node[]);
		if (n.type === 'Attribute') {
			const name = String(n.name);
			const val = n.value as Node[] | Node | true;
			if (skip || name === 'class' || name === 'style') return;
			if (name.startsWith('on')) {
				// Event handlers are code; only their confirm/alert/prompt text is converted.
				const parts = Array.isArray(val) ? val : val && val !== true ? [val] : [];
				for (const p of parts)
					if (p.type === 'ExpressionTag') edits.push(...literalEdits(ctx, p.expression, true));
				return;
			}
			if (
				isTextAttr(name, onComponent) &&
				Array.isArray(val) &&
				val.some((p) => p.type === 'Text' && isHardcodedText(String(p.data)))
			) {
				const { call } = messageFor(ctx, val);
				edits.push({ start: n.start, end: n.end, text: `${name}={${call}}` });
				return;
			}
			const parts = Array.isArray(val) ? val : val && val !== true ? [val] : [];
			for (const p of parts)
				if (p.type === 'ExpressionTag') edits.push(...literalEdits(ctx, p.expression));
			return;
		}
		if (n.type === 'EachBlock' && !skip) edits.push(...literalEdits(ctx, n.expression));
		const childSkip = skip || (n.type === 'RegularElement' && SKIP_ELEMENTS.has(String(n.name)));
		for (const [k, v] of Object.entries(n)) {
			if (k === 'expression' || k === 'metadata' || k === 'context' || k === 'key' || k === 'index')
				continue;
			if (k === 'attributes' && n.type === 'Component') {
				onComponent = true;
				visit(v, childSkip);
				onComponent = false;
				continue;
			}
			if (v && typeof v === 'object') visit(v, childSkip);
		}
	};
	visit(ast.fragment, false);

	edits.sort((a, b) => b.start - a.start);
	let out = source;
	for (const e of edits) out = out.slice(0, e.start) + e.text + out.slice(e.end);

	if (edits.length) out = ensureImport(out, ctx.alias);
	const messages = [...ctx.keys.entries()].map(([text, key]) => ({ key, text }));
	return { out, edits: edits.length, messages };
}

// --- <script> strings -------------------------------------------------------
// The check cannot tell displayed strings from ids in code, so script strings are
// converted in two steps: list candidates, review them, convert the approved list.

export type ScriptCandidate = { file: string; line: number; text: string };

const SKIP_CALLS =
	/^(console\.\w+|fetch|URLSearchParams|URL|localStorage\.\w+|sessionStorage\.\w+|\w+\.(querySelector\w*|getElementById|addEventListener|removeEventListener|setAttribute|getAttribute|getItem|setItem|removeItem|get|set|has|delete|append|startsWith|endsWith|includes|indexOf|split|replace|replaceAll|match|test|join|padStart|padEnd|toLocaleString|toLocaleDateString|toLocaleTimeString|dispatchEvent)|require|import|goto|encodeURIComponent|decodeURIComponent|parseInt|parseFloat|Number|new\s+Date|Date|getPageConfig|setLocale)$/;

function templateText(ctx: Ctx, tl: Node): { text: string; params: [string, string][] } {
	const quasis = tl.quasis as Node[];
	const exprs = tl.expressions as Node[];
	const params: [string, string][] = [];
	let text = '';
	quasis.forEach((q, i) => {
		text += String((q.value as Record<string, unknown>).cooked ?? '');
		if (i < exprs.length) {
			const src = ctx.source.slice(exprs[i].start, exprs[i].end);
			const same = params.find(([, sv]) => sv === src);
			let name = same ? same[0] : paramName(exprs[i]);
			if (!same) {
				let nn = name,
					k = 2;
				while (params.some(([pn]) => pn === nn)) nn = `${name}${k++}`;
				name = nn;
				params.push([name, src]);
			}
			text += `{${name}}`;
		}
	});
	return { text, params };
}

// Flattens a '+' chain made only of string literals; null otherwise.
function literalChain(n: Node): string | null {
	if (n.type === 'Literal' && typeof n.value === 'string') return n.value;
	if (n.type === 'BinaryExpression' && n.operator === '+') {
		const l = literalChain(n.left as Node),
			r = literalChain(n.right as Node);
		return l !== null && r !== null ? l + r : null;
	}
	return null;
}

type ScriptHit = { node: Node; text: string; params: [string, string][] };

function scriptStrings(source: string): ScriptHit[] {
	const ast = parse(source, { modern: true }) as unknown as { instance?: { content: Node } };
	if (!ast.instance) return [];
	const ctx = { source } as Ctx;
	const hits: ScriptHit[] = [];
	const calleeName = (c: Node): string => source.slice(c.start, c.end).replace(/\s+/g, ' ');
	const visit = (node: unknown, parent: Node | null, parentKey: string): void => {
		if (Array.isArray(node)) return node.forEach((x) => visit(x, parent, parentKey));
		if (!node || typeof node !== 'object') return;
		const n = node as Node;
		if (!n.type) return;
		if (
			n.type === 'ImportDeclaration' ||
			n.type === 'ExportAllDeclaration' ||
			n.type === 'TSTypeAnnotation' ||
			n.type === 'TSTypeAliasDeclaration' ||
			n.type === 'TSInterfaceDeclaration' ||
			(n.type === 'TSAsExpression' && false)
		)
			return;
		if (
			n.type.startsWith('TS') &&
			n.type !== 'TSAsExpression' &&
			n.type !== 'TSNonNullExpression' &&
			n.type !== 'TSSatisfiesExpression'
		)
			return;
		if (parent) {
			if (parent.type === 'Property' && parentKey === 'key') return;
			if (parent.type === 'MemberExpression' && parentKey === 'property') return;
			if (
				parent.type === 'BinaryExpression' &&
				['===', '!==', '==', '!=', 'in', 'instanceof'].includes(String(parent.operator))
			)
				return;
			if (parent.type === 'SwitchCase' && parentKey === 'test') return;
			if (
				(parent.type === 'CallExpression' || parent.type === 'NewExpression') &&
				parentKey === 'arguments' &&
				SKIP_CALLS.test(calleeName(parent.callee as Node))
			)
				return;
		}
		const chain = n.type === 'BinaryExpression' ? literalChain(n) : null;
		if (chain !== null) {
			if (looksLikeText(chain))
				hits.push({ node: n, text: chain.replace(/\s+/g, ' ').trim(), params: [] });
			return;
		}
		if (n.type === 'Literal' && typeof n.value === 'string') {
			if (looksLikeText(n.value))
				hits.push({ node: n, text: n.value.replace(/\s+/g, ' ').trim(), params: [] });
			return;
		}
		if (n.type === 'TemplateLiteral') {
			if (parent?.type === 'TaggedTemplateExpression') return;
			const { text, params } = templateText(ctx, n);
			if (looksLikeText(text.replace(/\{[^}]*\}/g, ' ')))
				hits.push({ node: n, text: text.replace(/\s+/g, ' ').trim(), params });
			return;
		}
		for (const [k, v] of Object.entries(n)) {
			if (k === 'metadata' || k === 'loc' || k === 'range') continue;
			if (v && typeof v === 'object') visit(v, n, k);
		}
	};
	visit(ast.instance.content, null, '');
	return hits;
}

export function scriptCandidates(file: string, source: string): ScriptCandidate[] {
	const lineOf = (pos: number) => source.slice(0, pos).split('\n').length;
	return scriptStrings(source).map((h) => ({ file, line: lineOf(h.node.start), text: h.text }));
}

// Converts the approved script strings (matched by line + text) in one file.
export function convertScript(
	source: string,
	prefix: string,
	existing: Record<string, string>,
	approved: ScriptCandidate[]
) {
	const lineOf = (pos: number) => source.slice(0, pos).split('\n').length;
	const want = new Set(approved.map((a) => `${a.line}\u0000${a.text}`));
	const ctx: Ctx = {
		source,
		prefix,
		keys: new Map(),
		used: new Set(),
		existing,
		alias: messagesAlias(source)
	};
	const edits: Edit[] = [];
	for (const h of scriptStrings(source)) {
		if (!want.has(`${lineOf(h.node.start)}\u0000${h.text}`)) continue;
		const key = allocKey(ctx, h.text);
		const args = h.params.length
			? `{ ${h.params.map(([nm, sv]) => (nm === sv ? nm : `${nm}: ${sv}`)).join(', ')} }`
			: '';
		edits.push({ start: h.node.start, end: h.node.end, text: `${ctx.alias}.${key}(${args})` });
	}
	edits.sort((a, b) => b.start - a.start);
	let out = source;
	for (const e of edits) out = out.slice(0, e.start) + e.text + out.slice(e.end);
	return {
		out,
		edits: edits.length,
		messages: [...ctx.keys.entries()].map(([text, key]) => ({ key, text }))
	};
}

function ensureImport(out: string, alias: string): string {
	if (/from\s+['"]\$lib\/paraglide\/messages(\.js)?['"]/.test(out)) return out;
	const open = out.match(/<script(?![^>]*context=["']module["'])(?![^>]*\bmodule\b)[^>]*>/);
	if (open && open.index !== undefined) {
		const at = open.index + open[0].length;
		return out.slice(0, at) + `\n\t${importLine(alias)}` + out.slice(at);
	}
	return `<script lang="ts">\n\t${importLine(alias)}\n</script>\n\n` + out;
}

function readJSON(p: string): Record<string, string> {
	return JSON.parse(readFileSync(p, 'utf8'));
}
function writeJSON(p: string, d: Record<string, string>) {
	writeFileSync(p, JSON.stringify(d, null, '\t') + '\n');
}

function main(argv: string[]): number {
	if (argv[0] === '--apply') {
		const tr: Record<string, Record<string, string>> = JSON.parse(readFileSync(argv[1], 'utf8'));
		for (const loc of ['en', 'zh-cn']) {
			const d = readJSON(MSG(loc));
			for (const [k, v] of Object.entries(tr)) {
				if (!(k in d)) throw new Error(`unknown key ${k}`);
				if (v[loc]) d[k] = v[loc];
			}
			writeJSON(MSG(loc), d);
		}
		console.log(`applied ${Object.keys(tr).length} translations`);
		return 0;
	}

	if (argv[0] === '--fix-params') {
		// Paraglide params must be non-null: wrap values svelte-check rejects, e.g.
		// { err } -> { err: String(err) }, { id: x.id } -> { id: x.id ?? '' }.
		const res = Bun.spawnSync(
			['npx', 'svelte-check', '--tsconfig', './tsconfig.json', '--output', 'machine'],
			{ cwd: ROOT }
		);
		const fixes = new Map<string, { line: number; col: number; unknown: boolean }[]>();
		for (const l of res.stdout.toString().split('\n')) {
			const mt = l.match(/^\d+ ERROR "([^"]+)" (\d+):(\d+) "(.*)"$/);
			if (!mt || !mt[4].includes("is not assignable to type '{}'")) continue;
			const file = join(ROOT, mt[1]);
			fixes.set(file, [
				...(fixes.get(file) ?? []),
				{ line: +mt[2], col: +mt[3], unknown: mt[4].startsWith("Type 'unknown'") }
			]);
		}
		let total = 0;
		for (const [file, list] of fixes) {
			const lines = readFileSync(file, 'utf8').split('\n');
			for (const { line, col, unknown } of list.sort((a, b) => b.line - a.line || b.col - a.col)) {
				const L = lines[line - 1];
				const i = col - 1;
				const head = L.slice(i).match(/^(\w+)(\s*:\s*)?/);
				if (!head) continue;
				const name = head[1];
				if (head[2]) {
					const start = i + head[0].length;
					let depth = 0,
						j = start;
					for (; j < L.length; j++) {
						const ch = L[j];
						if ('([{'.includes(ch)) depth++;
						else if (')]}'.includes(ch)) {
							if (depth === 0) break;
							depth--;
						} else if (ch === ',' && depth === 0) break;
					}
					const expr = L.slice(start, j).trim();
					lines[line - 1] =
						L.slice(0, start) + (unknown ? `String(${expr})` : `${expr} ?? ''`) + L.slice(j);
				} else {
					lines[line - 1] =
						L.slice(0, i) +
						(unknown ? `${name}: String(${name})` : `${name}: ${name} ?? ''`) +
						L.slice(i + name.length);
				}
				total++;
			}
			writeFileSync(file, lines.join('\n'));
		}
		console.log(`fixed ${total} message params`);
		return 0;
	}
	if (argv[0] === '--suggest') {
		// Translation memory: fill pending keys whose text already has exactly one
		// translation elsewhere in the message files. Prints { key: { lang: text } }.
		const pending: Pending[] = JSON.parse(readFileSync(argv[1], 'utf8'));
		const pendingKeys = new Set(pending.map((p) => p.key));
		const en = readJSON(MSG('en'));
		const zh = readJSON(MSG('zh-cn'));
		const tm = new Map<string, Set<string>>();
		const add = (from: string, to: string) => {
			if (!from || !to || from === to) return;
			if (!tm.has(from)) tm.set(from, new Set());
			tm.get(from)!.add(to);
		};
		for (const k of Object.keys(en)) {
			if (k === '$schema' || pendingKeys.has(k)) continue;
			add(`en:${en[k]}`, zh[k]);
			add(`zh-cn:${zh[k]}`, en[k]);
		}
		const out: Record<string, Record<string, string>> = {};
		for (const p of pending) {
			const hits = tm.get(`${p.source_lang}:${p.text}`);
			if (hits && hits.size === 1)
				out[p.key] = { [p.source_lang === 'en' ? 'zh-cn' : 'en']: [...hits][0] };
		}
		console.log(JSON.stringify(out, null, '\t'));
		console.error(`suggested ${Object.keys(out).length} of ${pending.length}`);
		return 0;
	}
	if (argv[0] === '--script-candidates') {
		const all = argv.slice(1).flatMap((f) => scriptCandidates(f, readFileSync(f, 'utf8')));
		console.log(JSON.stringify(all, null, '\t'));
		return 0;
	}
	if (argv[0] === '--script-apply') {
		const approved: ScriptCandidate[] = JSON.parse(readFileSync(argv[1], 'utf8'));
		const pendingOut = argv[2];
		if (!pendingOut) {
			console.error('usage: i18n-extract.ts --script-apply approved.json pending.json');
			return 2;
		}
		const en = readJSON(MSG('en'));
		const zh = readJSON(MSG('zh-cn'));
		const pending: Pending[] = existsSync(pendingOut)
			? JSON.parse(readFileSync(pendingOut, 'utf8'))
			: [];
		const byFile = new Map<string, ScriptCandidate[]>();
		for (const a of approved) byFile.set(a.file, [...(byFile.get(a.file) ?? []), a]);
		let missed = 0;
		for (const [file, list] of byFile) {
			const source = readFileSync(file, 'utf8');
			const { out, edits, messages } = convertScript(source, keyPrefix(file), en, list);
			if (edits < list.length) {
				console.error(
					`${file}: converted ${edits} of ${list.length} approved strings (line/text changed?)`
				);
				missed += list.length - edits;
			}
			for (const { key, text } of messages) {
				if (en[key] === text) continue;
				en[key] = text;
				zh[key] = text;
				pending.push({
					key,
					text,
					source_lang: CJK.test(text) && !/[A-Za-z]{3,}/.test(text) ? 'zh-cn' : 'en',
					file
				});
			}
			writeFileSync(file, edits ? ensureImport(out, messagesAlias(source)) : out);
			console.log(`converted ${file}: ${edits} script strings`);
		}
		writeJSON(MSG('en'), en);
		writeJSON(MSG('zh-cn'), zh);
		writeFileSync(pendingOut, JSON.stringify(pending, null, '\t') + '\n');
		return missed ? 1 : 0;
	}

	let pendingPath = '';
	let dry = false;
	const files: string[] = [];
	for (let i = 0; i < argv.length; i++) {
		if (argv[i] === '--pending') pendingPath = argv[++i];
		else if (argv[i] === '--dry-run') dry = true;
		else files.push(argv[i]);
	}
	if (!pendingPath || files.length === 0) {
		console.error('usage: i18n-extract.ts --pending out.json [--dry-run] <file.svelte>...');
		return 2;
	}
	const en = readJSON(MSG('en'));
	const zh = readJSON(MSG('zh-cn'));
	const pending: Pending[] = existsSync(pendingPath)
		? JSON.parse(readFileSync(pendingPath, 'utf8'))
		: [];
	let failed = 0;
	for (const file of files) {
		const source = readFileSync(file, 'utf8');
		const { out, edits, messages } = convert(source, keyPrefix(file), en);
		for (const { key, text } of messages) {
			if (en[key] === text) continue; // same text already under this key
			en[key] = text;
			zh[key] = text;
			pending.push({
				key,
				text,
				source_lang: CJK.test(text) && !/[A-Za-z]{3,}/.test(text) ? 'zh-cn' : 'en',
				file
			});
		}
		console.log(
			`${dry ? 'would convert' : 'converted'} ${file}: ${edits} edits, ${messages.length} messages`
		);
		if (!dry) writeFileSync(file, out);
	}
	if (!dry) {
		writeJSON(MSG('en'), en);
		writeJSON(MSG('zh-cn'), zh);
		writeFileSync(pendingPath, JSON.stringify(pending, null, '\t') + '\n');
	}
	return failed ? 1 : 0;
}

if (import.meta.main) process.exit(main(process.argv.slice(2)));
