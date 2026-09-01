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
