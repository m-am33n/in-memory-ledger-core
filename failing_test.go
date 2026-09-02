package ledger

import "testing"

// TestRejectedCriterion2_OneFeeOnDay2 is the required deliberately-failing test.
//
// It encodes acceptance criterion #2 — "E7 causes exactly one overdraft fee to
// be assessed, on Day 2." — as an executable assertion. It FAILS on purpose,
// and its failure is the point: it is the refusal from REJECTED.md made runnable.
//
// What it reveals: E7 is a debit value-dated to Day 2, so evaluated at the close
// of Day 5 it drags the closing balance of every day from Day 2 onward negative.
// The once-per-day-per-account overdraft rule therefore fires THREE times — on
// days 2, 4 and 5 — not once, and not only on Day 2. The criterion contradicts
// the brief's own overdraft rule once a back-valued debit is in play.
//
// This is the only failing test in the suite. Everything else is green; see
// README.md. To watch it fail: `go test -run TestRejectedCriterion2`.
func TestRejectedCriterion2_OneFeeOnDay2(t *testing.T) {
	e := runStream()

	var feeDays []Day
	for _, entry := range e.Ledger("ACC-001").Entries() {
		if entry.Kind == KindOverdraft {
			feeDays = append(feeDays, entry.ValueDate)
		}
	}

	// The wrong criterion: exactly one fee, and it must be on Day 2.
	if len(feeDays) != 1 || feeDays[0] != 2 {
		t.Errorf("criterion #2 REFUSED (this failure is intentional):\n"+
			"  criterion claims: exactly ONE overdraft fee, on Day 2\n"+
			"  our engine assesses: %d fees, value-dated to days %v\n"+
			"  reason: E7 (debit, value_date 2) makes days 2, 4 and 5 close\n"+
			"          negative at end of Day 5, so the once-per-day fee fires\n"+
			"          three times. Append-only accounting keeps all three even\n"+
			"          after E9 reverses E7. See REJECTED.md #2.",
			len(feeDays), feeDays)
	}
}
