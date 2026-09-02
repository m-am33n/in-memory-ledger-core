# In-Memory Account Ledger Core

A small, append-only account ledger that lives entirely in memory — no database,
no web layer, no UI. It replays a fixed six-day stream of ten events against two
accounts and prints a per-day report: closing balances, overdraft fees,
authorization states, and errors.

The interesting part is not the plumbing but the accounting: **value dates vs
booking dates** (an event can take effect on an earlier day than it arrives),
**append-only corrections** (nothing is ever mutated or deleted — a reversal is
a new opposite entry), and **exact money** (integer minor units, no floats).

## Requirements

Go 1.24+ (module `ledger`). No external dependencies.

## Run the report

```
go run ./cmd/replay
```

This builds the two accounts (ACC-001 in AED, ACC-002 in BHD), replays the
stream, and prints the report to stdout.

## Run the tests

```
go test ./...
```

**Exactly one test fails, on purpose.** `TestRejectedCriterion2_OneFeeOnDay2`
encodes a wrong acceptance criterion ("E7 causes exactly one overdraft fee, on
Day 2") as a live assertion so the refusal is executable, not just prose. Its
failure message explains what it reveals and points to `REJECTED.md`. Every
other test passes. To see just that failure and its explanation:

```
go test -run TestRejectedCriterion2 -v .
```

To confirm everything else is green, run the suite and check that the only
`--- FAIL` line is that one test.

## Reading the output

The report is printed day by day. For each day it shows:

- **events** — each event booked that day and what the engine decided
  (credited/debited, authorization approved/declined, settlement/reversal).
- **closing balance** — the account's balance *as it stood at the close of that
  day* (the "as-closed" balance). Where a later back-valued event restates the
  day, the restated balance is shown alongside, e.g.
  `AED -230.00 (restated to AED 390.00 after later back-valued entries)`.
- **overdraft fees assessed at close** — any AED 25.00 fees charged when that
  day's close is evaluated.

Then four summary sections:

- **Errors & rejected events** — e.g. the orphan settlement E6 (Auth-Z), and the
  declined authorization E8 (Auth-B).
- **Authorization states** — each hold and its final state (Auth-A settled,
  Auth-B declined).
- **Interest capitalized** — the single end-of-window interest credit per
  account, with its per-day accruals.
- **Final balances** — closing balances including the interest credit.

The headline results: **ACC-001 ends at AED 390.92**, **ACC-002 at BHD 10.008**,
with overdraft fees on days 2, 4 and 5. Every figure is derived by hand in
`NUMBERS.md`.

## Design

Four layers, each knowing only the one below it. Decision logic lives at the
top; the lower layers are deliberately dumb and easy to test in isolation.

```
Event stream (E1..E10)            fixed input, replayed in booking-day order
        │
        ▼
   Engine                         the brain: approves/declines authorizations,
   (engine.go)                    matches settlements, applies reversals, runs
        │                         the day-close overdraft and end-of-window
        │                         interest passes; logs one outcome per event
        ├──────────────┬────────────────┐
        ▼              ▼                 ▼
   Ledger         Holds            (assessed-fee set, error log)
   (ledger.go)    (holds.go)       held inside the engine
        │              │
        ▼              ▼
   Money  (money.go)   exact integer minor units + rounding
```

- **`money.go`** — `Money` as signed integer minor units; parse/format, add/sub,
  interest via an exact fraction, half-away-from-zero rounding, and
  `AllocateEqual` (largest-remainder split).
- **`ledger.go`** — the append-only `Ledger`: immutable `Entry` records and
  `BalanceAt(day)` (sum of entries with `value_date ≤ day`).
- **`holds.go`** — authorization `Holds`: place/settle/release and the active
  total that available balance is measured against.
- **`events.go`** — the `Event` model and the fixed `Stream()`.
- **`engine.go`** — the `Engine` that ties it together and the two day-driven
  passes (overdraft, interest).
- **`report.go`** — renders the per-day text report.
- **`cmd/replay/`** — the runnable entry point.

## Documents

- **`NUMBERS.md`** — every constant, why that value, and the full worked
  arithmetic behind the report.
- **`AMBIGUITIES.md`** — every ambiguity in the brief and how it was resolved
  (as-closed vs restated balances, interest basis, retroactive overdraft, …).
- **`REJECTED.md`** — the acceptance criteria refused as incorrect, with
  reasoning, plus approaches abandoned mid-build.
- **`WORKLOG.md`** — timestamped build log.
