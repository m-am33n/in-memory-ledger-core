package ledger

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Report renders the full replay as text: for each day it shows every account's
// events and closing balance, the overdraft fees assessed at that day's close,
// and any errors; then a summary of authorization states, capitalized interest,
// and final balances. The per-day closing balance is the operational balance,
// excluding the end-of-window interest credit (shown separately) so the numbers
// tell a clear story.
func (e *Engine) Report() string {
	var b strings.Builder
	title(&b, "Personal Account Ledger — 6-day replay")

	maxDay := e.lastDay()
	for d := Day(1); d <= maxDay; d++ {
		section(&b, fmt.Sprintf("Day %d", d))
		for _, acc := range e.order {
			e.writeAccountDay(&b, acc, d)
		}
		e.writeFees(&b, d)
	}

	e.writeErrors(&b)
	e.writeAuthStates(&b)
	e.writeInterest(&b)
	e.writeFinalBalances(&b, maxDay)
	return b.String()
}

// writeAccountDay lists an account's events booked on the day, then its
// operational closing balance for the day.
func (e *Engine) writeAccountDay(b *strings.Builder, acc string, d Day) {
	var events []Outcome
	for _, o := range e.log {
		if o.Account == acc && o.Day == d {
			events = append(events, o)
		}
	}
	if len(events) > 0 {
		fmt.Fprintf(b, "  %s events:\n", acc)
		for _, o := range events {
			fmt.Fprintf(b, "    %-3s %-14s %s\n", o.EventID, o.Kind, o.Detail)
		}
	}
	// The per-day closing balance is the as-closed figure: the balance as it
	// actually stood at the end of that day, using only entries booked on or
	// before it. This is what the day's own fee assessment and authorization
	// decisions were computed against, so the numbers on a day are consistent.
	// Where a later back-valued entry (E7, E9) restates the day, the restated
	// balance is shown alongside so the correction is visible, not hidden.
	bal := e.closingBalance(acc, d)
	restated := e.restatedBalance(acc, d)
	if restated.Units() != bal.Units() {
		fmt.Fprintf(b, "  %s closing balance: %s  (restated to %s after later back-valued entries)\n", acc, bal, restated)
	} else {
		fmt.Fprintf(b, "  %s closing balance: %s\n", acc, bal)
	}
}

// writeFees lists the overdraft fees assessed at this day's close (fees booked
// on day d), with the earlier day each fee is value-dated to.
func (e *Engine) writeFees(b *strings.Builder, d Day) {
	var lines []string
	for _, acc := range e.order {
		for _, entry := range e.ledgers[acc].Entries() {
			if entry.Kind == KindOverdraft && entry.BookedOn == d {
				lines = append(lines, fmt.Sprintf("    %s day %d  %s", acc, entry.ValueDate, entry.Amount))
			}
		}
	}
	if len(lines) > 0 {
		fmt.Fprintf(b, "  overdraft fees assessed at close:\n%s\n", strings.Join(lines, "\n"))
	}
}

func (e *Engine) writeErrors(b *strings.Builder) {
	var lines []string
	for _, o := range e.log {
		if !o.OK {
			lines = append(lines, fmt.Sprintf("  %-3s %-14s %s", o.EventID, o.Kind, o.Detail))
		}
	}
	section(b, "Errors & rejected events")
	if len(lines) == 0 {
		b.WriteString("  none\n")
		return
	}
	b.WriteString(strings.Join(lines, "\n") + "\n")
}

func (e *Engine) writeAuthStates(b *strings.Builder) {
	section(b, "Authorization states")
	any := false
	for _, acc := range e.order {
		for _, h := range e.holds[acc].All() {
			any = true
			fmt.Fprintf(b, "  %-7s %-8s (%s, hold %s)\n", h.AuthID, h.Status, acc, h.Amount)
		}
	}
	// Declined authorizations never became holds; surface them from the log.
	for _, o := range e.log {
		if o.Kind == EvAuthorization && !o.OK {
			any = true
			fmt.Fprintf(b, "  %-7s %-8s (%s — never placed)\n", authID(o.Detail), "DECLINED", o.Account)
		}
	}
	if !any {
		b.WriteString("  none\n")
	}
}

func (e *Engine) writeInterest(b *strings.Builder) {
	section(b, "Interest capitalized (end of window)")
	if len(e.interest) == 0 {
		b.WriteString("  none\n")
		return
	}
	for _, r := range e.interest {
		fmt.Fprintf(b, "  %s total %s\n", r.Account, r.Total)
		for d := Day(1); d <= e.lastDay(); d++ {
			if amt, ok := r.Daily[d]; ok {
				fmt.Fprintf(b, "    day %d  %s\n", d, amt)
			}
		}
	}
}

func (e *Engine) writeFinalBalances(b *strings.Builder, maxDay Day) {
	section(b, "Final balances (incl. interest)")
	for _, acc := range e.order {
		fmt.Fprintf(b, "  %s %s\n", acc, e.ledgers[acc].BalanceAt(maxDay))
	}
}

// closingBalance is the as-closed balance for a day: the sum of entries with
// value date on or before the day that were also booked on or before it, so a
// later back-valued entry does not retroactively rewrite the day's own line.
// The interest credit is excluded (reported separately at end of window).
func (e *Engine) closingBalance(acc string, d Day) Money {
	l := e.ledgers[acc]
	sum := Zero(l.currency)
	for _, entry := range l.entries {
		if entry.Kind == KindInterest || entry.ValueDate > d || entry.BookedOn > d {
			continue
		}
		sum = sum.Add(entry.Amount)
	}
	return sum
}

// restatedBalance is the day's closing balance as it stands at the end of the
// window: value date on or before the day, including entries booked later that
// back-value into it (E7, E9). This is the figure interest is computed on.
func (e *Engine) restatedBalance(acc string, d Day) Money {
	l := e.ledgers[acc]
	sum := Zero(l.currency)
	for _, entry := range l.entries {
		if entry.Kind == KindInterest || entry.ValueDate > d {
			continue
		}
		sum = sum.Add(entry.Amount)
	}
	return sum
}

func (e *Engine) lastDay() Day {
	max := Day(0)
	for _, acc := range e.order {
		for _, entry := range e.ledgers[acc].entries {
			if entry.BookedOn > max {
				max = entry.BookedOn
			}
		}
	}
	if max < 1 {
		max = 1
	}
	return max
}

// authID pulls the "Auth-X" token from a decline detail line like
// "Auth-B declined: available ...".
func authID(detail string) string {
	if i := strings.IndexByte(detail, ' '); i > 0 {
		return detail[:i]
	}
	return detail
}

func title(b *strings.Builder, s string) {
	fmt.Fprintf(b, "%s\n%s\n", s, strings.Repeat("=", utf8.RuneCountInString(s)))
}

func section(b *strings.Builder, s string) {
	fmt.Fprintf(b, "\n%s\n%s\n", s, strings.Repeat("-", utf8.RuneCountInString(s)))
}
