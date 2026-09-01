package ledger

import "testing"

func TestParseAndString(t *testing.T) {
	cases := []struct {
		cur  Currency
		in   string
		want string
	}{
		{AED, "1200.00", "AED 1200.00"},
		{AED, "950", "AED 950.00"},   // fewer decimals than precision is fine
		{AED, "-370.00", "AED -370.00"},
		{BHD, "10.000", "BHD 10.000"},
		{BHD, "3.334", "BHD 3.334"},
	}
	for _, c := range cases {
		got := MustParseMoney(c.cur, c.in).String()
		if got != c.want {
			t.Errorf("Parse(%s, %q).String() = %q, want %q", c.cur.Code, c.in, got, c.want)
		}
	}
}

func TestParseRejectsExcessPrecision(t *testing.T) {
	// AED has 2 places; 3 must be rejected rather than silently truncated.
	if _, err := ParseMoney(AED, "1.234"); err == nil {
		t.Fatal("expected error for over-precise AED amount, got nil")
	}
}

func TestAddSubNeg(t *testing.T) {
	a := MustParseMoney(AED, "1200.00")
	b := MustParseMoney(AED, "950.00")
	c := MustParseMoney(AED, "620.00")

	// 1200 - 950 - 620 = -370, the Day-2 closing balance from the brief.
	got := a.Sub(b).Sub(c)
	if got.String() != "AED -370.00" {
		t.Errorf("1200-950-620 = %s, want AED -370.00", got)
	}
	if !got.IsNegative() {
		t.Error("expected -370.00 to be negative")
	}
	if got.Neg().String() != "AED 370.00" {
		t.Errorf("Neg() = %s, want AED 370.00", got.Neg())
	}
}

func TestCurrencyMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on AED + BHD, got none")
		}
	}()
	MustParseMoney(AED, "1.00").Add(MustParseMoney(BHD, "1.000"))
}

func TestInterestRounding(t *testing.T) {
	// 0.04% = 4/10000.
	cases := []struct {
		cur     Currency
		balance string
		want    string
	}{
		{AED, "250.00", "AED 0.10"},   // 250 * 0.0004 = 0.10 exactly
		{AED, "625.00", "AED 0.25"},   // 0.25 exactly
		{AED, "390.00", "AED 0.16"},   // 0.156 -> rounds to 0.16
		{BHD, "10.000", "BHD 0.004"},  // 0.004 exactly
	}
	for _, c := range cases {
		got := MustParseMoney(c.cur, c.balance).
			MulFraction(InterestRateNumerator, InterestRateDenominator)
		if got.String() != c.want {
			t.Errorf("interest on %s = %s, want %s", c.balance, got, c.want)
		}
	}
}

func TestRoundHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		num, den, want int64
	}{
		{5, 10, 1},   // 0.5 -> 1
		{4, 10, 0},   // 0.4 -> 0
		{15, 10, 2},  // 1.5 -> 2
		{-5, 10, -1}, // -0.5 -> -1
		{-4, 10, 0},  // -0.4 -> 0
	}
	for _, c := range cases {
		if got := divRoundHalfAwayFromZero(c.num, c.den); got != c.want {
			t.Errorf("div(%d,%d) = %d, want %d", c.num, c.den, got, c.want)
		}
	}
}
