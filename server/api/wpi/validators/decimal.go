package validators

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Numbers are compared, rounded and divided exactly, as decimals (design D5).
// A JSON number is read through its shortest round-trip form, which is the
// float64 the browser parsed; decimal and bigint text keeps every digit.

// Text longer than maxNumberText characters, or with an exponent beyond
// ±maxExponent, is not a number, so aligning exponents stays cheap.
const (
	maxNumberText = 1000
	maxExponent   = 1000
)

// maxSafe is the largest integer a float64 (a JavaScript number) holds exactly.
const maxSafe = 1<<53 - 1

var (
	decimalSyntax = regexp.MustCompile(`^([+-])?(?:([0-9]+)(?:\.([0-9]*))?|\.([0-9]+))(?:[eE]([+-]?[0-9]+))?$`)
	integerSyntax = regexp.MustCompile(`^[+-]?[0-9]+$`)
	ten           = big.NewInt(10)
)

// dec is coef × 10^exp, with coef not divisible by 10, or zero with exp 0.
type dec struct {
	coef *big.Int
	exp  int
}

func normalized(coef *big.Int, exp int) dec {
	if coef.Sign() == 0 {
		return dec{coef, 0}
	}
	q, r := new(big.Int), new(big.Int)
	for {
		q.QuoRem(coef, ten, r)
		if r.Sign() != 0 {
			return dec{coef, exp}
		}
		coef, q = q, coef
		exp++
	}
}

// parseDec reads decimal text such as "-12.5", ".5" or "1e3".
func parseDec(s string) (dec, bool) {
	if len(s) > maxNumberText {
		return dec{}, false
	}
	m := decimalSyntax.FindStringSubmatch(s)
	if m == nil {
		return dec{}, false
	}
	whole, frac := m[2], m[3]
	if m[4] != "" {
		whole, frac = "", m[4]
	}
	exp := 0
	if m[5] != "" {
		if len(strings.TrimLeft(strings.TrimLeft(m[5], "+-"), "0")) > 4 {
			return dec{}, false
		}
		exp, _ = strconv.Atoi(m[5])
		if exp > maxExponent || exp < -maxExponent {
			return dec{}, false
		}
	}
	coef, _ := new(big.Int).SetString(whole+frac, 10)
	if m[1] == "-" {
		coef.Neg(coef)
	}
	return normalized(coef, exp-len(frac)), true
}

// fromFloat reads a finite float64 through its shortest round-trip form.
func fromFloat(f float64) dec {
	d, _ := parseDec(strconv.FormatFloat(f, 'g', -1, 64))
	return d
}

// jsonNumber returns a JSON number's value; decoders give json.Number or float64.
func jsonNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := strconv.ParseFloat(string(n), 64)
		return f, err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
	case float64:
		return n, !math.IsInf(n, 0) && !math.IsNaN(n)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// numberDec reads a constraint value: a JSON number, or decimal or integer text.
func numberDec(v any) (dec, bool) {
	if s, ok := v.(string); ok {
		return parseDec(s)
	}
	f, ok := jsonNumber(v)
	if !ok {
		return dec{}, false
	}
	return fromFloat(f), true
}

func (d dec) sign() int { return d.coef.Sign() }

func pow10(n int) *big.Int { return new(big.Int).Exp(ten, big.NewInt(int64(n)), nil) }

// aligned returns both coefficients scaled to the smaller exponent.
func aligned(a, b dec) (*big.Int, *big.Int) {
	e := min(a.exp, b.exp)
	x := new(big.Int).Mul(a.coef, pow10(a.exp-e))
	y := new(big.Int).Mul(b.coef, pow10(b.exp-e))
	return x, y
}

func (d dec) cmp(o dec) int {
	x, y := aligned(d, o)
	return x.Cmp(y)
}

// multipleOf reports whether d is an integer multiple of m (m > 0).
func (d dec) multipleOf(m dec) bool {
	x, y := aligned(d, m)
	return new(big.Int).Rem(x, y).Sign() == 0
}

func (d dec) isInteger() bool { return d.exp >= 0 }

// fracDigits is the number of digits after the point, trailing zeros excluded.
func (d dec) fracDigits() int { return max(0, -d.exp) }

// intDigits is the number of digits before the point, leading zeros excluded.
func (d dec) intDigits() int {
	if d.coef.Sign() == 0 {
		return 0
	}
	return max(0, len(new(big.Int).Abs(d.coef).String())+d.exp)
}

// digitCount is the number of digits of an integer; zero has one.
func (d dec) digitCount() int { return max(1, d.intDigits()) }

// round keeps scale digits after the point, by a rounding mode of the decimal
// validator.
func (d dec) round(scale int, mode string) dec {
	if d.fracDigits() <= scale {
		return d
	}
	p := pow10(-d.exp - scale)
	q, r := new(big.Int).QuoRem(new(big.Int).Abs(d.coef), p, new(big.Int))
	neg := d.coef.Sign() < 0
	up := false
	if r.Sign() != 0 {
		switch mode {
		case "up":
			up = true
		case "ceiling":
			up = !neg
		case "floor":
			up = neg
		case "half_up", "half_even":
			c := new(big.Int).Lsh(r, 1).Cmp(p)
			up = c > 0 || (c == 0 && (mode == "half_up" || q.Bit(0) == 1))
		}
	}
	if up {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return normalized(q, -scale)
}

// String is the plain form: no exponent, no trailing zeros, "0" for zero.
func (d dec) String() string {
	if d.coef.Sign() == 0 {
		return "0"
	}
	digits := new(big.Int).Abs(d.coef).String()
	var s string
	switch n := -d.exp; {
	case n <= 0:
		s = digits + strings.Repeat("0", -n)
	case len(digits) > n:
		s = digits[:len(digits)-n] + "." + digits[len(digits)-n:]
	default:
		s = "0." + strings.Repeat("0", n-len(digits)) + digits
	}
	if d.coef.Sign() < 0 {
		s = "-" + s
	}
	return s
}

func (d dec) float() float64 {
	f, _ := strconv.ParseFloat(d.String(), 64)
	return f
}
