// The text of validator errors, constraints and choices, in English and Chinese
// (OpenSpec wpi-primitive-validators, design D7, D9). An error's text is made
// from its code and params; an instance's messages may replace it.
import { m } from '$lib/paraglide/messages.js';
import { localized } from '../text';
import type { Json } from '../types';
import type { ValidationError } from './run';
import type { ValidatorInstance } from './index';

type Params = Record<string, string>;

// By the part of the code after the dot; type errors are by module.
const typeText: Record<string, () => string> = {
	string: m.wpi_vld_e_type_string,
	integer: m.wpi_vld_e_type_integer,
	number: m.wpi_vld_e_type_number,
	decimal: m.wpi_vld_e_type_decimal,
	boolean: m.wpi_vld_e_type_boolean,
	binary: m.wpi_vld_e_type_binary,
	bigint: m.wpi_vld_e_type_bigint
};

const signText: Record<string, () => string> = {
	positive: m.wpi_vld_e_sign_positive,
	negative: m.wpi_vld_e_sign_negative,
	non_negative: m.wpi_vld_e_sign_non_negative,
	non_positive: m.wpi_vld_e_sign_non_positive,
	non_zero: m.wpi_vld_e_sign_non_zero
};

const errorText: Record<string, (p: Params) => string> = {
	empty: () => m.wpi_vld_e_empty(),
	blank: () => m.wpi_vld_e_blank(),
	min_length: (p) =>
		p.length_unit === 'utf8_bytes'
			? m.wpi_vld_e_min_length_utf8_bytes({ min_length: p.min_length, actual: p.actual })
			: p.length_unit === 'utf16_units'
				? m.wpi_vld_e_min_length_utf16_units({ min_length: p.min_length, actual: p.actual })
				: m.wpi_vld_e_min_length_code_points({ min_length: p.min_length, actual: p.actual }),
	max_length: (p) =>
		p.length_unit === 'utf8_bytes'
			? m.wpi_vld_e_max_length_utf8_bytes({ max_length: p.max_length, actual: p.actual })
			: p.length_unit === 'utf16_units'
				? m.wpi_vld_e_max_length_utf16_units({ max_length: p.max_length, actual: p.actual })
				: m.wpi_vld_e_max_length_code_points({ max_length: p.max_length, actual: p.actual }),
	line_break: () => m.wpi_vld_e_line_break(),
	pattern: () => m.wpi_vld_e_pattern(),
	pattern_limit: (p) => m.wpi_vld_e_pattern_limit({ limit: p.limit }),
	prefix: (p) => m.wpi_vld_e_prefix({ prefix: p.prefix }),
	suffix: (p) => m.wpi_vld_e_suffix({ suffix: p.suffix }),
	contains: (p) => m.wpi_vld_e_contains({ contains: p.contains }),
	unsafe: () => m.wpi_vld_e_unsafe(),
	unsafe_number: () => m.wpi_vld_e_unsafe_number(),
	minimum: (p) => m.wpi_vld_e_minimum({ minimum: p.minimum }),
	maximum: (p) => m.wpi_vld_e_maximum({ maximum: p.maximum }),
	exclusive_minimum: (p) => m.wpi_vld_e_exclusive_minimum({ exclusive_minimum: p.exclusive_minimum }),
	exclusive_maximum: (p) => m.wpi_vld_e_exclusive_maximum({ exclusive_maximum: p.exclusive_maximum }),
	sign: (p) => (signText[p.sign] ?? (() => m.wpi_vld_e_unknown({ code: 'sign' })))(),
	multiple_of: (p) => m.wpi_vld_e_multiple_of({ multiple_of: p.multiple_of }),
	range: (p) => m.wpi_vld_e_range({ lowest: p.lowest, highest: p.highest }),
	integer: () => m.wpi_vld_e_integer(),
	finite: () => m.wpi_vld_e_finite(),
	scale: (p) => m.wpi_vld_e_scale({ scale: p.scale }),
	precision: (p) => m.wpi_vld_e_precision({ precision: p.precision }),
	integer_digits: (p) => m.wpi_vld_e_integer_digits({ integer_digits: p.integer_digits }),
	must_be: (p) => (p.must_be === 'false' ? m.wpi_vld_e_must_be_false() : m.wpi_vld_e_must_be_true()),
	not_allowed: () => m.wpi_vld_e_not_allowed(),
	excluded: () => m.wpi_vld_e_excluded(),
	must_be_null: () => m.wpi_vld_e_must_be_null(),
	must_not_be_null: () => m.wpi_vld_e_must_not_be_null(),
	encoding: (p) => m.wpi_vld_e_encoding({ encoding: p.encoding }),
	padding: (p) => (p.padding === 'forbidden' ? m.wpi_vld_e_padding_forbidden() : m.wpi_vld_e_padding_required()),
	case: (p) => (p.hex_case === 'upper' ? m.wpi_vld_e_case_upper() : m.wpi_vld_e_case_lower()),
	min_bytes: (p) => m.wpi_vld_e_min_bytes({ min_bytes: p.min_bytes, actual: p.actual }),
	max_bytes: (p) => m.wpi_vld_e_max_bytes({ max_bytes: p.max_bytes, actual: p.actual }),
	min_digits: (p) => m.wpi_vld_e_min_digits({ min_digits: p.min_digits, actual: p.actual }),
	max_digits: (p) => m.wpi_vld_e_max_digits({ max_digits: p.max_digits, actual: p.actual }),
	leading_zeros: () => m.wpi_vld_e_leading_zeros()
};

