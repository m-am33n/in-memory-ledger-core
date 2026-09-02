# Worklog

Timestamps are real, local time.

## 2026-09-01

- Set up repo: `git init`, `go mod init ledger` (Go 1.24.5).
- Built the `Money` type: exact integer minor units, no floating point.
  - `Currency` carries its own decimal precision (AED 2, BHD 3).
  - `ParseMoney` rejects amounts with more precision than the currency allows
    (silent truncation would corrupt money).
  - Interest rate held as an exact fraction 4/10000; rounding is
    half-away-from-zero via `divRoundHalfAwayFromZero`.
  - Fixed a formatting bug: sign was printing before the currency code
    (`-AED 370.00`); corrected to `AED -370.00` to match the brief.
- Tests cover parse/format, precision rejection, add/sub/neg, currency-mismatch
  panic, and interest rounding. All green; `go vet` clean.

## 2026-09-02

- Added the append-only `Ledger`: immutable `Entry` records, `Append` assigns a
  monotonic `Seq` and never mutates prior entries. `Entries()` returns a copy so
  callers can't rewrite history.
- `Entry` carries both `BookedOn` and `ValueDate`. `BalanceAt(day)` sums entries
  with `ValueDate <= day` — the definition overdraft and interest use, and what
  makes a back-valued entry (E7) retroactively change an earlier day.
- Tests: Day-1 sum, back-valued E7 pushing Day 2 to -370 while leaving Day 1
  untouched, `Entries()` copy immutability, wrong-currency panic. All green.
- Confirmed Day is modelled as a plain int (1..6); no calendar dates.
- Added the holds layer: an authorization hold reserves funds but is *not* a
  ledger entry (no money has moved). Available balance = ledger balance minus
  active holds — the check an AUTHORIZATION is approved against.
- `Holds` is deliberately dumb like the ledger: place, get, settle, release,
  and sum active holds. Whether a hold may be placed or settled is the engine's
  decision, not this type's. Settling only flips the hold's status; the money
  movement will be a ledger entry the engine appends.
- Tests: active hold cuts available (250 - 200 = 50), settle stops reserving
  and can't run twice, settling an unknown auth (E6 Auth-Z) fails, duplicate
  auth id panics. All green; `go vet` clean.
- Refactored boundary checks from panics to returned errors so the engine can
  handle bad data gracefully instead of crashing: `Ledger.Append` and
  `Holds.Place` now return `error` (wrong currency, negative/duplicate hold).
  Kept `Money.Add`/`Sub` panicking on a currency mismatch — that guards against
  our own arithmetic bug (mixing AED and BHD), is unreachable from stream data,
  and returning an error there would clutter every arithmetic call site.
- Built the event model and engine core. `Event` is one struct for the whole
  stream (E1..E10); `Stream()` holds the fixed input. The engine dispatches each
  event, moves money via `Ledger.Append`, reserves funds via `Holds`, and logs
  one `Outcome` per event (approved/declined/rejected + detail).
- `Run` processes day by day, grouping events by booking day rather than slice
  position — E9 is booked day 6 but listed before E10 (day 5). This is also the
  hook point for the later overdraft day-close and interest passes.
- Realised overdraft can't be a post-pass over the final entry set: after E9
  reverses E7, days 2/4/5 are positive again. The fees exist because those days
  were negative *at the close of day 5* (E7 booked, E9 not yet), and append-only
  means E9 can't undo them. Hence the day-by-day structure.
- Two interpretation calls (for AMBIGUITIES.md): the authorization available
  check uses the ledger balance as of the booking day, so E8/Auth-B is declined
  on the already-negative day-5 balance (consistent with "never settled");
  E10 splits 10.000 into 3.333 + 3.333 + 3.334.
- Tests: full-stream final balances, auth approve/decline, settlement match vs
  orphan (E6/Auth-Z), reversal is append-only (both -620 and +620 present),
  instalment split sums exact. All green; `go vet` clean.
- Added the overdraft day-close pass. After each day's events, every account is
  scanned for days 1..d whose closing balance is negative; each such day is
  charged one flat 25.00 fee, value-dated to the negative day, booked on the
  close day. An assessed set makes it once-per-day-per-account and monotonic.
- The scan covers all days up to d (not just d) so a back-valued entry booked
  today (E7) is caught on the earlier day it makes negative. Fees are appended
  in ascending day order, so each fee carries forward into later days' balances.
- Result: fees on days 2, 4, 5 for ACC-001. E9's day-6 reversal lifts those
  days back to positive but does NOT refund the fees — the overdraft happened,
  and corrections are append-only. Final ACC-001 day-6 balance 390.00
  (465 - 75 in fees). ACC-002 is only credited, so never charged.
