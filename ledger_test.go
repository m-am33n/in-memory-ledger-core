package ledger

import "testing"

// aed is a small helper for readable AED amounts in tests.
func aed(s string) Money { return MustParseMoney(AED, s) }

func TestBalanceAtSumsByValueDate(t *testing.T) {
	l := NewLedger("ACC-001", AED)
	l.Append(KindCredit, aed("1200.00"), 1, 1, "E1")
	l.Append(KindDebit, aed("-950.00"), 1, 1, "E2")

	if got := l.BalanceAt(1); got.String() != "AED 250.00" {
		t.Errorf("Day 1 balance = %s, want AED 250.00", got)
	}
}

// A back-valued entry (booked later, value_date earlier) must retroactively
// change the earlier day's closing balance. This is the E7 mechanism.
func TestBackValuedEntryChangesEarlierDay(t *testing.T) {
	l := NewLedger("ACC-001", AED)
	l.Append(KindCredit, aed("1200.00"), 1, 1, "E1")
	l.Append(KindDebit, aed("-950.00"), 1, 1, "E2")
	// E7: booked on Day 5, value_date Day 2.
	l.Append(KindDebit, aed("-620.00"), 5, 2, "E7")

	if got := l.BalanceAt(1); got.String() != "AED 250.00" {
		t.Errorf("Day 1 balance = %s, want AED 250.00 (E7 must not touch Day 1)", got)
	}
	if got := l.BalanceAt(2); got.String() != "AED -370.00" {
		t.Errorf("Day 2 balance = %s, want AED -370.00 (E7 back-values into Day 2)", got)
	}
}

func TestEntriesReturnsCopy(t *testing.T) {
	l := NewLedger("ACC-001", AED)
	l.Append(KindCredit, aed("100.00"), 1, 1, "E1")

	got := l.Entries()
	got[0].Amount = aed("999.00") // mutate the copy
	if l.BalanceAt(1).String() != "AED 100.00" {
		t.Error("mutating Entries() result changed the ledger; history must be immutable")
	}
}

func TestWrongCurrencyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic appending BHD to an AED ledger")
		}
	}()
	NewLedger("ACC-001", AED).Append(KindCredit, MustParseMoney(BHD, "1.000"), 1, 1, "x")
}
