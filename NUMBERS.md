# Numbers

Every constant this ledger relies on, why it has the value it has , and the full worked arithmetic behind the report so
any figure can be checked by hand.

---

## Constants

### Money is stored as integer minor units — not a float

`Money` holds a signed `int64` count of the currency's smallest unit (fils),
never a `float64`. 0.04% of 250.00, or a three-way split of 10.000, cannot be
represented exactly in binary floating point; storing minor units makes every
amount exact and rounding explicit. `int64` covers ±9.2×10¹⁸ minor units — for
BHD (3 dp) that is billions of dinar, far beyond anything in a six-day window,
so 64 bits is chosen deliberately over 32 (which would overflow at ~21 million
fils, ~£21k — too small to be safe).

### Currency precision — AED 2, BHD 3

```
AED = {Code: "AED", Decimals: 2}   // 1 dirham = 100 fils
BHD = {Code: "BHD", Decimals: 3}   // 1 dinar  = 1000 fils
```

Straight from the question. Precision lives on the currency, so the same code
rounds AED to 2 places and BHD to 3 without special-casing. A parsed amount with
more fractional digits than its currency allows is **rejected**, not truncated —
silently dropping a digit would corrupt money.

### Interest rate — 4 / 10000

```
InterestRateNumerator   = 4
InterestRateDenominator = 10000      // 4/10000 = 0.0004 = 0.04% per day
```

Held as an exact fraction, never as `0.0004` (which has no exact float form).
Interest is `balance × 4 / 10000`, computed in integer arithmetic and rounded
once. Why 4/10000 and not 0.04 or 40/100000: it is the smallest integer pair
that expresses 0.04% exactly, and keeping numerator and denominator separate
lets the multiply happen before the divide, so precision is never lost mid-way.

### Overdraft fee — 25 major units

```
OverdraftFeeMajor = 25               // AED 25.00, or 2500 fils
```

From the brief. Stored as major units and scaled to the account's currency at
use (25.00 AED = 2500 fils; a BHD overdraft would be 25.000 = 25000 fils), so
the fee is never posted in the wrong currency. Not hard-coded to fils, so it
reads as "25" and cannot be mis-scaled.

### Rounding — half away from zero

`divRoundHalfAwayFromZero` rounds a half up in magnitude in both directions:
0.5→1, −0.5→−1. Chosen over truncation (which would always under-credit) and
over banker's rounding (round-half-to-even), because the brief's worked interest
figure — 390.00 × 0.0004 = 0.156 → **0.16** — is a plain half-up-in-magnitude
result. Applied consistently to interest and to nothing else (all other
arithmetic is exact and needs no rounding).

### Days — plain integers 1..6

The window is six numbered days, no calendar. `Day` is an `int`; a "closing
balance for day d" is the sum of entries with `value_date ≤ d`. Nothing about
dates, time zones or weekends enters into it.

---

## Worked arithmetic

### ACC-001 — running balance (as-closed, before interest)

Entries in value-date order, with the day each is booked:

| Entry | Booked | Value date | Amount | Note |
|----|----|----|----|----|
| E1 | 1 | 1 | +1200.00 | credit |
| E2 | 1 | 1 | −950.00 | debit |
| E3 | 2 | 2 | (hold 200.00) | not a ledger entry |
| E4 | 3 | 3 | +400.00 | credit |
| E5 | 4 | 4 | −185.00 | Auth-A settled |
| E6 | 4 | 4 | (rejected) | Auth-Z, no entry |
| E7 | 5 | 2 | −620.00 | back-valued debit |
| fee | 5 | 2 | −25.00 | overdraft, Day 2 |
| fee | 5 | 4 | −25.00 | overdraft, Day 4 |
| fee | 5 | 5 | −25.00 | overdraft, Day 5 |
| E8 | 5 | 5 | (declined) | Auth-B, no entry |
| E9 | 6 | 2 | +620.00 | reverses E7 |
| interest | 6 | 6 | +0.92 | capitalized |

**As-closed closing balance** (only entries booked on or before the day):

```
Day 1:  1200 − 950                                  = 250.00
Day 2:  250                                          = 250.00   (E3 is a hold)
Day 3:  250 + 400                                    = 650.00
Day 4:  650 − 185                                    = 465.00
Day 5:  465 − 620 − 25 − 25 − 25                     = −230.00
Day 6:  −230 + 620 (E9)                              = 390.00
```

**Restated closing balance** (as of end of window, includes later back-values):

```
Day 2:  1200 − 950 − 620 + 620 − 25                  = 225.00
Day 3:  225 + 400                                    = 625.00
Day 4:  625 − 185 − 25                               = 415.00
Day 5:  415 − 25                                     = 390.00
```

The report prints the as-closed figure and appends the restated one where they
differ (see AMBIGUITIES.md §1).

### Overdraft fees — days 2, 4, 5

Evaluated at the close of Day 5 (E7 booked, E9 not yet), each day's closing
balance with fees carried forward as they are booked:

```
Day 2:  1200 − 950 − 620            = −370.00  → negative → fee −25   (criterion #1: −370.00 ✓)
Day 3:  −370 + 400 + (Day-2 fee)−25 =    5.00  → positive → no fee
Day 4:  5 − 185                     = −180.00  → negative → fee −25
Day 5:  −180 − 25 (Day-4 fee)       = −205.00  → negative → fee −25
```

Three fees, total −75.00. E9's reversal on Day 6 does not refund them
(append-only — see REJECTED.md #6).

### ACC-001 interest — capitalizes to 0.92

Interest uses the restated (Final-view) closing balance, positive days only,
× 0.0004, kept as exact fils before rounding:

```
Day 1:  250.00 × 0.0004 = 0.1000  = 10.00 fils
Day 2:  225.00 × 0.0004 = 0.0900  =  9.00 fils
Day 3:  625.00 × 0.0004 = 0.2500  = 25.00 fils
Day 4:  415.00 × 0.0004 = 0.1660  = 16.60 fils
Day 5:  390.00 × 0.0004 = 0.1560  = 15.60 fils
Day 6:  390.00 × 0.0004 = 0.1560  = 15.60 fils
                          exact sum = 91.80 fils = 0.918
```

Capitalized total = round(0.918) = **0.92** (92 fils).

Largest-remainder allocation of 92 fils across the six days — floor each, then
give the 2 leftover fils to the largest fractional remainders (days 4, 5, 6 each
.60; ties broken to the earlier day → days 4 and 5):

```
floors:   10  9  25  16  15  15   = 90 fils
+1 fil to day 4 and day 5:
final:    10  9  25  17  16  15   = 92 fils = 0.92 ✓
report:  0.10 0.09 0.25 0.17 0.16 0.15
```

Naive per-day rounding would give 0.10+0.09+0.25+0.17+0.16+0.16 = 0.93 — one
fil too many (see REJECTED.md #8).

### ACC-002 — BHD instalments and interest

E10 credits 10.000 BHD as three equal instalments. 10000 fils / 3 = 3333 r 1:

```
3.333 + 3.333 + 3.334 = 10.000   (largest remainder gives the extra fil to the last)
```

ACC-002 is positive only on days 5 and 6 (10.000 each):

```
Day 5:  10.000 × 0.0004 = 0.004000 = 4 fils
Day 6:  10.000 × 0.0004 = 0.004000 = 4 fils
                          total     = 8 fils = 0.008
```

Capitalized interest = **0.008 BHD**. No fees (never negative).

### Final balances (incl. interest)

```
ACC-001:  390.00 + 0.92  = AED 390.92
ACC-002:  10.000 + 0.008 = BHD 10.008
```
