// The nine primitive validator code modules and their shared constraints
// (OpenSpec wpi-primitive-validators), the twin of server/api/wpi/validators/run.go:
// both give the same errors, in the same order, for every case in
// server/api/wpi/validators/validator-cases.json. undefined is an absent value.
import type { Json, JsonObject } from '../types';
import spec from '../../../../../server/api/wpi/validators/modules.json';
import {
	MAX_NUMBER_TEXT,
	MAX_SAFE,
	cmp,
	decString,
	decimalSyntax,
	digitCount,
	fracDigits,
	fromFloat,
	intDigits,
	integerSyntax,
	jsonNumber,
	multipleOf,
	numberDec,
	parseDec,
	round,
	sign,
	type Dec
} from './decimal';
import { PATTERN_LIMIT, isLineBreak, isSpace, matchesWhole, normalize, textLength, trimSpace } from './text';

export type ConstraintSpec = { name: string; type: string; choices?: string[]; default?: Json };
export type ModuleSpec = {
	category: string;
	value_types: string[];
	empty_values: string[];
	empty_constraint?: string;
	execution: string;
	runs_in: string;
	constraints: ConstraintSpec[];
	error_codes: Record<string, string[]>;
};

// The validator categories of the requirements, §3.1–§3.11, and the code modules.
export const categories: string[] = spec.categories;
export const modules = spec.modules as unknown as Record<string, ModuleSpec>;

// One failure (§2.6.3): a code, the JSON Pointer of the value, the params its
// text is made from, and the validator's index in its element's list.
export type ValidationError = { code: string; path: string; params: Record<string, Json>; validator: number };

// value is the normalized value when the value is valid and has one (§2.6.1).
export type ValidationResult = { valid: boolean; errors: ValidationError[]; value?: Json; hasValue: boolean };

// The module's code defaults overlaid with the given constraints.
export function effectiveConstraints(module: string, given: JsonObject): JsonObject {
	const out: JsonObject = {};
	for (const c of modules[module]?.constraints ?? []) if ('default' in c) out[c.name] = c.default as Json;
	return { ...out, ...given };
}

type Out = { value?: Json; has: boolean };
const none: Out = { has: false };
const some = (value: Json): Out => ({ value, has: true });

class Run {
	errs: ValidationError[] = [];
	constructor(
		readonly module: string,
		readonly c: JsonObject,
		readonly path: string
	) {}
	fail(code: string, params: Record<string, Json> = {}) {
		this.errs.push({ code: `${this.module}.${code}`, path: this.path, params, validator: 0 });
	}
	str(name: string): string {
		const v = this.c[name];
		return typeof v === 'string' ? v : '';
	}
	flag(name: string): boolean {
		return this.c[name] === true;
	}
	count(name: string): number | undefined {
		const v = this.c[name];
		return jsonNumber(v) ? Math.trunc(v) : undefined;
	}
	// The empty constraint (design D6): pass ends the checks, fail reports
	// <module>.empty, check goes on with zero instead.
	empty(v: Json | undefined, zero: Json | undefined): { v: Json | undefined; done: boolean } {
		if (v !== undefined && v !== null && v !== '') return { v, done: false };
		switch (this.str('empty')) {
			case 'fail':
				this.fail('empty');
				return { v: undefined, done: true };
			case 'check':
				return { v: zero, done: false };
		}
		return { v: undefined, done: true };
	}
}

function text(r: Run, value: Json | undefined): Out {
	const e = r.empty(value, '');
	if (e.done) return none;
	if (typeof e.v !== 'string') {
		r.fail('type');
		return none;
	}
	const s = normalize(trimSpace(e.v, r.str('trim')), r.str('normalization'));
	const unit = r.str('length_unit');
	const n = textLength(s, unit);
	const min = r.count('min_length');
	if (min !== undefined && n < min) r.fail('min_length', { min_length: r.c.min_length, length_unit: unit, actual: n });
	const max = r.count('max_length');
	if (max !== undefined && n > max) r.fail('max_length', { max_length: r.c.max_length, length_unit: unit, actual: n });
	if (r.flag('non_blank') && Array.from(s).every(isSpace)) r.fail('blank');
	if (r.flag('single_line') && Array.from(s).some(isLineBreak)) r.fail('line_break');
	const pattern = r.c.pattern;
	if (typeof pattern === 'string') {
		if (Array.from(s).length > PATTERN_LIMIT) r.fail('pattern_limit', { limit: PATTERN_LIMIT });
		else if (!matchesWhole(pattern, s)) r.fail('pattern', { pattern });
	}
	const { prefix, suffix, contains } = r.c;
	if (typeof prefix === 'string' && !s.startsWith(prefix)) r.fail('prefix', { prefix });
	if (typeof suffix === 'string' && !s.endsWith(suffix)) r.fail('suffix', { suffix });
	if (typeof contains === 'string' && !s.includes(contains)) r.fail('contains', { contains });
	return some(s);
}