- Note (REJECTED.md): a criterion expecting fees to disappear after the
  reversal is wrong under append-only accounting; the fees stand.
- Tests: one fee per negative day at the right value dates, fee amount
  AED -25.00, ACC-002 fee-free, updated final balances. All green; vet clean.
- Added the interest capitalization pass (end of window). Confirmed with the
  user: Final view — each day's closing balance is taken from the finished
  ledger, so a day restored to positive by the reversal earns interest and
  overdraft fees (already in the balance) reduce what it earns.
- Accruals are exact fractions (balance * 4 / 10000), never rounded per day in
  isolation. The capitalized credit is the *rounded sum of exact accruals*
  (0.918 -> 0.92), not the sum of per-day rounded accruals (0.93). That single
  total is then split back across the positive days by largest remainder, so
  the rounded per-day figures reconcile to the credit exactly.
- ACC-001: days 1-6 all positive -> 0.10 0.09 0.25 0.17 0.16 0.15 = 0.92, one
  credit value-dated day 6. ACC-002: positive only on days 5-6 (10.000 each)
  -> 0.004 + 0.004 = 0.008. Final balances: ACC-001 390.92, ACC-002 10.008.
- Note (REJECTED.md): naive per-day rounding to 0.93 over-credits by a fil and
  fails "rounded daily accruals must sum exactly to the capitalized total".
- Tests: capitalized total is 0.92 not 0.93, daily allocation values, daily sum
  reconciles to the credit, ACC-002 0.008. All green; `go vet` clean.
- Built the report printer (`Engine.Report()` returns text, so it's testable)
  and the `cmd/replay` command that runs the stream and prints it. Per day it
  shows each account's events + closing balance and the overdraft fees assessed
  at that close; then sections for errors/rejected events, authorization states
  (Auth-A settled, Auth-B declined), capitalized interest, and final balances.
- Per-day closing balance is the operational balance (excludes the day-6
  interest credit, shown separately) and uses the final restated value, so
  day 2 reads 225.00 — the day-2 fee is value-dated back to day 2 even though it
  was booked on day 5. To note in NUMBERS.md.
- Small fixes: rune-count the section underlines (the em-dash is multi-byte, so
  len() over-drew the rule); labelled the failures section "Errors & rejected
  events" since a decline is a valid outcome, not an error like the orphan
  settlement.
- Test asserts the report surfaces the key facts (balances, a fee, auth states,
  the orphan-settlement error, interest total, final balances). Green; vet clean.
- User spotted that the report's day-5 closing balance read +390 (the restated
  value, after E9) when as it actually closed it was negative. Correct catch.
  Changed the per-day line to the as-closed balance (only entries booked on or
  before the day), so day 5 now reads AED -230.00 — consistent with the fee and
  the Auth-B decline printed on the same day — with the restated value shown
  alongside where they differ. Added restatedBalance() for that note.
- Interest keeps the Final (restated) view per the earlier decision; the split
  between point-in-time (fees/auth) and restated (interest) is deliberate.
- Wrote AMBIGUITIES.md documenting all ten interpretation calls: as-closed vs
  restated report balance, interest Final view, retroactive overdraft + no
  refund on reversal, auth available-balance basis, settlement-vs-hold amount,
  orphan settlement, instalment + interest largest-remainder rounding, simple
  (non-compounding) interest, fee currency, opening balances. Wrong acceptance
  criteria are deferred to REJECTED.md (still to write).
- Recovered the exact acceptance-criteria wording from the session transcript
  (the brief was pasted in chat, not saved) so REJECTED.md quotes them verbatim
  rather than paraphrasing.
- Wrote REJECTED.md. Accepted #1, #3, #4, #5; refused #2 (E7 causes three fees
  on days 2/4/5, not one on Day 2), #6 (balances return after E9 but fees do not
  — append-only), #7 (3.334x3 = 10.002 != 10.000; correct split 3.333/3.333/
  3.334), #8 (remainder must be allocated, not discarded — contradicts the
  brief's own reconciliation rule). Also documented the one approach abandoned
  mid-build: overdraft as a single post-pass over the final entry set, which
  would find nothing negative after E9 and charge zero fees.
- Added the required deliberately-failing test (failing_test.go): it encodes the
  refused criterion #2 as an assertion and fails on purpose, printing the three
  real fees (days 2, 4, 5) and the reason. It is the only failing test in the
  suite; its failure message is self-explanatory and points to REJECTED.md.
