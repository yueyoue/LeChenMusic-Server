package scanner

import "testing"

// Regression tests for 评审 §4.14 P2-9: scan result counters feed the "scan complete"
// log line, so an operator can see what a scan actually did.

func TestScanStateSummary(t *testing.T) {
	var state scanState
	if got := state.summary(); got != (scanSummary{}) {
		t.Fatalf("fresh state must summarize to zeros, got %+v", got)
	}

	state.newFiles.Add(3)
	state.updatedFiles.Add(2)
	state.purgedFiles.Add(1)

	if got := state.summary(); got != (scanSummary{New: 3, Updated: 2, Purged: 1}) {
		t.Fatalf("unexpected summary %+v", got)
	}

	// Counters must be independent.
	state.newFiles.Add(1)
	if got := state.summary(); got.New != 4 || got.Updated != 2 || got.Purged != 1 {
		t.Fatalf("counters must accumulate independently, got %+v", got)
	}
}