// A JSON number that must be a safe integer, else <module>.type or unsafeCode.
function wholeNumber(r: Run, v: Json | undefined, unsafeCode: string): Dec | null {
	if (!jsonNumber(v) || !Number.isInteger(v)) {
		r.fail('type');
		return null;
	}
	if (Math.abs(v) > MAX_SAFE) {
		r.fail(unsafeCode);
		return null;
	}
	return fromFloat(v);
}

const safeMax: Dec = { coef: BigInt(MAX_SAFE), exp: 0 };
const safeMin: Dec = { coef: -BigInt(MAX_SAFE), exp: 0 };

function integer(r: Run, v: Json | undefined): Out {
	if (r.empty(v, null).done) return none;
	let d: Dec | null;
	if (typeof v === 'string') {
		const t = trimSpace(v, 'both');
		if (!r.flag('accept_strings') || t.length > MAX_NUMBER_TEXT || !integerSyntax.test(t)) {
			r.fail('type');
			return none;
		}
		d = parseDec(t)!;
		if (cmp(d, safeMax) > 0 || cmp(d, safeMin) < 0) {
			r.fail('unsafe');
			return none;
		}
	} else if (!(d = wholeNumber(r, v, 'unsafe'))) return none;
	numberChecks(r, d, integerRanges);
	return some(Number(decString(d)));
}

function number(r: Run, v: Json | undefined): Out {
	if (r.empty(v, null).done) return none;
	let f: number;
	if (typeof v === 'string') {
		const t = trimSpace(v, 'both');
		if (!r.flag('accept_strings')) {
			r.fail('type');
			return none;
		} else if (t === 'Infinity' || t === '+Infinity') f = Infinity;
		else if (t === '-Infinity') f = -Infinity;
		else if (t === 'NaN') f = NaN;
		else if (t.length > MAX_NUMBER_TEXT || !decimalSyntax.test(t)) {
			r.fail('type');
			return none;
		} else f = Number(t); // ±Infinity on overflow, as Go's ParseFloat
	} else if (typeof v === 'number') f = v;
	else {
		r.fail('type');
		return none;
	}
	if (!Number.isFinite(f)) {
		// Other constraints do not apply to a non-finite value.
		if (!r.flag('allow_non_finite')) r.fail('finite');
		return none;
	}
	if (r.flag('integer_only') && !Number.isInteger(f)) r.fail('integer');
	numberChecks(r, fromFloat(f), {});
	return some(f === 0 ? 0 : f);
}

function decimal(r: Run, v: Json | undefined): Out {
	if (r.empty(v, null).done) return none;
	let d: Dec | null;
	if (typeof v === 'string') {
		if (!(d = parseDec(trimSpace(v, 'both')))) {
			r.fail('type');
			return none;
		}
	} else if (jsonNumber(v) && r.flag('accept_numbers')) d = fromFloat(v);
	else {
		r.fail('type');
		return none;
	}
	const scale = r.count('scale');
	if (scale !== undefined && fracDigits(d) > scale) {
		const mode = r.str('rounding');
		if (mode === 'reject' || mode === '') r.fail('scale', { scale: r.c.scale, actual: fracDigits(d) });
		else d = round(d, scale, mode);
	}
	const precision = r.count('precision');
	if (precision !== undefined) {
		if (scale !== undefined) {
			const allowed = Math.max(0, precision - scale);
			if (intDigits(d) > allowed) r.fail('integer_digits', { integer_digits: allowed, actual: intDigits(d) });
		} else {
			const total = intDigits(d) + fracDigits(d);
			if (total > precision) r.fail('precision', { precision: r.c.precision, actual: total });
		}
	}
	numberChecks(r, d, {});
	return some(decString(d));
}

const asciiLower = (s: string) => s.replace(/[A-Z]/g, (c) => c.toLowerCase());

