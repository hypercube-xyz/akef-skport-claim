package model

import (
	"math"
	"testing"
)

func TestRewardSummary(t *testing.T) {
	count := uint64(math.MaxUint64)
	if got := (Reward{Name: "Credits", Count: &count}).Summary(); got != "Credits x18446744073709551615" {
		t.Fatalf("unexpected counted summary: %s", got)
	}
	if got := (Reward{Name: "Unknown"}).Summary(); got != "Unknown" {
		t.Fatalf("unexpected uncounted summary: %s", got)
	}
}
