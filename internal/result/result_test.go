package result

import "testing"

func TestRewardSummary(t *testing.T) {
	count := uint64(200)
	tests := []struct {
		name   string
		reward Reward
		want   string
	}{
		{"with count", Reward{Name: "Orundum", Count: &count}, "Orundum x200"},
		{"no count", Reward{Name: "LMD", Count: nil}, "LMD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.reward.Summary(); got != tt.want {
				t.Errorf("Summary() = %q; want %q", got, tt.want)
			}
		})
	}
}