const paramText = (v: Json): string => (typeof v === 'string' ? v : JSON.stringify(v));

// Fills {param} placeholders of an instance's own message.
function fill(text: string, params: Params): string {
	return text.replace(/\{(\w+)\}/g, (all, name: string) => (name in params ? params[name] : all));
}

// The text shown for an error: the instance's own message for its code, else
// the built-in text for the code and params.
export function validationErrorText(error: ValidationError, instance?: ValidatorInstance): string {
	const params: Params = Object.fromEntries(Object.entries(error.params).map(([k, v]) => [k, paramText(v)]));
	const own = instance?.messages?.[error.code];
	if (own !== undefined) {
		const text = localized(own);
		if (text) return fill(text, params);
	}
	const dot = error.code.indexOf('.');
	const module = error.code.slice(0, dot);
	const rule = error.code.slice(dot + 1);
	if (rule === 'type' && typeText[module]) return typeText[module]();
	return errorText[rule]?.(params) ?? m.wpi_vld_e_unknown({ code: error.code });
}

const constraintLabels: Record<string, () => string> = {
	empty: m.wpi_vld_c_empty,
	trim: m.wpi_vld_c_trim,
	normalization: m.wpi_vld_c_normalization,
	length_unit: m.wpi_vld_c_length_unit,
	min_length: m.wpi_vld_c_min_length,
	max_length: m.wpi_vld_c_max_length,
	non_blank: m.wpi_vld_c_non_blank,
	single_line: m.wpi_vld_c_single_line,
	pattern: m.wpi_vld_c_pattern,
	prefix: m.wpi_vld_c_prefix,
	suffix: m.wpi_vld_c_suffix,
	contains: m.wpi_vld_c_contains,
	accept_strings: m.wpi_vld_c_accept_strings,
	accept_numbers: m.wpi_vld_c_accept_numbers,
	minimum: m.wpi_vld_c_minimum,
	maximum: m.wpi_vld_c_maximum,
	exclusive_minimum: m.wpi_vld_c_exclusive_minimum,
	exclusive_maximum: m.wpi_vld_c_exclusive_maximum,
	sign: m.wpi_vld_c_sign,
	multiple_of: m.wpi_vld_c_multiple_of,
	representation: m.wpi_vld_c_representation,
	integer_only: m.wpi_vld_c_integer_only,
	allow_non_finite: m.wpi_vld_c_allow_non_finite,
	precision: m.wpi_vld_c_precision,
	scale: m.wpi_vld_c_scale,
	rounding: m.wpi_vld_c_rounding,
	min_digits: m.wpi_vld_c_min_digits,
	max_digits: m.wpi_vld_c_max_digits,
	allow_leading_zeros: m.wpi_vld_c_allow_leading_zeros,
	must_be: m.wpi_vld_c_must_be,
	allowed_values: m.wpi_vld_c_allowed_values,
	excluded_values: m.wpi_vld_c_excluded_values,
	case_sensitive: m.wpi_vld_c_case_sensitive,
	type_sensitive: m.wpi_vld_c_type_sensitive,
	mode: m.wpi_vld_c_mode,
	absent_is_null: m.wpi_vld_c_absent_is_null,
	empty_string_is_null: m.wpi_vld_c_empty_string_is_null,
	encoding: m.wpi_vld_c_encoding,
	ignore_whitespace: m.wpi_vld_c_ignore_whitespace,
	padding: m.wpi_vld_c_padding,
	hex_case: m.wpi_vld_c_hex_case,
	min_bytes: m.wpi_vld_c_min_bytes,
	max_bytes: m.wpi_vld_c_max_bytes
};

