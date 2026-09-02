package ledger

// EventKind is the type of an incoming event in the stream. It is distinct from
// EntryKind: an event is an instruction to the engine, while an entry is the
// immutable record the engine may write in response. One event can produce
// zero, one, or several entries (a rejected authorization writes none; a
// three-instalment credit writes three).
type EventKind string

const (
	EvCredit        EventKind = "CREDIT"
	EvDebit         EventKind = "DEBIT"
	EvAuthorization EventKind = "AUTHORIZATION"
	EvSettlement    EventKind = "SETTLEMENT"
	EvReversal      EventKind = "REVERSAL"
)

// Event is one instruction in the stream. Not every field applies to every
// kind; the engine reads only the ones relevant to Kind.
//
//   - Amount is a positive magnitude; the engine decides the sign (a debit
//     subtracts, a credit adds).
//   - AuthID names the hold for AUTHORIZATION and SETTLEMENT.
//   - Target names the event to undo for REVERSAL (e.g. "E7").
//   - Instalments > 1 splits a CREDIT into that many equal parts (E10).
//   - BookedOn is the day the event arrives; ValueDate is the day it takes
//     effect. They differ for a back-valued event (E7: booked 5, value 2).
type Event struct {
	ID          string
	Kind        EventKind
	Account     string
	Amount      Money
	AuthID      string
	Target      string
	BookedOn    Day
	ValueDate   Day
	Instalments int
	Note        string
}

// Stream returns the fixed six-day event stream from the brief, in arrival
// order. This is the input the replay tool and tests feed to the engine.
func Stream() []Event {
	aed := func(s string) Money { return MustParseMoney(AED, s) }
	bhd := func(s string) Money { return MustParseMoney(BHD, s) }
	return []Event{
		{ID: "E1", Kind: EvCredit, Account: "ACC-001", Amount: aed("1200.00"), BookedOn: 1, ValueDate: 1, Note: "opening credit"},
		{ID: "E2", Kind: EvDebit, Account: "ACC-001", Amount: aed("950.00"), BookedOn: 1, ValueDate: 1, Note: "debit"},
		{ID: "E3", Kind: EvAuthorization, Account: "ACC-001", Amount: aed("200.00"), AuthID: "Auth-A", BookedOn: 2, ValueDate: 2, Note: "authorization"},
		{ID: "E4", Kind: EvCredit, Account: "ACC-001", Amount: aed("400.00"), BookedOn: 3, ValueDate: 3, Note: "credit"},
		{ID: "E5", Kind: EvSettlement, Account: "ACC-001", Amount: aed("185.00"), AuthID: "Auth-A", BookedOn: 4, ValueDate: 4, Note: "settles Auth-A"},
		{ID: "E6", Kind: EvSettlement, Account: "ACC-001", Amount: aed("180.00"), AuthID: "Auth-Z", BookedOn: 4, ValueDate: 4, Note: "settles Auth-Z (never authorized)"},
		{ID: "E7", Kind: EvDebit, Account: "ACC-001", Amount: aed("620.00"), BookedOn: 5, ValueDate: 2, Note: "back-valued debit"},
		{ID: "E8", Kind: EvAuthorization, Account: "ACC-001", Amount: aed("90.00"), AuthID: "Auth-B", BookedOn: 5, ValueDate: 5, Note: "authorization (never settled)"},
		{ID: "E9", Kind: EvReversal, Account: "ACC-001", Target: "E7", BookedOn: 6, ValueDate: 2, Note: "reverses E7"},
		{ID: "E10", Kind: EvCredit, Account: "ACC-002", Amount: bhd("10.000"), Instalments: 3, BookedOn: 5, ValueDate: 5, Note: "credit in three instalments"},
	}
}
