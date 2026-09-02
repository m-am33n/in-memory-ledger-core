# Rejected Criteria & Abandoned Approaches

The brief states: *"Some of the following criteria are wrong. Identify every
incorrect criterion, refuse it, and document your reasoning."* Below, every
acceptance criterion is quoted verbatim and marked **ACCEPTED** or **REFUSED**,
with the numbers our engine produces. Four are wrong: **#2, #6, #7, #8**.

The criteria are numbered in the order the brief lists them.

---

## #1 — ACCEPTED

> "The Day 2 closing ledger balance, evaluated at end of Day 5 and before any
> fee is assessed, is AED −370.00."

Correct. Evaluated at the end of Day 5 (E7 booked, E9 not yet), the entries with
value date ≤ 2 are E1 (+1200), E2 (−950) and the back-valued E7 (−620):

```
1200.00 − 950.00 − 620.00 = −370.00
```

This is precisely the negative that triggers the Day 2 overdraft fee. Our
report shows this exact figure as Day 5's as-closed line for Day 2's balance.

---

## #2 — REFUSED

> "E7 causes exactly one overdraft fee to be assessed, on Day 2."

Wrong on both counts: it is **not one fee**, and **not only Day 2**.

E7 is value-dated to Day 2, so at the end of Day 5 it drags the closing balance
of **every day from Day 2 onward** negative. Assessing the once-per-day fee over
days 1..5 (fees carry forward as they are booked):

| Day | Closing balance (value_date ≤ day, at Day-5 close) | Negative? | Fee |
|----|----|----|----|
| 2 | −370.00 | yes | −25.00 |
| 4 | −180.00 (incl. Day-2 fee carried in) | yes | −25.00 |
| 5 | −205.00 (incl. Day-2 and Day-4 fees) | yes | −25.00 |

**Three** fees are assessed — days 2, 4 and 5 — not one. (Day 3 closes at
+5.00 after the Day-2 fee, so it escapes.) The criterion's "exactly one, on
Day 2" contradicts the brief's own once-per-day-per-account rule applied to a
back-valued debit.

---

## #3 — ACCEPTED

> "The Day 4 settlement of Auth-A must be accepted."

Correct. E3 placed Auth-A (hold 200.00) and it is still active on Day 4, so E5
settles it: the hold is released and the settled 185.00 is booked as a debit.

---

## #4 — ACCEPTED

> "Any settlement referencing an authorization ID not present in the ledger must
> be rejected and the funds must not leave the account."

Correct, and exactly how E6 (Auth-Z) is handled: no matching authorization
exists, so the settlement is rejected, **no entry is booked**, and it is
recorded as an error. Funds do not move.

---

## #5 — ACCEPTED

> "If Auth-B is approved, its hold reduces available balance but not ledger
> balance."

Correct as a statement of how holds work: a hold is a reservation against
*available* balance (ledger − active holds) and never a ledger entry, so it
never changes the ledger balance. The criterion is conditional ("if approved").

Note the antecedent is false in this run: Auth-B (E8, Day 5) is checked against
the already-negative Day-5 balance (available −155.00) and is **declined**, so
no hold is ever placed. The conditional is still a true statement, so it is
accepted rather than refused — but the "if" never fires.

---

## #6 — REFUSED

> "After E9, all balances and fees return to their pre-E7 values."

Half right, and therefore wrong. **Balances** do return: E9 reverses E7 by
appending a +620.00 compensating entry at value date 2, so from Day 6 the
running balance is back to where it would have been without E7 (Day 6 closes at
390.00 before interest — the pre-E7 figure).

But **fees do not return**. The ledger is append-only: the three overdraft fees
booked at Day-5 close are real, immutable records. E9 does not — and must not —
delete them. The overdraft genuinely occurred given the information the system
had on Day 5; a later reversal corrects the balance going forward, it does not
rewrite history. So the fees stand and the account is 75.00 lighter than its
pre-E7 state. "All balances **and fees** return" is false.

(This is the whole point of the fairness-vs-auditability tension: append-only is
about a truthful record, not about being lenient.)

---

## #7 — REFUSED

> "The three BHD instalments in E10 must each be BHD 3.334."

Wrong — it does not conserve the total:

```
3.334 + 3.334 + 3.334 = 10.002 BHD  ≠  10.000 BHD
```

Rounding each instalment up invents 0.002 BHD from nowhere. The correct split
uses largest-remainder allocation so the parts sum to exactly the original:

```
3.333 + 3.333 + 3.334 = 10.000 BHD
```

Our engine posts 3.333, 3.333, 3.334.

---

## #8 — REFUSED

> "If the rounded daily interest accruals do not sum to the capitalized total,
> the remainder is discarded."

Wrong, and it directly contradicts the brief's own non-negotiable rule:
*"The rounded daily accruals must sum exactly to the capitalized total."*

Discarding the remainder would break that reconciliation. For ACC-001 the exact
daily accruals sum to 0.918, which capitalizes to **0.92**. Rounding each day
independently gives 0.10+0.09+0.25+0.17+0.16+0.16 = **0.93** — a fil too much.
The remainder must be **allocated** (largest-remainder), not discarded and not
left to naive per-day rounding:

```
capitalized total = round(0.918) = 0.92
allocated daily   = 0.10 0.09 0.25 0.17 0.16 0.15  (sums to 0.92 exactly)
```

Our engine capitalizes 0.92 and reconciles the daily figures to it.

---

# Approaches abandoned mid-build

**Overdraft as a single post-pass over the final entry set.** The first plan was
to process all ten events, then sweep each day once at the end and charge a fee
for any day that closes negative. This is wrong here: after E9 reverses E7, days
2, 4 and 5 are **positive again** in the final entry set, so a final-state sweep
would find nothing negative and charge **zero** fees — losing the three fees that
were genuinely incurred at Day-5 close.

The fix was to make the engine run **day by day**, assessing overdraft at each
day's close against the state as it stood then, and guarding fees with a
monotonic assessed set so a later reversal cannot un-charge them. This day-driven
structure is why `Engine.Run` groups events by booking day rather than iterating
the slice in order. (See WORKLOG.md, 2026-09-02.)
