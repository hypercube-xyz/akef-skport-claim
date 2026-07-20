package skport

import "testing"

func TestResponseMalformedAndRewardShapes(t *testing.T) {
	for _, data := range []string{"", "null", "{"} {
		response := AttendanceResponse{Data: []byte(data)}
		if root := response.root(); root != nil {
			t.Fatalf("root(%q)=%#v", data, root)
		}
	}
	if rewards := (ClaimResponse{Data: []byte("{")}).Rewards(); rewards != nil {
		t.Fatalf("malformed rewards=%#v", rewards)
	}
	response := ClaimResponse{Data: []byte(`{"awardIds":[{"awardId":"a"},{"id":"b"},42],"resourceInfoMap":{"a":{"name":"A"},"b":{"name":"B"}}}`)}
	rewards := response.Rewards()
	if len(rewards) != 2 || rewards[0].Name != "A" || rewards[1].Name != "B" {
		t.Fatalf("mapped rewards=%#v", rewards)
	}
}
