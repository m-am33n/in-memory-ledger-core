// Command replay builds the two assessment accounts, replays the fixed event
// stream through the ledger engine, and prints the per-day report.
//
//	go run ./cmd/replay
package main

import (
	"fmt"

	"ledger"
)

func main() {
	engine := ledger.NewEngine([]ledger.Account{
		{ID: "ACC-001", Currency: ledger.AED, Opening: ledger.Zero(ledger.AED)},
		{ID: "ACC-002", Currency: ledger.BHD, Opening: ledger.Zero(ledger.BHD)},
	})
	engine.Run(ledger.Stream())
	fmt.Print(engine.Report())
}
