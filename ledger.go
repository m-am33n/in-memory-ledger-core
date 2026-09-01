package ledger

import "fmt"

// Day is a day within the six-day window, numbered 1..6. The brief works in
// whole days ("Day 1".."Day 6"), so a plain integer is all that's needed; no
// calendar dates are involved.
type Day int

// EntryKind labels why an entry exists. It is descriptive only — every kind is
// just a signed money movement in the ledger — but it makes the audit trail and
// the per-day report readable.
type EntryKind string

const (
	KindCredit     EntryKind = "CREDIT"
	KindDebit      EntryKind = "DEBIT"
	KindSettlement EntryKind = "SETTLEMENT"
	KindReversal   EntryKind = "REVERSAL"
	KindOverdraft  EntryKind = "OVERDRAFT_FEE"
	KindInterest   EntryKind = "INTEREST"
)

// Entry is one immutable line in the ledger. Once appended it is never changed
// or removed; corrections are made by appending further entries (e.g. a
// REVERSAL). Amount is signed: credits are positive, debits negative.
//
// BookedOn is the day the entry was recorded (the event's day in the stream);
// ValueDate is the day it takes effect for balance purposes. These usually
// match, but a back-valued event (BookedOn after ValueDate) changes the closing
// balance of an earlier day — the mechanism at the heart of this exercise.
type Entry struct {
	Seq       int    // append order, unique and monotonic
	Account   string // e.g. "ACC-001"
	Kind      EntryKind
	Amount    Money // signed
	BookedOn  Day
	ValueDate Day
	Note      string // human-readable context for the report/audit trail
}

// Ledger is an append-only list of entries for one account. All balances are
// derived by summing entries; nothing is cached or mutated in place.
type Ledger struct {
	account  string
	currency Currency
	entries  []Entry
	nextSeq  int
}

// NewLedger creates an empty ledger for an account in a fixed currency.
func NewLedger(account string, currency Currency) *Ledger {
	return &Ledger{account: account, currency: currency}
}

// Account returns the account identifier.
func (l *Ledger) Account() string { return l.account }

// Currency returns the ledger's currency.
func (l *Ledger) Currency() Currency { return l.currency }

// Append records a new immutable entry and returns it (with Seq assigned). The
// amount must be in the ledger's currency.
func (l *Ledger) Append(kind EntryKind, amount Money, bookedOn, valueDate Day, note string) Entry {
	if amount.Currency().Code != l.currency.Code {
		panic(fmt.Sprintf("account %s is %s but got %s entry",
			l.account, l.currency.Code, amount.Currency().Code))
	}
	e := Entry{
		Seq:       l.nextSeq,
		Account:   l.account,
		Kind:      kind,
		Amount:    amount,
		BookedOn:  bookedOn,
		ValueDate: valueDate,
		Note:      note,
	}
	l.entries = append(l.entries, e)
	l.nextSeq++
	return e
}

// Entries returns a copy of all entries in append order. A copy is returned so
// callers cannot mutate the append-only history.
func (l *Ledger) Entries() []Entry {
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// BalanceAt returns the closing ledger balance for the given day: the sum of
// every entry whose ValueDate is on or before that day. This is the definition
// the overdraft and interest rules operate on, and it is what makes a
// back-valued entry retroactively change an earlier day's balance.
func (l *Ledger) BalanceAt(day Day) Money {
	sum := Zero(l.currency)
	for _, e := range l.entries {
		if e.ValueDate <= day {
			sum = sum.Add(e.Amount)
		}
	}
	return sum
}