// A constraint's label; an unknown one shows its name.
export function constraintLabel(name: string): string {
	return constraintLabels[name]?.() ?? name;
}

// Choices that read better than their value; the others (int32, base64, nfc) show as they are.
const choiceLabels: Record<string, () => string> = {
	'empty.pass': m.wpi_vld_choice_empty_pass,
	'empty.fail': m.wpi_vld_choice_empty_fail,
	'empty.check': m.wpi_vld_choice_empty_check,
	'trim.none': m.wpi_vld_choice_trim_none,
	'trim.both': m.wpi_vld_choice_trim_both,
	'trim.start': m.wpi_vld_choice_trim_start,
	'trim.end': m.wpi_vld_choice_trim_end,
	'normalization.none': m.wpi_vld_choice_normalization_none,
	'length_unit.code_points': m.wpi_vld_choice_length_unit_code_points,
	'length_unit.utf16_units': m.wpi_vld_choice_length_unit_utf16_units,
	'length_unit.utf8_bytes': m.wpi_vld_choice_length_unit_utf8_bytes,
	'sign.any': m.wpi_vld_choice_sign_any,
	'sign.positive': m.wpi_vld_choice_sign_positive,
	'sign.negative': m.wpi_vld_choice_sign_negative,
	'sign.non_negative': m.wpi_vld_choice_sign_non_negative,
	'sign.non_positive': m.wpi_vld_choice_sign_non_positive,
	'sign.non_zero': m.wpi_vld_choice_sign_non_zero,
	'representation.any': m.wpi_vld_choice_representation_any,
	'representation.unbounded': m.wpi_vld_choice_representation_unbounded,
	'rounding.reject': m.wpi_vld_choice_rounding_reject,
	'rounding.half_up': m.wpi_vld_choice_rounding_half_up,
	'rounding.half_even': m.wpi_vld_choice_rounding_half_even,
	'rounding.down': m.wpi_vld_choice_rounding_down,
	'rounding.up': m.wpi_vld_choice_rounding_up,
	'rounding.floor': m.wpi_vld_choice_rounding_floor,
	'rounding.ceiling': m.wpi_vld_choice_rounding_ceiling,
	'must_be.either': m.wpi_vld_choice_must_be_either,
	'must_be.true': m.wpi_vld_choice_must_be_true,
	'must_be.false': m.wpi_vld_choice_must_be_false,
	'mode.must_be_null': m.wpi_vld_choice_mode_must_be_null,
	'mode.must_not_be_null': m.wpi_vld_choice_mode_must_not_be_null,
	'padding.optional': m.wpi_vld_choice_padding_optional,
	'padding.required': m.wpi_vld_choice_padding_required,
	'padding.forbidden': m.wpi_vld_choice_padding_forbidden,
	'hex_case.any': m.wpi_vld_choice_hex_case_any,
	'hex_case.lower': m.wpi_vld_choice_hex_case_lower,
	'hex_case.upper': m.wpi_vld_choice_hex_case_upper
};

export function choiceLabel(constraint: string, choice: string): string {
	return choiceLabels[`${constraint}.${choice}`]?.() ?? choice;
}

// The palette's sub-folder label of a validator category (requirements §3).
const categoryLabels: Record<string, () => string> = {
	primitive: m.nodepalette_validator_category_primitive,
	text: m.nodepalette_validator_category_text,
	network: m.nodepalette_validator_category_network,
	datetime: m.nodepalette_validator_category_datetime,
	collection: m.nodepalette_validator_category_collection,
	identifier: m.nodepalette_validator_category_identifier,
	file: m.nodepalette_validator_category_file,
	geographic: m.nodepalette_validator_category_geographic,
	numeric: m.nodepalette_validator_category_numeric,
	security: m.nodepalette_validator_category_security,
	relational: m.nodepalette_validator_category_relational
};

export function validatorCategoryLabel(category: string): string {
	return categoryLabels[category]?.() ?? category.toUpperCase();
}

// The texts of an element's validator errors: its own checks when it has run
// them, else the server's from the last save (design D8).
export function elementValidationTexts(own: ValidationError[], server: ValidationError[], list?: ValidatorInstance[]): string[] {
	const shown = own.length ? own : server;
	return [...new Set(shown.map((e) => validationErrorText(e, list?.[e.validator])))];
}
