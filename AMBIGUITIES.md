# Ambiguities & Interpretation Decisions

The brief leaves several points open. Each is listed with the interpretation
chosen and why. Where an acceptance criterion is outright *wrong* (not merely
ambiguous), it is refused in `REJECTED.md` instead; this file is for honest
judgement calls where more than one reading is defensible.

Terminology used throughout:

- **Booked-on day** — the day an event arrives in the stream.
- **Value date** — the day an entry takes effect for balance purposes.
- **As-closed balance** — a day's balance as it actually stood at the close of
  that day, using only entries booked on or before it.
- **Restated balance** — a day's balance as of the end of the window, including
  later entries that back-value into it (E7 booked day 5 → value date 2;
  E9 booked day 6 → value date 2).

---

## 1. Which balance the per-day report prints (as-closed vs restated)

**Ambiguity.** The brief says the report prints, per day, the *"closing
balance"* but never says *as of when*. This only matters because E7 and E9
back-value into earlier days, so a day has two legitimate balances: the one it
had when it closed, and the one it has after later corrections.

**Decision.** The per-day line shows the **as-closed** balance, with the
**restated** balance appended where they differ, e.g.:

```
ACC-001 closing balance: AED -230.00  (restated to AED 390.00 after later back-valued entries)
```

**Why.** The report also prints, on the same day, the overdraft fee and the
authorization decisions for that day — and those were computed against the
as-closed balance. Printing the restated balance next to them would contradict
them: day 5 would show a fee and a decline ("available −155") beside a positive
+390. The as-closed figure keeps each day internally consistent; the restated
figure is still shown so the correction is visible, not hidden.

Consequence: ACC-001 reads 250, 250, 650, 465, **−230**, 390 across days 1–6
(as-closed), restating to 250, 225, 625, 415, 390, 390.

---

## 2. Which balance interest accrues on (Final view vs point-in-time)

**Ambiguity.** Interest accrues on each day's *positive closing balance*. With
back-valuing, "closing balance" is again as-of-dependent. On day 5, ACC-001 was
−230 as-closed but +390 after E9 reversed E7.

**Decision.** Interest uses the **Final (restated) view**: each day's closing
balance as of end of window. So day 5 earns interest on +390.

**Why.** E9 reverses E7 — the day-5 debit should never have stood, so the money
genuinely *was* there. Honouring the reversal for balances but ignoring it for
interest would half-apply it. Overdraft fees, by contrast, are point-in-time and
sticky (see §3): the overdraft genuinely occurred even if later reversed.

**The deliberate split.** Fees and authorizations are judged point-in-time;
interest is judged on the restated balance. This is intentional and is the
reason the report shows both balances per day. It is not an inconsistency — the
two rules measure different things (a penalty for what happened vs a return on
what is genuinely held).

---

## 3. Overdraft: retroactive assessment and non-refund on reversal

**Ambiguity.** E7 (booked day 5, value date 2) pushes days 2, 4 and 5 negative
after the fact. The brief doesn't say whether a back-valued debit triggers fees
on the earlier days, nor whether E9's reversal on day 6 refunds them.

**Decision.**
- A back-valued debit **does** trigger the fee on each earlier day it makes
  close negative → fees on days **2, 4 and 5**.
- Each fee is charged **once per account per day** (a monotonic assessed set),
  value-dated to the negative day, booked on the day it is assessed (day 5).
- E9's reversal **does not refund** the fees.

**Why.** Append-only accounting: a fee is a real event, corrected only by a new
compensating entry, never deleted. The overdraft genuinely happened at day-5
close; a later reversal changes the balance going forward but does not erase
history. This is also why "exactly one fee on day 2" and "fees return after the
reversal" are refused in `REJECTED.md`.

---

## 4. Authorization: which balance the available check uses

**Ambiguity.** An authorization is approved if available balance
(ledger − active holds) stays ≥ 0. The brief doesn't state which day's ledger
balance to use, nor whether back-valued entries already booked count.