function boolean(r: Run, value: Json | undefined): Out {
	const e = r.empty(value, false);
	if (e.done) return none;
	const v = e.v;
	let b: boolean;
	if (typeof v === 'boolean') b = v;
	else if (typeof v === 'string') {
		const t = asciiLower(trimSpace(v, 'both'));
		if (!r.flag('accept_strings') || (t !== 'true' && t !== 'false')) {
			r.fail('type');
			return none;
		}
		b = t === 'true';
	} else if (jsonNumber(v) && r.flag('accept_numbers') && (v === 0 || v === 1)) b = v === 1;
	else {
		r.fail('type');
		return none;
	}
	const want = r.str('must_be');
	if ((want === 'true' && !b) || (want === 'false' && b)) r.fail('must_be', { must_be: want });
	return some(b);
}

function enumeration(r: Run, value: Json | undefined): Out {
	// check compares the empty value itself; absent is compared as null.
	if (r.empty(value, null).done) return none;
	const v: Json = value === undefined ? null : value;
	const caseSensitive = r.flag('case_sensitive');
	const typeSensitive = r.flag('type_sensitive');
	const key = enumKey(v, caseSensitive, typeSensitive);
	const has = (list: Json[]) => list.some((x) => enumKey(x, caseSensitive, typeSensitive) === key);
	const allowed = r.c.allowed_values;
	if (Array.isArray(allowed) && !has(allowed)) r.fail('not_allowed', { allowed_values: allowed });
	const excluded = r.c.excluded_values;
	if (Array.isArray(excluded) && has(excluded)) r.fail('excluded', { excluded_values: excluded });
	return some(v);
}

function nullness(r: Run, v: Json | undefined): Out {
	const isNull = v === null || (v === undefined && r.flag('absent_is_null')) || (v === '' && r.flag('empty_string_is_null'));
	if (r.str('mode') === 'must_not_be_null') {
		if (isNull) r.fail('must_not_be_null');
	} else if (!isNull) r.fail('must_be_null');
	if (isNull) return some(null);
	return v === undefined ? none : some(v);
}

const base64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
const base64url = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';

function binary(r: Run, v: Json | undefined): Out {
	if (r.empty(v, null).done) return none;
	if (typeof v !== 'string') {
		r.fail('type');
		return none;
	}
	const s = r.flag('ignore_whitespace') ? Array.from(v).filter((c) => !isSpace(c)).join('') : v;
	const encoding = r.str('encoding');
	let size: number;
	if (encoding === 'hex') {
		if (s.length % 2 !== 0 || !/^[0-9a-fA-F]*$/.test(s)) {
			r.fail('encoding', { encoding });
			return none;
		}
		const want = r.str('hex_case');
		if ((want === 'lower' && /[A-F]/.test(s)) || (want === 'upper' && /[a-f]/.test(s))) r.fail('case', { hex_case: want });
		size = s.length / 2;
	} else {
		const alphabet = encoding === 'base64url' ? base64url : base64;
		const body = s.replace(/=+$/, '');
		const pad = s.length - body.length;
		const rest = body.length % 4;
		if (pad > 2 || rest === 1 || (pad > 0 && (body.length + pad) % 4 !== 0) || Array.from(body).some((c) => !alphabet.includes(c))) {
			r.fail('encoding', { encoding });
			return none;
		}
		const want = r.str('padding');
		if ((want === 'required' && rest !== 0 && pad === 0) || (want === 'forbidden' && pad > 0)) r.fail('padding', { padding: want });
		size = Math.floor((body.length * 3) / 4);
	}
	const min = r.count('min_bytes');
	if (min !== undefined && size < min) r.fail('min_bytes', { min_bytes: r.c.min_bytes, actual: size });
	const max = r.count('max_bytes');
	if (max !== undefined && size > max) r.fail('max_bytes', { max_bytes: r.c.max_bytes, actual: size });
	return some(s);
}

function bigint(r: Run, v: Json | undefined): Out {
	if (r.empty(v, null).done) return none;
	let d: Dec | null;
	let leading = false;
	if (typeof v === 'string') {
		const t = trimSpace(v, 'both');
		if (t.length > MAX_NUMBER_TEXT || !integerSyntax.test(t)) {
			r.fail('type');
			return none;
		}
		const digits = t.replace(/^[+-]/, '');
		leading = digits.length > 1 && digits[0] === '0';
		d = parseDec(t)!;
	} else {
		if (!jsonNumber(v) || !r.flag('accept_numbers')) {
			r.fail('type');
			return none;
		}
		if (!(d = wholeNumber(r, v, 'unsafe_number'))) return none;
	}
	if (leading && !r.flag('allow_leading_zeros')) r.fail('leading_zeros');
	const n = digitCount(d);
	const min = r.count('min_digits');
	if (min !== undefined && n < min) r.fail('min_digits', { min_digits: r.c.min_digits, actual: n });
	const max = r.count('max_digits');
	if (max !== undefined && n > max) r.fail('max_digits', { max_digits: r.c.max_digits, actual: n });
	numberChecks(r, d, bigintRanges);
	return some(decString(d));
}

