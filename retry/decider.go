// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"httpx/request"
	"httpx/transient"
	"time"
)

// A Decider decides if a retry should be done.
//
// Implementations of Waiter must be safe for concurrent use by multiple
// goroutines.
//
// This package provides one Decider implementation, DeciderFunc. It
// also provides functions for constructing and composing DeciderFunc
// instances, and a default instance, DefaultDecider, which is suitable
// for many use cases.
type Decider interface {
	// Decide returns true if a retry should be done, and false
	// otherwise, after examining the current HTTP request plan
	// execution state.
	Decide(e *request.Execution) bool
}

// A DeciderFunc returns true if a retry should be done, and false
// otherwise, after examining the current HTTP request plan execution
// state.
//
// Every DeciderFunc must be safe for concurrent use by multiple
// goroutines.
//
// Simple DeciderFunc functions can be composed into complex decision trees
// using the logical composition functions Decider.And and DeciderFunc.Or.
type DeciderFunc func(e *request.Execution) bool

// DefaultTimes is the number of times DefaultPolicy will retry.
const DefaultTimes = 5

// DefaultDecider is a general-purpose retry decider suitable for
// common use cases. It will allow up to DefaultTimes retries (i.e. up
// to 6 total attempts), and will retry in the case of a transient error
// (TransientErr) or if a valid HTTP response is received but it contains
// one of the following status codes: 429 (Too Many Requests); 502 (Bad
// Gateway); 503 (Service Unavailable); or 504 (Gateway Timeout).
var DefaultDecider = Times(DefaultTimes).And(StatusCode(429, 502, 503, 504).Or(TransientErr))

// TransientErr is a decider that indicates a retry if the current
// error is transient according to transient.Categorize.
//
// TransientErr only looks at the error, so it will always return false
// if a valid HTTP response is returned. Compose it with other deciders,
// for example a status code decider constructed with StatusCodes(), to
// get more complex functionality.
var TransientErr DeciderFunc = transientErr

func (f DeciderFunc) Decide(e *request.Execution) bool {
	return f(e)
}

// And composes two retry deciders into a new decider which returns true
// if both sub-deciders return true, and false otherwise.
func (f DeciderFunc) And(g DeciderFunc) DeciderFunc {
	return func(e *request.Execution) bool {
		return f(e) && g(e)
	}
}

// Or composes two retry deciders into a new decider which returns
// true if either of the two sub-deciders returns true, but false if
// they both return false.
func (f DeciderFunc) Or(g DeciderFunc) DeciderFunc {
	return func(e *request.Execution) bool {
		return f(e) || g(e)
	}
}

// Times constructs a retry decider which allows up to n retries. The
// returned decider returns true while the execution attempt index
// e.Attempt is less than n, and false otherwise.
func Times(n int) DeciderFunc {
	return func(e *request.Execution) bool {
		return e.Attempt < n
	}
}

// Before constructs a retry decider allowing retries until a certain
// amount of time has passed. The returned decider returns true while
// the execution duration is less than d, and false afterward.
func Before(d time.Duration) DeciderFunc {
	return func(e *request.Execution) bool {
		return e.Duration() < d
	}
}

// StatusCode constructs a retry decider allowing based on the HTTP
// response status code. If the most recent request attempt within the
// plan execution received a valid HTTP response, and the response
// status code is contained in the list ss, the decider returns true.
// Otherwise, it returns false.
func StatusCode(ss ...int) DeciderFunc {
	ss2 := make([]int, len(ss))
	copy(ss2, ss)
	return func(e *request.Execution) bool {
		for _, s := range ss2 {
			if e.StatusCode() == s {
				return true
			}
		}
		return false
	}
}

func transientErr(e *request.Execution) bool {
	return transient.Categorize(e.Err) != transient.Not
}
