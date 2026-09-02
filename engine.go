package ledger

import "fmt"

// Account configures one account the engine manages.
type Account struct {
	ID       string
	Currency Currency
	Opening  Money // opening balance, booked with value date 1 if non-zero
}

// Outcome records what the engine decided for one event, for the report and
// the audit trail. OK is false when the event was rejected or errored (a
// declined authorization, a settlement with no matching hold, bad data). The
// event is still consumed either way — the stream is append-only.
type Outcome struct {
	EventID string
	Kind    EventKind
	OK      bool
	Detail  string
}

// Engine replays the event stream against the ledgers. It holds the decision
// logic the ledger and holds layers deliberately lack: whether an authorization
// is approved, whether a settlement matches a hold, what a reversal undoes.
//
// Money movements go through Ledger.Append; funds reservations go through
// Holds. Overdraft and interest are separate day-close passes (added later);
// this core just processes the event stream in arrival order.
type Engine struct {
	order          []string // account ids, in configured order (report order)
	ledgers        map[string]*Ledger
	holds          map[string]*Holds
	entriesByEvent map[string][]Entry // event id -> entries it produced (for reversal)
	log            []Outcome
}

// NewEngine builds an engine for the given accounts. A non-zero opening balance
// is recorded as an ordinary credit with value date 1.
func NewEngine(accounts []Account) *Engine {
	e := &Engine{
		ledgers:        map[string]*Ledger{},
		holds:          map[string]*Holds{},
		entriesByEvent: map[string][]Entry{},
	}
	for _, a := range accounts {
		e.order = append(e.order, a.ID)
		l := NewLedger(a.ID, a.Currency)
		e.ledgers[a.ID] = l
		e.holds[a.ID] = NewHolds(a.Currency)
		if !a.Opening.IsZero() {
			l.Append(KindCredit, a.Opening, 1, 1, "opening balance")
		}
	}
	return e
}

// Run replays the stream day by day. Events are grouped by their booking day
// (not their position in the slice, since a back-valued or later-booked event
// can appear out of booking order — E9 is booked on day 6 but listed before
// E10 on day 5). Within a day, events keep their original order.
//
// Processing day by day is what lets overdraft assessment (a later day-close
// pass) see each day's balance as it stood when that day closed, before a
// following day's reversal changes it.
func (e *Engine) Run(events []Event) {
	byDay := map[Day][]Event{}
	maxDay := Day(0)
	for _, ev := range events {
		byDay[ev.BookedOn] = append(byDay[ev.BookedOn], ev)
		if ev.BookedOn > maxDay {
			maxDay = ev.BookedOn
		}
	}
	for d := Day(1); d <= maxDay; d++ {
		for _, ev := range byDay[d] {
			e.process(ev)
		}
		// day-close overdraft assessment goes here (next part)
	}
	// end-of-window interest capitalization goes here (later part)
}

// Ledger returns the ledger for an account, or nil if unknown.
func (e *Engine) Ledger(account string) *Ledger { return e.ledgers[account] }

// Accounts returns the account ids in configured (report) order.
func (e *Engine) Accounts() []string { return append([]string(nil), e.order...) }

// Log returns the per-event outcomes in processing order.
func (e *Engine) Log() []Outcome { return append([]Outcome(nil), e.log...) }

// process dispatches one event by kind.
func (e *Engine) process(ev Event) {
	l := e.ledgers[ev.Account]
	if l == nil {
		e.record(ev, false, fmt.Sprintf("unknown account %q", ev.Account))
		return
	}
	switch ev.Kind {
	case EvCredit:
		e.processCredit(ev, l)
	case EvDebit:
		e.processDebit(ev, l)
	case EvAuthorization:
		e.processAuth(ev, l)
	case EvSettlement:
		e.processSettlement(ev, l)
	case EvReversal:
		e.processReversal(ev, l)
	default:
		e.record(ev, false, fmt.Sprintf("unknown event kind %q", ev.Kind))
	}
}

