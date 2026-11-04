// The contract every value editor follows (OpenSpec wpi-property-editors, D4):
// it edits one value, calls onChange only with values that are valid, and
// onChange(undefined) unsets the value.
import type { Json } from '$lib/wpi/types';
import type { ValidatorClass } from '$lib/wpi/validators';

export type ValueEditorOptions = {
	// value-picker and ordered-multi-picker: the values offered, with their labels.
	choices?: string[];
	choiceLabel?: (value: string) => string;
	// value-picker: the label of the choice that unsets the value; none offered when absent.
	emptyLabel?: () => string;
	// ordered-multi-picker: the fewest values that must stay chosen.
	minItems?: number;
	// length: the keywords and units allowed; a number without a unit means px.
	keywords?: string[];
	units?: string[];
	defaultValue?: string;
	useDefault?: boolean;
	displayType?: boolean;
	valueDisplay?: 'input' | 'label' | 'select';
	allowCustomValue?: boolean;
	helpTip?: string | { en: string; 'zh-cn': string };
	readOnly?: boolean;
	// number
	min?: number;
	max?: number;
	integer?: boolean;
	// color: whether separate light and dark colours are allowed.
	theme?: boolean;
	// The property panel places the color mode selector in its heading.
	colorModeInHeading?: boolean;
	// localized-text: a button that sets the value to the empty string.
	emptyAction?: () => string;
	// Typed text is applied on leaving the field, not on every key.
	commitOnBlur?: boolean;
	// multiline-text and json: initial rows.
	rows?: number;
	// validation: number rules (min, max, step) instead of text rules.
	numberRules?: boolean;
	// field-picker: the table whose fields are offered, as database.schema.table.
	table?: string;
	// validator-list: the validator classes an author may add.
	validatorClasses?: ValidatorClass[];
};

export type ValueEditorProps = {
	id: string;
	label: string;
	value: Json | undefined;
	options: ValueEditorOptions;
	// The property's own rule; a message means the value is not applied.
	check?: (value: Json) => string | null;
	onChange: (value: Json | undefined) => void;
};
