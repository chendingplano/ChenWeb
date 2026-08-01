package semrules

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

const (
	// These limits bound both parsing work and the base-10 powers allocated for
	// exact numeric comparison. Canonical output always remains scientific, so
	// an allowed exponent never expands into a run of zeroes.
	maxJSONNumberDigits           = 1024
	maxJSONNumberAbsoluteExponent = 10000
	maxJSONNumberLiteralBytes     = maxJSONNumberDigits + 32
)

type parsedNumber struct {
	canonical       string
	coefficient     string
	decimalExponent int
}

func isNumericValue(value any) bool {
	switch value.(type) {
	case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func parseNumber(value any) (parsedNumber, error) {
	var raw string
	switch number := value.(type) {
	case json.Number:
		raw = number.String()
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return parsedNumber{}, fmt.Errorf("invalid number %v", number)
		}
		raw = strconv.FormatFloat(number, 'g', -1, 64)
	case float32:
		if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
			return parsedNumber{}, fmt.Errorf("invalid number %v", number)
		}
		raw = strconv.FormatFloat(float64(number), 'g', -1, 32)
	case int:
		raw = strconv.FormatInt(int64(number), 10)
	case int8:
		raw = strconv.FormatInt(int64(number), 10)
	case int16:
		raw = strconv.FormatInt(int64(number), 10)
	case int32:
		raw = strconv.FormatInt(int64(number), 10)
	case int64:
		raw = strconv.FormatInt(number, 10)
	case uint:
		raw = strconv.FormatUint(uint64(number), 10)
	case uint8:
		raw = strconv.FormatUint(uint64(number), 10)
	case uint16:
		raw = strconv.FormatUint(uint64(number), 10)
	case uint32:
		raw = strconv.FormatUint(uint64(number), 10)
	case uint64:
		raw = strconv.FormatUint(number, 10)
	default:
		return parsedNumber{}, fmt.Errorf("value has non-numeric type %T", value)
	}
	parsed, err := parseJSONNumber(raw)
	if err != nil {
		return parsedNumber{}, fmt.Errorf("invalid JSON number %q: %w", raw, err)
	}
	return parsed, nil
}

func parseJSONNumber(raw string) (parsedNumber, error) {
	if raw == "" {
		return parsedNumber{}, fmt.Errorf("empty literal")
	}
	if len(raw) > maxJSONNumberLiteralBytes {
		return parsedNumber{}, fmt.Errorf("literal exceeds %d bytes", maxJSONNumberLiteralBytes)
	}

	index := 0
	negative := false
	if raw[index] == '-' {
		negative = true
		index++
		if index == len(raw) {
			return parsedNumber{}, fmt.Errorf("missing integer")
		}
	}

	integerStart := index
	if raw[index] == '0' {
		index++
		if index < len(raw) && isASCIIDigit(raw[index]) {
			return parsedNumber{}, fmt.Errorf("leading zero is not permitted")
		}
	} else if raw[index] >= '1' && raw[index] <= '9' {
		for index < len(raw) && isASCIIDigit(raw[index]) {
			index++
		}
	} else {
		return parsedNumber{}, fmt.Errorf("integer must start with a digit")
	}
	integerEnd := index

	fractionStart := index
	if index < len(raw) && raw[index] == '.' {
		index++
		fractionStart = index
		for index < len(raw) && isASCIIDigit(raw[index]) {
			index++
		}
		if fractionStart == index {
			return parsedNumber{}, fmt.Errorf("fraction requires at least one digit")
		}
	}
	fractionEnd := index
	fractionDigits := fractionEnd - fractionStart
	if fractionStart == integerEnd {
		fractionDigits = 0
	}

	digitCount := integerEnd - integerStart + fractionDigits
	if digitCount > maxJSONNumberDigits {
		return parsedNumber{}, fmt.Errorf("mantissa has %d digits; maximum is %d", digitCount, maxJSONNumberDigits)
	}

	exponent := 0
	if index < len(raw) && (raw[index] == 'e' || raw[index] == 'E') {
		index++
		exponentNegative := false
		if index < len(raw) && (raw[index] == '+' || raw[index] == '-') {
			exponentNegative = raw[index] == '-'
			index++
		}
		exponentStart := index
		for index < len(raw) && isASCIIDigit(raw[index]) {
			digit := int(raw[index] - '0')
			if exponent > (maxJSONNumberAbsoluteExponent-digit)/10 {
				return parsedNumber{}, fmt.Errorf("absolute exponent exceeds %d", maxJSONNumberAbsoluteExponent)
			}
			exponent = exponent*10 + digit
			index++
		}
		if exponentStart == index {
			return parsedNumber{}, fmt.Errorf("exponent requires at least one digit")
		}
		if exponentNegative {
			exponent = -exponent
		}
	}
	if index != len(raw) {
		return parsedNumber{}, fmt.Errorf("unexpected character %q", raw[index])
	}

	digits := raw[integerStart:integerEnd]
	if fractionDigits > 0 {
		digits += raw[fractionStart:fractionEnd]
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return parsedNumber{canonical: "0", coefficient: "0"}, nil
	}
	trailingZeroes := len(digits) - len(strings.TrimRight(digits, "0"))
	digits = strings.TrimRight(digits, "0")
	decimalExponent := exponent - fractionDigits + trailingZeroes
	scientificExponent := decimalExponent + len(digits) - 1

	var canonical strings.Builder
	if negative {
		canonical.WriteByte('-')
	}
	canonical.WriteByte(digits[0])
	if len(digits) > 1 {
		canonical.WriteByte('.')
		canonical.WriteString(digits[1:])
	}
	if scientificExponent != 0 {
		canonical.WriteByte('e')
		canonical.WriteString(strconv.Itoa(scientificExponent))
	}
	coefficient := digits
	if negative {
		coefficient = "-" + coefficient
	}
	return parsedNumber{canonical: canonical.String(), coefficient: coefficient, decimalExponent: decimalExponent}, nil
}

func (number parsedNumber) rat() *big.Rat {
	coefficient, ok := new(big.Int).SetString(number.coefficient, 10)
	if !ok {
		panic("semrules: validated numeric coefficient did not parse")
	}
	result := new(big.Rat).SetInt(coefficient)
	if number.decimalExponent == 0 || coefficient.Sign() == 0 {
		return result
	}
	powerExponent := number.decimalExponent
	if powerExponent < 0 {
		powerExponent = -powerExponent
	}
	power := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(powerExponent)), nil)
	if number.decimalExponent > 0 {
		return result.Mul(result, new(big.Rat).SetInt(power))
	}
	return result.Quo(result, new(big.Rat).SetInt(power))
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