func (e *Engine) processCredit(ev Event, l *Ledger) {
	parts := []Money{ev.Amount}
	if ev.Instalments > 1 {
		parts = AllocateEqual(ev.Amount, ev.Instalments)
	}
	for i, p := range parts {
		note := ev.Note
		if len(parts) > 1 {
			note = fmt.Sprintf("%s (%d/%d)", ev.Note, i+1, len(parts))
		}
		if entry, err := l.Append(KindCredit, p, ev.BookedOn, ev.ValueDate, note); err != nil {
			e.record(ev, false, err.Error())
			return
		} else {
			e.entriesByEvent[ev.ID] = append(e.entriesByEvent[ev.ID], entry)
		}
	}
	if len(parts) > 1 {
		e.record(ev, true, fmt.Sprintf("credited %s in %d instalments", ev.Amount, len(parts)))
	} else {
		e.record(ev, true, fmt.Sprintf("credited %s", ev.Amount))
	}
}

func (e *Engine) processDebit(ev Event, l *Ledger) {
	entry, err := l.Append(KindDebit, ev.Amount.Neg(), ev.BookedOn, ev.ValueDate, ev.Note)
	if err != nil {
		e.record(ev, false, err.Error())
		return
	}
	e.entriesByEvent[ev.ID] = append(e.entriesByEvent[ev.ID], entry)
	e.record(ev, true, fmt.Sprintf("debited %s", ev.Amount))
}

// processAuth approves an authorization only if available balance stays at or
// above zero once the hold is applied. Available = ledger balance (as of the
// booking day) minus already-active holds.
func (e *Engine) processAuth(ev Event, l *Ledger) {
	h := e.holds[ev.Account]
	available := l.BalanceAt(ev.BookedOn).Sub(h.ActiveTotal())
	afterHold := available.Sub(ev.Amount)
	if afterHold.IsNegative() {
		e.record(ev, false, fmt.Sprintf("%s declined: available %s < hold %s",
			ev.AuthID, available, ev.Amount))
		return
	}
	if _, err := h.Place(ev.AuthID, ev.Amount, ev.BookedOn, ev.ValueDate); err != nil {
		e.record(ev, false, err.Error())
		return
	}
	e.record(ev, true, fmt.Sprintf("%s approved: hold %s, available now %s",
		ev.AuthID, ev.Amount, afterHold))
}

// processSettlement releases the matching hold and books the settled amount as
// a debit. A settlement with no matching authorization (E6, Auth-Z) is
// rejected and books nothing.
func (e *Engine) processSettlement(ev Event, l *Ledger) {
	h := e.holds[ev.Account]
	hold, ok := h.Get(ev.AuthID)
	if !ok {
		e.record(ev, false, fmt.Sprintf("%s settlement rejected: no matching authorization", ev.AuthID))
		return
	}
	if !h.Settle(ev.AuthID) {
		e.record(ev, false, fmt.Sprintf("%s settlement rejected: hold is %s, not active", ev.AuthID, hold.Status))
		return
	}
	entry, err := l.Append(KindSettlement, ev.Amount.Neg(), ev.BookedOn, ev.ValueDate, ev.Note)
	if err != nil {
		e.record(ev, false, err.Error())
		return
	}
	e.entriesByEvent[ev.ID] = append(e.entriesByEvent[ev.ID], entry)
	e.record(ev, true, fmt.Sprintf("%s settled %s (hold of %s released)", ev.AuthID, ev.Amount, hold.Amount))
}

// processReversal undoes the entries a prior event produced by appending an
// opposite entry for each, at the original entry's value date. The original
// entries stay in place — reversal is a new compensating record, not a deletion.
func (e *Engine) processReversal(ev Event, l *Ledger) {
	origs := e.entriesByEvent[ev.Target]
	if len(origs) == 0 {
		e.record(ev, false, fmt.Sprintf("nothing to reverse for %q", ev.Target))
		return
	}
	for _, o := range origs {
		note := fmt.Sprintf("%s (reverses %s seq %d)", ev.Note, ev.Target, o.Seq)
		rev, err := l.Append(KindReversal, o.Amount.Neg(), ev.BookedOn, o.ValueDate, note)
		if err != nil {
			e.record(ev, false, err.Error())
			return
		}
		e.entriesByEvent[ev.ID] = append(e.entriesByEvent[ev.ID], rev)
	}
	e.record(ev, true, fmt.Sprintf("reversed %s (%d entr%s)", ev.Target, len(origs), plural(len(origs))))
}

func (e *Engine) record(ev Event, ok bool, detail string) {
	e.log = append(e.log, Outcome{EventID: ev.ID, Kind: ev.Kind, OK: ok, Detail: detail})
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
