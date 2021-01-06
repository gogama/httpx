// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"time"

	"github.com/gogama/httpx/request"
	"github.com/gogama/httpx/transient"
)

// A Decider decides if a retry should be done.
//
// Implementations of Decider must be safe for concurrent use by
// multiple goroutines.
//
// Use the built-in constructors Times, StatusCode, and Before, and the
// built-in decider TransientErr; or implement your Decider. Use
// DeciderFunc to convert an ordinary function into a Decider, and to
// compose deciders logically using DeciderFunc.And and DeciderFunc.Or.
type Decider interface {
	Decide(e *request.Execution) bool
}

// The DeciderFunc type is an adapter to allow the use of ordinary
// functions as retry deciders. It implements the Decider interface, and
// also provides the logical composition methods And and Or.
//
// Every DeciderFunc must be safe for concurrent use by multiple
// goroutines.
//
// Simple DeciderFunc functions can be composed into complex decision
// trees using the logical composition functions DeciderFunc.And and
// DeciderFunc.Or. Because of this composition ability, it will often
// be convenient to work directly with DeciderFunc rather than with
// Decider.
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
// for example a status code decider constructed with StatusCode, to
// get more complex functionality.
var TransientErr DeciderFunc = transientErr

// Decide returns true if a retry should be done, and false otherwise,
// after examining the current HTTP request plan execution state.
func (f DeciderFunc) Decide(e *request.Execution) bool {
	return f(e)
}

// And composes two retry deciders into a new decider which returns true
// if both sub-deciders return true, and false otherwise.
//
// Short-circuit logic is used, so g will not be evaluated if f returns
// false.
func (f DeciderFunc) And(g DeciderFunc) DeciderFunc {
	return func(e *request.Execution) bool {
		return f(e) && g(e)
	}
}

// Or composes two retry deciders into a new decider which returns
// true if either of the two sub-deciders returns true, but false if
// they both return false.
//
// Short-circuit logic is used, so g will not be evaluated if f returns
// true.
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
// amount of time has elapsed since the start of the logical HTTP request
// plan execution. The returned decider returns true while the execution
// duration is less than d, and false afterward.
func Before(d time.Duration) DeciderFunc {
	return func(e *request.Execution) bool {
		return e.Duration() < d
	}
}

// StatusCode constructs a retry decider allowing retries based on the
// HTTP response status code. If the most recent request attempt within
// the plan execution received a valid HTTP response, and the response
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