// The shared numeric constraints: the bounds, sign, multiple_of and, with
// ranges, representation.
function numberChecks(r: Run, d: Dec, ranges: Record<string, [string, string]>) {
	const bounds: [string, (c: number) => boolean][] = [
		['minimum', (c) => c < 0],
		['maximum', (c) => c > 0],
		['exclusive_minimum', (c) => c <= 0],
		['exclusive_maximum', (c) => c >= 0]
	];
	for (const [name, fails] of bounds) {
		const limit = numberDec(r.c[name]);
		if (limit && fails(cmp(d, limit))) r.fail(name, { [name]: r.c[name] });
	}
	const s = sign(d);
	const want = r.str('sign');
	if (
		(want === 'positive' && s <= 0) ||
		(want === 'negative' && s >= 0) ||
		(want === 'non_negative' && s < 0) ||
		(want === 'non_positive' && s > 0) ||
		(want === 'non_zero' && s === 0)
	)
		r.fail('sign', { sign: want });
	const m = numberDec(r.c.multiple_of);
	if (m && sign(m) > 0 && !multipleOf(d, m)) r.fail('multiple_of', { multiple_of: r.c.multiple_of });
	const range = ranges[r.str('representation')];
	if (range) {
		const [lo, hi] = range;
		if (cmp(d, parseDec(lo)!) < 0 || cmp(d, parseDec(hi)!) > 0)
			r.fail('range', { representation: r.str('representation'), lowest: lo, highest: hi });
	}
}

const integerRanges: Record<string, [string, string]> = {
	int8: ['-128', '127'],
	int16: ['-32768', '32767'],
	int32: ['-2147483648', '2147483647'],
	uint8: ['0', '255'],
	uint16: ['0', '65535'],
	uint32: ['0', '4294967295']
};

const bigintRanges: Record<string, [string, string]> = {
	int64: ['-9223372036854775808', '9223372036854775807'],
	uint64: ['0', '18446744073709551615'],
	int128: ['-170141183460469231731687303715884105728', '170141183460469231731687303715884105727'],
	uint128: ['0', '340282366920938463463374607431768211455']
};

// A text two JSON values share exactly when they are equal for enum: numbers by
// value, objects by content, strings length-prefixed. Without type sensitivity
// numbers and booleans compare as their text; without case sensitivity text
// compares lowercased.
function enumKey(v: Json, caseSensitive: boolean, typeSensitive: boolean): string {
	const text = (s: string) => {
		const t = caseSensitive ? s : s.toLowerCase();
		return `s${t.length}:${t}`;
	};
	if (v === null) return 'z';
	if (typeof v === 'boolean') return typeSensitive ? (v ? 'b1' : 'b0') : text(String(v));
	if (typeof v === 'string') return text(v);
	if (typeof v === 'number') return typeSensitive ? 'n' + decString(fromFloat(v)) : text(decString(fromFloat(v)));
	if (Array.isArray(v)) return '[' + v.map((x) => enumKey(x, caseSensitive, typeSensitive)).join(',') + ']';
	return (
		'{' +
		Object.keys(v)
			.sort()
			.map((k) => `s${k.length}:${k}=` + enumKey(v[k], caseSensitive, typeSensitive))
			.join(',') +
		'}'
	);
}

const runners: Record<string, (r: Run, v: Json | undefined) => Out> = {
	string: text,
	integer,
	number,
	decimal,
	boolean,
	enum: enumeration,
	null: nullness,
	binary,
	bigint
};

// Checks value with a validator code module and its constraints, overlaid on
// the module's code defaults; path is the JSON Pointer reported in errors.
export function runValidator(module: string, constraints: JsonObject, value: Json | undefined, path: string): ValidationResult {
	const runner = runners[module];
	if (!runner || !modules[module])
		return { valid: false, hasValue: false, errors: [{ code: 'validator.unknown', path, params: { module }, validator: 0 }] };
	const r = new Run(module, effectiveConstraints(module, constraints), path);
	const out = runner(r, value);
	const valid = r.errs.length === 0;
	return valid && out.has ? { valid, errors: r.errs, value: out.value, hasValue: true } : { valid, errors: r.errs, hasValue: false };
}
