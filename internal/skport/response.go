package skport

import (
	"encoding/json"
	"math"
	"sort"

	"github.com/hypercube-xyz/akef-skport-claim/internal/result"
)

type APIResponse struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type RefreshResponse struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"`
	} `json:"data"`
}

type AttendanceResponse APIResponse
type ClaimResponse APIResponse

type ClaimClass int

const (
	ClaimSuccess ClaimClass = iota
	ClaimAlreadyDone
	ClaimAuthError
	ClaimAPIError
)

var availableKeys = map[string]struct{}{
	"available": {}, "canClaim": {}, "can_claim": {}, "isAvailable": {}, "is_available": {},
}
var doneKeys = map[string]struct{}{
	"done": {}, "isDone": {}, "is_done": {}, "alreadyDone": {}, "already_done": {},
	"isClaimed": {}, "claimed": {}, "hasClaimed": {}, "has_claimed": {},
}
var hasTodayKeys = map[string]struct{}{
	"hasToday": {}, "has_today": {},
}

// AttendanceState is the normalized attendance decision derived from an API
// response. Known fields are tracked separately because a missing field must not
// be interpreted as false. Conflict is set when any attendance item says it is
// both available and done; the conservative action is to fail the entire response
// closed and never send a claim POST from contradictory data.
type AttendanceState struct {
	Available      bool
	AvailableKnown bool
	Done           bool
	DoneKnown      bool
	Conflict       bool
	HasToday       bool
	HasTodayKnown  bool
	SessionValid   bool
}

func (r AttendanceResponse) root() any {
	var value any
	if len(r.Data) == 0 || string(r.Data) == "null" || json.Unmarshal(r.Data, &value) != nil {
		return nil
	}
	return value
}

// State evaluates the root compatibility shape and nested attendance items rather
// than accepting available/done fields from arbitrary metadata. This distinction
// matters when historical calendar entries are done while today's entry is
// available, and prevents unrelated nested objects from triggering a claim.
func (r AttendanceResponse) State() AttendanceState {
	root := r.root()
	state := AttendanceState{}

	var cleanAvailable bool
	var anyDone bool
	evaluate := func(object map[string]any) {
		available, availableKnown := directBool(object, availableKeys)
		done, doneKnown := directBool(object, doneKeys)
		state.AvailableKnown = state.AvailableKnown || availableKnown
		state.DoneKnown = state.DoneKnown || doneKnown

		conflict := availableKnown && available && doneKnown && done
		if conflict {
			state.Conflict = true
			anyDone = true
			return
		}
		if availableKnown && available {
			cleanAvailable = true
		}
		if doneKnown && done {
			anyDone = true
		}
	}

	if object, ok := root.(map[string]any); ok {
		state.HasToday, state.HasTodayKnown = directBool(object, hasTodayKeys)
		// Some observed and legacy response shapes expose state directly on data.
		evaluate(object)
		for _, nested := range object {
			walkJSON(nested, evaluate)
		}
	} else {
		walkJSON(root, evaluate)
	}

	switch {
	case state.Conflict:
		// Any contradictory attendance item makes the response unsafe to act on.
		// Failing closed is preferable to issuing a duplicate claim POST.
		state.Available = false
		state.Done = true
	case cleanAvailable:
		state.Available = true
		state.Done = false
	case anyDone:
		state.Available = false
		state.Done = true
	}
	state.SessionValid = r.Code == 0 && (!state.HasTodayKnown || state.HasToday || state.Available || state.Done)
	return state
}

func (r AttendanceResponse) AvailableRewards() []result.Reward {
	if r.State().Conflict {
		return nil
	}
	root := r.root()
	resourceMap := resourceInfoMap(root)
	ids := map[string]bool{}
	collectAvailableIDs(root, ids)
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	rewards := make([]result.Reward, 0, len(keys))
	for _, id := range keys {
		rewards = append(rewards, rewardFromResource(resourceMap, id))
	}
	return rewards
}

func (r ClaimResponse) Classify() ClaimClass {
	switch {
	case r.Code == 0:
		return ClaimSuccess
	case r.Code == 10001:
		return ClaimAlreadyDone
	case IsAuthCode(r.Code):
		return ClaimAuthError
	default:
		return ClaimAPIError
	}
}

func (r ClaimResponse) Rewards() []result.Reward {
	var root map[string]any
	if json.Unmarshal(r.Data, &root) != nil {
		return nil
	}
	values, _ := root["awardIds"].([]any)
	resources := resourceInfoMap(root)
	rewards := make([]result.Reward, 0, len(values))
	for _, value := range values {
		id := ""
		switch item := value.(type) {
		case string:
			id = item
		case map[string]any:
			id, _ = item["id"].(string)
			if id == "" {
				id, _ = item["awardId"].(string)
			}
		}
		if id != "" {
			rewards = append(rewards, rewardFromResource(resources, id))
		}
	}
	return rewards
}

func IsAuthCode(code int64) bool {
	switch code {
	case 401, 403, 10002, 10003, 10004, 20001, 20002:
		return true
	default:
		return false
	}
}

func directBool(object map[string]any, keys map[string]struct{}) (bool, bool) {
	knownFalse := false
	for key, item := range object {
		if _, wanted := keys[key]; !wanted {
			continue
		}
		value, ok := item.(bool)
		if !ok {
			continue
		}
		if value {
			return true, true
		}
		knownFalse = true
	}
	return false, knownFalse
}

func walkJSON(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, nested := range typed {
			walkJSON(nested, visit)
		}
	case []any:
		for _, nested := range typed {
			walkJSON(nested, visit)
		}
	}
}

func walkAttendanceItems(value any, visit func(map[string]any)) {
	walkJSON(value, func(obj map[string]any) {
		if awardID, ok := obj["awardId"].(string); ok && awardID != "" {
			visit(obj)
		}
	})
}

func collectAvailableIDs(value any, ids map[string]bool) {
	walkJSON(value, func(obj map[string]any) {
		available, availableKnown := directBool(obj, availableKeys)
		done, doneKnown := directBool(obj, doneKeys)
		if availableKnown && available && (!doneKnown || !done) {
			if id, ok := obj["awardId"].(string); ok {
				ids[id] = true
			}
		}
	})
}

func resourceInfoMap(value any) map[string]any {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	resources, _ := root["resourceInfoMap"].(map[string]any)
	return resources
}

func rewardFromResource(resources map[string]any, id string) result.Reward {
	reward := result.Reward{ID: id, Name: id}
	item, _ := resources[id].(map[string]any)
	if name, ok := item["name"].(string); ok && name != "" {
		reward.Name = name
	}
	if count, ok := item["count"].(float64); ok && count >= 0 && count <= math.MaxUint64 && math.Trunc(count) == count {
		value := uint64(count)
		reward.Count = &value
	}
	return reward
}
