package skport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return json.RawMessage(data)
}

func TestIsAuthCode(t *testing.T) {
	codes := []int64{401, 403, 10002, 10003, 10004, 20001, 20002}
	for _, code := range codes {
		if !IsAuthCode(code) {
			t.Errorf("IsAuthCode(%d) = false; want true", code)
		}
	}
	if IsAuthCode(0) || IsAuthCode(500) {
		t.Error("IsAuthCode(0/500) = true; want false")
	}
}

func TestDirectBool(t *testing.T) {
	tests := []struct {
		name     string
		object   map[string]any
		keys     map[string]struct{}
		wantVal  bool
		wantKnow bool
	}{
		{"known true", map[string]any{"available": true}, availableKeys, true, true},
		{"known false", map[string]any{"available": false}, availableKeys, false, true},
		{"missing key", map[string]any{"other": true}, availableKeys, false, false},
		{"non-bool ignored", map[string]any{"available": "yes"}, availableKeys, false, false},
		{"done key", map[string]any{"isDone": true}, doneKeys, true, true},
		{"hasToday key", map[string]any{"hasToday": true}, hasTodayKeys, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotKnow := directBool(tt.object, tt.keys)
			if gotVal != tt.wantVal || gotKnow != tt.wantKnow {
				t.Errorf("directBool() = (%v, %v); want (%v, %v)", gotVal, gotKnow, tt.wantVal, tt.wantKnow)
			}
		})
	}
}

func TestClaimResponse_Classify(t *testing.T) {
	tests := []struct {
		name string
		code int64
		want ClaimClass
	}{
		{"success", 0, ClaimSuccess},
		{"already done", 10001, ClaimAlreadyDone},
		{"auth 401", 401, ClaimAuthError},
		{"auth 10002", 10002, ClaimAuthError},
		{"api error", 99999, ClaimAPIError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ClaimResponse{Code: tt.code}
			if got := r.Classify(); got != tt.want {
				t.Errorf("Classify() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestAttendanceState(t *testing.T) {
	tests := []struct {
		fixture string
		check   func(AttendanceState)
	}{
		{"attendance_available.json", func(s AttendanceState) {
			if !s.Available || s.Done {
				t.Errorf("State() = %+v; want Available=true, Done=false", s)
			}
			if !s.SessionValid {
				t.Error("SessionValid = false; want true")
			}
		}},
		{"attendance_done.json", func(s AttendanceState) {
			if s.Available || !s.Done {
				t.Errorf("State() = %+v; want Done=true", s)
			}
		}},
		{"attendance_conflict.json", func(s AttendanceState) {
			if !s.Conflict || !s.Done {
				t.Errorf("State() = %+v; want Conflict=true, Done=true (fails closed)", s)
			}
		}},
		{"attendance_nested.json", func(s AttendanceState) {
			if !s.Available || s.Done {
				t.Errorf("State() = %+v; want Available=true", s)
			}
		}},
		{"attendance_hastoday.json", func(s AttendanceState) {
			if !s.HasToday || !s.HasTodayKnown {
				t.Errorf("State() = %+v; want HasToday=true", s)
			}
		}},
		{"attendance_hastoday_false.json", func(s AttendanceState) {
			if s.SessionValid {
				t.Error("SessionValid = true; want false (hasToday=false)")
			}
		}},
		{"attendance_null.json", func(s AttendanceState) {
			if s.Available || s.Done || s.AvailableKnown || s.DoneKnown {
				t.Errorf("State() = %+v; want empty state", s)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			resp := AttendanceResponse{Code: 0, Data: readFixture(t, tt.fixture)}
			tt.check(resp.State())
		})
	}
}

func TestAvailableRewards(t *testing.T) {
	resp := AttendanceResponse{Code: 0, Data: readFixture(t, "attendance_rewards.json")}
	rewards := resp.AvailableRewards()
	if len(rewards) != 1 || rewards[0].ID != "reward1" || rewards[0].Name != "Orundum" || *rewards[0].Count != 200 {
		t.Errorf("AvailableRewards() = %+v; want [Orundum x200]", rewards)
	}
}

func TestClaimResponse_Rewards(t *testing.T) {
	resp := ClaimResponse{Code: 0, Data: readFixture(t, "claim_rewards.json")}
	rewards := resp.Rewards()
	if len(rewards) != 2 || rewards[0].Name != "LMD" || *rewards[0].Count != 500 || rewards[1].Name != "Gold" {
		t.Errorf("Rewards() = %+v; want [LMD x500, Gold]", rewards)
	}
}

func TestWalkAttendanceItems(t *testing.T) {
	raw := readFixture(t, "attendance_walk.json")
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	var visited []string
	walkAttendanceItems(data, func(obj map[string]any) {
		visited = append(visited, obj["awardId"].(string))
	})
	if len(visited) != 2 || visited[0] != "a1" || visited[1] != "a2" {
		t.Errorf("walkAttendanceItems() visited = %v; want [a1 a2]", visited)
	}
}

func TestRewardFromResource(t *testing.T) {
	resources := map[string]any{"r1": map[string]any{"name": "Orundum", "count": float64(200)}}
	reward := rewardFromResource(resources, "r1")
	if reward.ID != "r1" || reward.Name != "Orundum" || *reward.Count != 200 {
		t.Errorf("rewardFromResource() = %+v; want Orundum x200", reward)
	}
	reward2 := rewardFromResource(resources, "missing")
	if reward2.Name != "missing" {
		t.Errorf("rewardFromResource() name = %q; want 'missing'", reward2.Name)
	}
}