// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"time"

	"github.com/gogama/httpx/request"
)

// A Policy controls if and how retries are done in an HTTP request
// plan execution. In particular, after every attempt during the HTTP
// request plan execution, a Policy decides whether a retry should be
// done and, if so, how long the wait period should be before retrying
// the attempt.
//
// Implementations of Policy must be safe for concurrent use by multiple
// goroutines.
//
// A Policy is composed of the Decider and Waiter interfaces. While you
// can implement Policy yourself, it may be more efficient to use one
// of the built-in retry policies, DefaultPolicy or Never, or to construct
// your policy using the NewPolicy constructor using existing Decider
// and Waiter implementations.
type Policy interface {
	Decider
	Waiter
}

// DefaultPolicy is a general-purpose retry policy suitable for common
// use cases. It is a composition of DefaultDecider for retry decisions
// and DefaultWaiter for wait time calculations.
var DefaultPolicy Policy = policy{DefaultDecider, DefaultWaiter}

// Never is a policy that never retries. It is useful if you want to use
// the other features of httpx.Client but do not want retries.
var Never Policy = policy{Times(0), DefaultWaiter}

type policy struct {
	decider Decider
	waiter  Waiter
}

// NewPolicy composes a Decider and a Waiter into a retry Policy.
func NewPolicy(d Decider, w Waiter) Policy {
	return policy{decider: d, waiter: w}
}

func (p policy) Decide(e *request.Execution) bool {
	return p.decider.Decide(e)
}

func (p policy) Wait(e *request.Execution) time.Duration {
	return p.waiter.Wait(e)
}
