package ledger

import (
	"strings"
	"testing"
)

// The report must surface, in one place, the four things the brief asks for per
// the exercise: closing balances, fee assessments, authorization states, and
// errors. Rather than pin the exact layout, assert the key facts appear.
func TestReportContainsKeyFacts(t *testing.T) {
	r := runStream().Report()

	must := []string{
		"ACC-001 closing balance: AED 250.00", // day 1
		// Day 5 closes negative as it actually stood (before E9), consistent
		// with the fee and decline shown that day; the restated value follows.
		"ACC-001 closing balance: AED -230.00  (restated to AED 390.00 after later back-valued entries)",
		"Auth-A approved",                  // authorization state
		"Auth-Z settlement rejected",       // orphan settlement error (E6)
		"Auth-B declined",                  // declined authorization (E8)
		"overdraft fees assessed at close", // fee section
		"ACC-001 day 2  AED -25.00",        // a specific fee
		"ACC-001 total AED 0.92",           // capitalized interest
		"credited BHD 10.000 in 3 instalments",
		"ACC-001 AED 390.92", // final balance incl. interest
		"ACC-002 BHD 10.008",
	}
	for _, s := range must {
		if !strings.Contains(r, s) {
			t.Errorf("report missing %q\n---\n%s", s, r)
		}
	}
}
