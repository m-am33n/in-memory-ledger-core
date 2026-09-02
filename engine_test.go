package ledger

import "testing"

// runStream builds the two assessment accounts and replays the fixed stream.
func runStream() *Engine {
	e := NewEngine([]Account{
		{ID: "ACC-001", Currency: AED, Opening: Zero(AED)},
		{ID: "ACC-002", Currency: BHD, Opening: Zero(BHD)},
	})
	e.Run(Stream())
	return e
}

// outcomes indexes the log by event id for easy assertions.
func outcomes(e *Engine) map[string]Outcome {
	m := map[string]Outcome{}
	for _, o := range e.Log() {
		m[o.EventID] = o
	}
	return m
}

// Final ledger balances after the whole stream, excluding overdraft and
// interest (those are separate passes). E7 and its reversal E9 cancel at
// value date 2, so day 2 ends positive even though it was negative on day 5.
func TestStreamFinalBalances(t *testing.T) {
	e := runStream()
	acc1 := e.Ledger("ACC-001")
	want := map[Day]string{
		1: "AED 250.00",
		2: "AED 250.00", // E7 (-620) and E9 (+620) cancel here
		3: "AED 650.00",
		4: "AED 465.00",
		5: "AED 465.00",
		6: "AED 465.00",
	}
	for d := Day(1); d <= 6; d++ {
		if got := acc1.BalanceAt(d).String(); got != want[d] {
			t.Errorf("ACC-001 day %d = %s, want %s", d, got, want[d])
		}
	}

	acc2 := e.Ledger("ACC-002")
	if got := acc2.BalanceAt(6).String(); got != "BHD 10.000" {
		t.Errorf("ACC-002 day 6 = %s, want BHD 10.000", got)
	}
}

func TestAuthApprovedAgainstAvailable(t *testing.T) {
	o := outcomes(runStream())
	// E3: available 250 - 0 holds, hold 200 -> 50 left, approved.
	if !o["E3"].OK {
		t.Errorf("E3 (Auth-A) should be approved, got: %s", o["E3"].Detail)
	}
	// E8: at day 5 the ledger is already negative (E7 back-valued -620),
	// so a 90 hold cannot be covered -> declined.
	if o["E8"].OK {
		t.Errorf("E8 (Auth-B) should be declined on a negative balance, got OK: %s", o["E8"].Detail)
	}
}

func TestSettlementMatchingAndOrphan(t *testing.T) {
	o := outcomes(runStream())
	// E5 settles Auth-A: matches the hold, books the 185 debit.
	if !o["E5"].OK {
		t.Errorf("E5 should settle Auth-A, got: %s", o["E5"].Detail)
	}
	// E6 settles Auth-Z which was never authorized: rejected, books nothing.
	if o["E6"].OK {
		t.Errorf("E6 (Auth-Z) should be rejected, got OK: %s", o["E6"].Detail)
	}
}

// A settled hold stops reserving funds: after E5 releases Auth-A's 200 hold,
// no holds remain active on ACC-001 (Auth-B was declined, so never placed).
func TestReversalIsAppendOnly(t *testing.T) {
	e := runStream()
	o := outcomes(e)
	if !o["E9"].OK {
		t.Errorf("E9 should reverse E7, got: %s", o["E9"].Detail)
	}
	// The original E7 debit is still present; the reversal is a new entry, not
	// a deletion. So the ledger holds both the -620 and a +620.
	var debit, reversal bool
	for _, entry := range e.Ledger("ACC-001").Entries() {
		if entry.Kind == KindDebit && entry.Amount.String() == "AED -620.00" {
			debit = true
		}
		if entry.Kind == KindReversal && entry.Amount.String() == "AED 620.00" {
			reversal = true
		}
	}
	if !debit || !reversal {
		t.Errorf("expected both the -620 debit and its +620 reversal to remain (append-only); debit=%v reversal=%v", debit, reversal)
	}
}

// E10: 10.000 BHD credited as three equal instalments that sum to exactly the
// original — 3.333 + 3.333 + 3.334, never 3.334 x 3.
func TestInstalmentSplitSumsExact(t *testing.T) {
	e := runStream()
	var credits []string
	for _, entry := range e.Ledger("ACC-002").Entries() {
		if entry.Kind == KindCredit {
			credits = append(credits, entry.Amount.String())
		}
	}
	if len(credits) != 3 {
		t.Fatalf("ACC-002 credits = %v, want 3 instalments", credits)
	}
	want := []string{"BHD 3.333", "BHD 3.333", "BHD 3.334"}
	for i := range want {
		if credits[i] != want[i] {
			t.Errorf("instalment %d = %s, want %s", i+1, credits[i], want[i])
		}
	}
}
