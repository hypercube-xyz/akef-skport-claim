// Package result defines the outcome of one application run.
package result

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
	ID    string
	Name  string
	Count *uint64
}

func (r Reward) Summary() string {
	if r.Count == nil {
		return r.Name
	}
	return r.Name + " x" + strconv.FormatUint(*r.Count, 10)
}

type Account struct {
	Name    string
	Outcome Outcome
	Summary string
	Rewards []Reward
}

type Run struct {
	Duration time.Duration
	Accounts []Account
}
