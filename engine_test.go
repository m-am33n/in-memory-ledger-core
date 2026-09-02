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

// Final ledger balances after the whole stream: overdraft fees (days 2, 4, 5)
// and the day-6 interest credit are both included. E7 (-620) and reversal E9
// (+620) cancel at value date 2. Day 6 carries the 0.92 interest credit.
func TestStreamFinalBalances(t *testing.T) {
	e := runStream()
	acc1 := e.Ledger("ACC-001")
	want := map[Day]string{
		1: "AED 250.00",
		2: "AED 225.00", // E7/E9 cancel; day-2 fee -25 remains
		3: "AED 625.00",
		4: "AED 415.00", // day-4 fee -25
		5: "AED 390.00", // day-5 fee -25
		6: "AED 390.92", // + 0.92 capitalized interest
	}
	for d := Day(1); d <= 6; d++ {
		if got := acc1.BalanceAt(d).String(); got != want[d] {
			t.Errorf("ACC-001 day %d = %s, want %s", d, got, want[d])
		}
	}

	acc2 := e.Ledger("ACC-002")
	if got := acc2.BalanceAt(6).String(); got != "BHD 10.008" {
		t.Errorf("ACC-002 day 6 = %s, want BHD 10.008 (10.000 + 0.008 interest)", got)
	}
}

// Interest: Final view accrues on every positive closing day. The capitalized
// credit is the rounded sum of exact accruals (0.918 -> 0.92), NOT the sum of
// per-day rounded accruals (which would be 0.93). Largest-remainder allocation
// makes the per-day figures reconcile to the credit exactly.
func TestInterestCapitalizedAndReconciles(t *testing.T) {
	e := runStream()

	var acc1 *InterestResult
	for i := range e.interest {
		if e.interest[i].Account == "ACC-001" {
			acc1 = &e.interest[i]
		}
	}
	if acc1 == nil {
		t.Fatal("no interest result for ACC-001")
	}
	if acc1.Total.String() != "AED 0.92" {
		t.Errorf("ACC-001 capitalized interest = %s, want AED 0.92 (not the naive 0.93)", acc1.Total)
	}

	wantDaily := map[Day]string{
		1: "AED 0.10", 2: "AED 0.09", 3: "AED 0.25",
		4: "AED 0.17", 5: "AED 0.16", 6: "AED 0.15",
	}
	sum := Zero(AED)
	for d := Day(1); d <= 6; d++ {
		got, ok := acc1.Daily[d]
		if !ok {
			t.Errorf("ACC-001 day %d missing from accruals", d)
			continue
		}
		if got.String() != wantDaily[d] {
			t.Errorf("ACC-001 day %d accrual = %s, want %s", d, got, wantDaily[d])
		}
		sum = sum.Add(got)
	}
	if sum.String() != acc1.Total.String() {
		t.Errorf("daily accruals sum to %s but capitalized total is %s; must reconcile", sum, acc1.Total)
	}

	// ACC-002: positive only on days 5 and 6 (10.000 each) -> 0.004 + 0.004.
	var acc2 *InterestResult
	for i := range e.interest {
		if e.interest[i].Account == "ACC-002" {
			acc2 = &e.interest[i]
		}
	}
	if acc2 == nil || acc2.Total.String() != "BHD 0.008" {
		t.Fatalf("ACC-002 interest = %v, want BHD 0.008", acc2)
	}
}

// Overdraft: E7's back-valued debit makes days 2, 4 and 5 close negative, so
// exactly one 25.00 fee is charged on each (value-dated to the negative day).
// E9's reversal on day 6 does not refund them — assessment is append-only.
func TestOverdraftFeesOncePerNegativeDay(t *testing.T) {
	e := runStream()
	fees := map[Day]int{}
	for _, entry := range e.Ledger("ACC-001").Entries() {
		if entry.Kind == KindOverdraft {
			if entry.Amount.String() != "AED -25.00" {
				t.Errorf("fee amount = %s, want AED -25.00", entry.Amount)
			}
			fees[entry.ValueDate]++
		}
	}
	want := map[Day]int{2: 1, 4: 1, 5: 1}
	for d := Day(1); d <= 6; d++ {
		if fees[d] != want[d] {
			t.Errorf("day %d fees = %d, want %d", d, fees[d], want[d])
		}
	}

	// ACC-002 is only ever credited, so it is never charged.
	for _, entry := range e.Ledger("ACC-002").Entries() {
		if entry.Kind == KindOverdraft {
			t.Errorf("ACC-002 should have no overdraft fees, found one on day %d", entry.ValueDate)
		}
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
