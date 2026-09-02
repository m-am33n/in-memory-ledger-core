package ledger

import "fmt"

// HoldStatus is the lifecycle state of an authorization hold.
type HoldStatus string

const (
	// HoldActive means the hold is reserving funds and reduces available balance.
	HoldActive HoldStatus = "ACTIVE"
	// HoldSettled means the authorization completed and the hold no longer
	// reserves funds; the settled money moves into the ledger as a real entry.
	HoldSettled HoldStatus = "SETTLED"
	// HoldReleased means the hold was cancelled without settling (not used by the
	// given event stream, but part of a complete hold lifecycle).
	HoldReleased HoldStatus = "RELEASED"
)

// Hold is a pending authorization. It reserves funds against the available
// balance but is not a ledger entry: no money has moved yet. Money moves only
// when the hold settles, at which point the engine appends a ledger entry and
// marks the hold SETTLED.
//
// Amount is stored as a positive magnitude; a hold always reduces available
// balance, so the sign is implied.
type Hold struct {
	AuthID    string
	Amount    Money // positive magnitude reserved
	PlacedOn  Day
	ValueDate Day
	Status    HoldStatus
}

// Holds is the set of authorization holds for one account. Like the ledger it
// is deliberately dumb: it records and looks up holds and sums the active ones.
// Whether a hold may be placed or settled is the engine's decision, not this
// type's.
type Holds struct {
	currency Currency
	byID     map[string]*Hold
	order    []string // insertion order, for deterministic iteration
}

// NewHolds creates an empty hold set for an account's currency.
func NewHolds(currency Currency) *Holds {
	return &Holds{currency: currency, byID: map[string]*Hold{}}
}

// Place records a new active hold. The amount must be in the account's currency
// and non-negative, and the auth id must be unused; any of these returns an
// error so the engine can record it and carry on rather than crashing.
func (h *Holds) Place(authID string, amount Money, placedOn, valueDate Day) (*Hold, error) {
	if amount.Currency().Code != h.currency.Code {
		return nil, fmt.Errorf("hold currency %s, want %s", amount.Currency().Code, h.currency.Code)
	}
	if amount.IsNegative() {
		return nil, fmt.Errorf("hold amount must be non-negative, got %s", amount)
	}
	if _, exists := h.byID[authID]; exists {
		return nil, fmt.Errorf("duplicate authorization id %q", authID)
	}
	hold := &Hold{
		AuthID:    authID,
		Amount:    amount,
		PlacedOn:  placedOn,
		ValueDate: valueDate,
		Status:    HoldActive,
	}
	h.byID[authID] = hold
	h.order = append(h.order, authID)
	return hold, nil
}

// Get returns the hold with the given auth id, if any.
func (h *Holds) Get(authID string) (*Hold, bool) {
	hold, ok := h.byID[authID]
	return hold, ok
}

// Settle marks an existing active hold as settled and reports success. It does
// not move money — the engine appends the ledger entry. A missing or
// non-active hold returns false so the engine can record the error.
func (h *Holds) Settle(authID string) bool {
	hold, ok := h.byID[authID]
	if !ok || hold.Status != HoldActive {
		return false
	}
	hold.Status = HoldSettled
	return true
}

// Release marks an active hold as released (cancelled without settling).
func (h *Holds) Release(authID string) bool {
	hold, ok := h.byID[authID]
	if !ok || hold.Status != HoldActive {
		return false
	}
	hold.Status = HoldReleased
	return true
}

// All returns every hold in the order it was placed. Copies are returned so
// callers cannot mutate hold state.
func (h *Holds) All() []Hold {
	out := make([]Hold, 0, len(h.order))
	for _, id := range h.order {
		out = append(out, *h.byID[id])
	}
	return out
}

// ActiveTotal sums the amounts of all currently active holds. Available
// balance is ledger balance minus this total.
func (h *Holds) ActiveTotal() Money {
	sum := Zero(h.currency)
	for _, id := range h.order {
		if hold := h.byID[id]; hold.Status == HoldActive {
			sum = sum.Add(hold.Amount)
		}
	}
	return sum
}
