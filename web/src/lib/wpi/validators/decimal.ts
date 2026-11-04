// Exact decimals for the validators (OpenSpec wpi-primitive-validators, design
// D5), the twin of server/api/wpi/validators/decimal.go: coef × 10^exp, with coef
// not divisible by 10, or zero with exp 0. A JSON number is read through its
// shortest round-trip form (String(n)); decimal and bigint text keeps every digit.

// Text longer than this, or with an exponent beyond ±MAX_EXPONENT, is not a number.
export const MAX_NUMBER_TEXT = 1000;
const MAX_EXPONENT = 1000;
// The largest integer a JavaScript number holds exactly.
export const MAX_SAFE = 2 ** 53 - 1;

export const decimalSyntax = /^([+-])?(?:([0-9]+)(?:\.([0-9]*))?|\.([0-9]+))(?:[eE]([+-]?[0-9]+))?$/;
export const integerSyntax = /^[+-]?[0-9]+$/;

export type Dec = { coef: bigint; exp: number };

function normalized(coef: bigint, exp: number): Dec {
	if (coef === 0n) return { coef, exp: 0 };
	while (coef % 10n === 0n) {
		coef /= 10n;
		exp++;
	}
	return { coef, exp };
}

// Reads decimal text such as "-12.5", ".5" or "1e3".
export function parseDec(s: string): Dec | null {
	if (s.length > MAX_NUMBER_TEXT) return null;
	const m = decimalSyntax.exec(s);
	if (!m) return null;
	let whole = m[2] ?? '';
	let frac = m[3] ?? '';
	if (m[4] !== undefined) [whole, frac] = ['', m[4]];
	let exp = 0;
	if (m[5] !== undefined) {
		if (m[5].replace(/^[+-]/, '').replace(/^0+/, '').length > 4) return null;
		exp = parseInt(m[5], 10);
		if (exp > MAX_EXPONENT || exp < -MAX_EXPONENT) return null;
	}
	let coef = BigInt(whole + frac);
	if (m[1] === '-') coef = -coef;
	return normalized(coef, exp - frac.length);
}

// A finite number through its shortest round-trip form.
export function fromFloat(f: number): Dec {
	return parseDec(String(f))!;
}

export function jsonNumber(v: unknown): v is number {
	return typeof v === 'number' && Number.isFinite(v);
}

// A constraint value: a JSON number, or decimal or integer text.
export function numberDec(v: unknown): Dec | null {
	if (typeof v === 'string') return parseDec(v);
	return jsonNumber(v) ? fromFloat(v) : null;
}

export function sign(d: Dec): number {
	return d.coef > 0n ? 1 : d.coef < 0n ? -1 : 0;
}

const abs = (n: bigint) => (n < 0n ? -n : n);
const pow10 = (n: number) => 10n ** BigInt(n);

function aligned(a: Dec, b: Dec): [bigint, bigint] {
	const e = Math.min(a.exp, b.exp);
	return [a.coef * pow10(a.exp - e), b.coef * pow10(b.exp - e)];
}

export function cmp(a: Dec, b: Dec): number {
	const [x, y] = aligned(a, b);
	return x < y ? -1 : x > y ? 1 : 0;
}

// Whether d is an integer multiple of m (m > 0).
export function multipleOf(d: Dec, m: Dec): boolean {
	const [x, y] = aligned(d, m);
	return x % y === 0n;
}

export const isInteger = (d: Dec) => d.exp >= 0;

// Digits after the point, trailing zeros excluded.
export const fracDigits = (d: Dec) => Math.max(0, -d.exp);

// Digits before the point, leading zeros excluded.
export function intDigits(d: Dec): number {
	if (d.coef === 0n) return 0;
	return Math.max(0, abs(d.coef).toString().length + d.exp);
}

// The digits of an integer; zero has one.
export const digitCount = (d: Dec) => Math.max(1, intDigits(d));

// Keeps scale digits after the point, by a rounding mode of the decimal validator.
export function round(d: Dec, scale: number, mode: string): Dec {
	if (fracDigits(d) <= scale) return d;
	const p = pow10(-d.exp - scale);
	let q = abs(d.coef) / p;
	const r = abs(d.coef) % p;
	const neg = d.coef < 0n;
	let up = false;
	if (r !== 0n) {
		switch (mode) {
			case 'up':
				up = true;
				break;
			case 'ceiling':
				up = !neg;
				break;
			case 'floor':
				up = neg;
				break;
			case 'half_up':
			case 'half_even': {
				const twice = r * 2n;
				up = twice > p || (twice === p && (mode === 'half_up' || q % 2n === 1n));
			}
		}
	}
	if (up) q += 1n;
	return normalized(neg ? -q : q, -scale);
}

// The plain form: no exponent, no trailing zeros, "0" for zero.
export function decString(d: Dec): string {
	if (d.coef === 0n) return '0';
	const digits = abs(d.coef).toString();
	const n = -d.exp;
	let s: string;
	if (n <= 0) s = digits + '0'.repeat(-n);
	else if (digits.length > n) s = digits.slice(0, digits.length - n) + '.' + digits.slice(digits.length - n);
	else s = '0.' + '0'.repeat(n - digits.length) + digits;
	return d.coef < 0n ? '-' + s : s;
}
