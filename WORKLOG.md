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
