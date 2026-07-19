package model

import (
	"strconv"
	"time"
)

type Outcome string

const (
	Claimed        Outcome = "claimed"
	AlreadyClaimed Outcome = "already_claimed"
	Unavailable    Outcome = "unavailable"
	AuthExpired    Outcome = "auth_expired"
	TransientError Outcome = "transient_error"
	ClaimError     Outcome = "claim_error"
	AmbiguousClaim Outcome = "claim_ambiguous"
	InternalError  Outcome = "internal_error"
	Skipped        Outcome = "skipped"
)

type Reward struct {
	ID    string  `json:"id,omitempty"`
	Name  string  `json:"name"`
	Count *uint64 `json:"count,omitempty"`
}

func (r Reward) Summary() string {
	if r.Count == nil {
		return r.Name
	}
	return r.Name + " x" + strconv.FormatUint(*r.Count, 10)
}

type AccountResult struct {
	Account string
	Outcome Outcome
	Summary string
	Rewards []Reward
}

type RunReport struct {
	Duration time.Duration
	Results  []AccountResult
}