**Decision.** Available uses the ledger balance **as of the authorization's
booked-on day, including every entry booked by then** (back-valued ones
included). Auth-B (E8, day 5) is checked against the day-5 balance, which
already includes E7's −620 → available −155 → **declined**.

**Why.** This matches how an authorization works in reality: you check funds
against what the account shows *now*, and E7 was already booked when E8 arrived.
It is also consistent with the brief's own note that Auth-B is "never settled" —
a declined authorization places no hold, so there is nothing to settle.

---

## 5. Settlement amount differs from the held amount

**Ambiguity.** Auth-A holds 200.00 but settles for 185.00 (E5). The brief
doesn't spell out what happens to the 15.00 difference.

**Decision.** On settlement the **full hold is released** and the **actual
settled amount (185.00)** is booked as the debit. The 15.00 difference simply
frees back into available balance; it is never booked.

**Why.** A hold is only a reservation; the real money movement is the
settlement. Booking the settled amount and releasing the reservation is standard
card-authorization behaviour.

---

## 6. Orphan settlement (E6, Auth-Z)

**Ambiguity.** E6 settles Auth-Z, for which no authorization exists. Silently
book it, or reject it?

**Decision.** **Rejected**, books nothing, recorded as an error in the report.

**Why.** Settling an authorization that was never made is not a valid money
movement; accepting it would fabricate a debit with no basis. Recording it as a
visible error preserves the audit trail.

---

## 7. Instalment split (E10) and interest reconciliation rounding

**Ambiguity.** E10 credits BHD 10.000 "as three equal instalments"; 10.000 / 3
is not exact. Likewise the daily interest accruals must be rounded but "sum
exactly to the capitalized total".

**Decision.** Both use **largest-remainder allocation**: split to the floor,
then hand the leftover minor units out one at a time (to the later instalments
for E10; to the largest fractional remainders for interest).
- E10 → **3.333 + 3.333 + 3.334** = 10.000 exactly.
- ACC-001 interest → capitalize the **rounded sum of exact accruals**
  (0.918 → **0.92**), then allocate: 0.10, 0.09, 0.25, 0.17, 0.16, 0.15.

**Why.** Naive per-instalment or per-day rounding breaks conservation: 3.334×3 =
10.002, and per-day-rounded interest sums to 0.93 not 0.92 — inventing money
from nowhere. Largest-remainder keeps the parts summing to exactly the whole.
(The naive readings are refused in `REJECTED.md`.)

---

## 8. Interest is simple, not compounded, within the window

**Ambiguity.** The brief says interest is accrued daily and *"capitalized as a
single credit at the end of day 6"*. It doesn't say whether earlier days' accrual
compounds into later days' balances.

**Decision.** **No compounding** within the window. Daily accruals are computed
on the day's balance and summed; a single credit is booked at end of day 6.

**Why.** "A single credit at the end" reads as simple accrual, not daily
capitalization. Compounding would also require defining an intraday ordering of
interest vs other postings that the brief never gives.

---

## 9. Overdraft fee currency and the BHD account

**Ambiguity.** The fee is stated as "AED 25.00". ACC-002 is in BHD.

**Decision.** The fee is charged in the **account's own currency** as a flat 25
major units. In this stream it is moot — ACC-002 is only ever credited and never
closes negative, so it is never charged — but the fee is modelled per-currency
rather than hard-coded to AED, so a negative BHD day would be charged BHD 25.000.

**Why.** A ledger shouldn't post an AED fee into a BHD account (currencies never
mix). Flat-25-in-account-currency is the natural generalisation; noted here
because the "AED 25.00" wording could also be read as AED-only.

---

## 10. Opening balances

Both accounts open at zero (ACC-001 AED 0.00, ACC-002 BHD 0.000), as the brief
states. Modelled as a configurable opening credit (value date 1) for generality,
but zero here means no opening entry is booked.
