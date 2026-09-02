package ledger

import "testing"

// A placed hold reserves funds: available balance = ledger balance - active
// holds. This is the check an authorization is approved against.
func TestActiveHoldReducesAvailable(t *testing.T) {
	l := NewLedger("ACC-001", AED)
	l.Append(KindCredit, aed("1200.00"), 1, 1, "E1")
	l.Append(KindDebit, aed("-950.00"), 1, 1, "E2") // ledger = 250.00

	h := NewHolds(AED)
	h.Place("Auth-A", aed("200.00"), 2, 2) // hold 200 -> available 50

	available := l.BalanceAt(2).Sub(h.ActiveTotal())
	if available.String() != "AED 50.00" {
		t.Errorf("available = %s, want AED 50.00", available)
	}
	if available.IsNegative() {
		t.Error("available should be non-negative, auth would be approved")
	}
}

// Settling a hold stops it reserving funds. The money movement itself is the
// engine's job (a ledger entry); here we only check the hold is released.
func TestSettleStopsReserving(t *testing.T) {
	h := NewHolds(AED)
	h.Place("Auth-A", aed("200.00"), 2, 2)
	if h.ActiveTotal().String() != "AED 200.00" {
		t.Fatalf("active total = %s, want AED 200.00", h.ActiveTotal())
	}

	if !h.Settle("Auth-A") {
		t.Fatal("Settle(Auth-A) = false, want true")
	}
	if h.ActiveTotal().String() != "AED 0.00" {
		t.Errorf("active total after settle = %s, want AED 0.00", h.ActiveTotal())
	}
	// A hold cannot settle twice.
	if h.Settle("Auth-A") {
		t.Error("second Settle(Auth-A) = true, want false")
	}
}

// E6: a settlement for Auth-Z that was never authorized must fail. Holds
// reports the miss; the engine turns it into a recorded error.
func TestSettleUnknownHoldFails(t *testing.T) {
	h := NewHolds(AED)
	if h.Settle("Auth-Z") {
		t.Error("Settle(Auth-Z) = true for a hold that was never placed, want false")
	}
}

func TestDuplicateAuthErrors(t *testing.T) {
	h := NewHolds(AED)
	if _, err := h.Place("Auth-A", aed("200.00"), 2, 2); err != nil {
		t.Fatalf("first Place = %v, want nil", err)
	}
	if _, err := h.Place("Auth-A", aed("10.00"), 3, 3); err == nil {
		t.Error("second Place with same auth id = nil error, want error")
	}
}
