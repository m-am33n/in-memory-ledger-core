// Package ledger is an in-memory, append-only account ledger core.
//
// Money is modelled as an exact integer number of minor units (fils) rather
// than a floating-point value, so every amount is precise and rounded to its
// currency's precision by construction. See NUMBERS.md for the reasoning
// behind the constants defined here.
package ledger

import (
	"fmt"
	"strconv"
	"strings"
)

// Currency describes a currency and how many decimal places it is stored and
// rounded to. AED uses 2 places, BHD uses 3.
type Currency struct {
	Code     string
	Decimals int
}

// The two currencies used by this ledger.
var (
	AED = Currency{Code: "AED", Decimals: 2}
	BHD = Currency{Code: "BHD", Decimals: 3}
)

// scale returns 10^Decimals: the number of minor units in one major unit.
func (c Currency) scale() int64 {
	s := int64(1)
	for i := 0; i < c.Decimals; i++ {
		s *= 10
	}
	return s
}

// Daily interest rate 0.04%, held as an exact fraction (4 / 10000) so interest
// is computed with integer maths and never touches floating point.
const (
	InterestRateNumerator   = 4
	InterestRateDenominator = 10000
)

// Money is an exact amount in a single currency, stored as a signed integer
// count of minor units. All arithmetic is exact; rounding happens only where
// explicitly requested (e.g. interest).
type Money struct {
	currency Currency
	units    int64
}

// NewMoney builds Money directly from a minor-unit count.
func NewMoney(c Currency, units int64) Money { return Money{currency: c, units: units} }

// Zero returns a zero amount in the given currency.
func Zero(c Currency) Money { return Money{currency: c} }

// ParseMoney reads a decimal string such as "1200.00" or "10.000" into Money.
// It accepts fewer fractional digits than the currency's precision but rejects
// more, since that would silently discard data.
func ParseMoney(c Currency, s string) (Money, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Money{}, fmt.Errorf("empty amount")
	}

	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}

	intPart, fracPart := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart, fracPart = s[:dot], s[dot+1:]
	}
	if len(fracPart) > c.Decimals {
		return Money{}, fmt.Errorf("%q has more than %d decimal places for %s", s, c.Decimals, c.Code)
	}

	major, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("invalid amount %q: %w", s, err)
	}
	// Right-pad the fractional part to the currency's precision.
	fracPart += strings.Repeat("0", c.Decimals-len(fracPart))
	var minor int64
	if fracPart != "" {
		if minor, err = strconv.ParseInt(fracPart, 10, 64); err != nil {
			return Money{}, fmt.Errorf("invalid amount %q: %w", s, err)
		}
	}

	units := major*c.scale() + minor
	if neg {
		units = -units
	}
	return Money{currency: c, units: units}, nil
}

// MustParseMoney is ParseMoney that panics on error, for constant test data.
func MustParseMoney(c Currency, s string) Money {
	m, err := ParseMoney(c, s)
	if err != nil {
		panic(err)
	}
	return m
}

// assertSame panics on a currency mismatch: mixing currencies is a programming
// error, not a runtime condition to recover from.
func (m Money) assertSame(o Money) {
	if m.currency.Code != o.currency.Code {
		panic(fmt.Sprintf("currency mismatch: %s vs %s", m.currency.Code, o.currency.Code))
	}
}

// Add returns m + o.
func (m Money) Add(o Money) Money {
	m.assertSame(o)
	return Money{m.currency, m.units + o.units}
}

// Sub returns m - o.
func (m Money) Sub(o Money) Money {
	m.assertSame(o)
	return Money{m.currency, m.units - o.units}
}

// Neg returns -m.
func (m Money) Neg() Money { return Money{m.currency, -m.units} }

// MulFraction returns m * (num/den), rounded to the currency's precision using
// half-away-from-zero rounding. Used for daily interest (num=4, den=10000).
func (m Money) MulFraction(num, den int64) Money {
	return Money{m.currency, divRoundHalfAwayFromZero(m.units*num, den)}
}

// IsNegative reports whether the amount is strictly below zero.
func (m Money) IsNegative() bool { return m.units < 0 }

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m.units == 0 }

// Sign returns -1, 0 or +1.
func (m Money) Sign() int {
	switch {
	case m.units < 0:
		return -1
	case m.units > 0:
		return 1
	default:
		return 0
	}
}

// Currency returns the amount's currency.
func (m Money) Currency() Currency { return m.currency }

// Units returns the raw minor-unit count.
func (m Money) Units() int64 { return m.units }

// String renders the amount with its currency code and full precision,
// e.g. "AED -370.00" or "BHD 3.334".
func (m Money) String() string {
	scale := m.currency.scale()
	u := m.units
	sign := ""
	if u < 0 {
		sign, u = "-", -u
	}
	major, minor := u/scale, u%scale
	if m.currency.Decimals == 0 {
		return fmt.Sprintf("%s %s%d", m.currency.Code, sign, major)
	}
	return fmt.Sprintf("%s %s%d.%0*d", m.currency.Code, sign, major, m.currency.Decimals, minor)
}

// AllocateEqual splits an amount into n parts that sum back to exactly the
// original, distributing the indivisible remainder one minor unit at a time to
// the later parts. Splitting 10.000 BHD three ways gives 3.333, 3.333, 3.334 —
// never 3.334 x 3 = 10.002. n must be >= 1.
func AllocateEqual(m Money, n int) []Money {
	if n < 1 {
		panic("AllocateEqual: n must be >= 1")
	}
	base := m.units / int64(n)
	rem := m.units % int64(n)
	step := int64(1)
	if rem < 0 { // negative amount: hand out negative units
		step, rem = -1, -rem
	}
	parts := make([]Money, n)
	for i := range parts {
		parts[i] = Money{m.currency, base}
	}
	// Give the leftover units to the last |rem| parts, so earlier instalments
	// are the smaller ones.
	for i := 0; i < int(rem); i++ {
		parts[n-1-i].units += step
	}
	return parts
}

// divRoundHalfAwayFromZero divides num by den, rounding halves away from zero
// (so 0.5 -> 1 and -0.5 -> -1). den is assumed positive-or-negative non-zero.
func divRoundHalfAwayFromZero(num, den int64) int64 {
	if den < 0 {
		num, den = -num, -den
	}
	if num >= 0 {
		return (num + den/2) / den
	}
	return -((-num + den/2) / den)
}
