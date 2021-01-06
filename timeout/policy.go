// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package timeout

import (
	"time"

	"github.com/gogama/httpx/request"
)

// A Policy defines a timeout policy which may be plugged into the
// robust HTTP client (httpx.Client) to direct how to set the request
// timeout for the initial attempt, as well as for any subsequent retries.
//
// Implementations of Policy must be safe for concurrent use by multiple
// goroutines.
type Policy interface {
	// Timeout returns the timeout to set on the next HTTP request
	// attempt within the plan execution
	//
	// Parameter e contains the current state of the HTTP request plan
	// execution. The return value is the timeout to set on the next
	// request attempt within the execution.
	Timeout(e *request.Execution) time.Duration
}

// DefaultPolicy is the default timeout policy. It sets a fixed timeout
// of 5 seconds on each attempt.
var DefaultPolicy Policy = Fixed(5 * time.Second)

// Infinite is a built-in timeout policy which never times out.
var Infinite Policy = Fixed(1<<63 - 1)

// Fixed constructs a timeout policy that uses the same value to set
// every attempt timeout. The return value is a timeout policy that
// always returns the value d.
//
// Use Fixed to create the typical timeout behavior supported by most
// retrying HTTP client software.
func Fixed(d time.Duration) Policy {
	return policy([]time.Duration{d})
}

// Adaptive constructs a timeout policy that varies the next timeout
// value if the previous attempt timed out.
//
// Use Adaptive if you find the remote service often exhibits one-off slow
// response times that can be cured by quickly timing out and retrying,
// but you also need to protect your application (and the remote service)
// from retry storms and failure if the remote service goes through a
// burst of slowness where most response times during the burst are
// slower than your usual quick timeout.
//
// Parameter usual represents the timeout value the policy will return
// for an initial attempt and for any retry where the immediately
// preceding attempt did not time out.
//
// Parameter after contains timeout values the policy will return if
// the previous attempt timed out. If this was the first timeout of the
// execution, after[0] is returned; if the second, after[1], and so on.
// If more attempts have timed out within the execution than after has
// elements, then the last element of after is returned.
//
// Consider the following timeout policy:
//
// 	p := Adaptive(200*time.Millisecond, time.Second, 10*time.Second)
//
// The policy p will use 200 milliseconds as the usual timeout but if
// the preceding attempt timed out and was the first timeout of the
// execution, it will use 1 second; and if the previous attempt timed
// out and was not the first attempt, it will use 10 seconds.
func Adaptive(usual time.Duration, after ...time.Duration) Policy {
	p := make([]time.Duration, 1, 1+len(after))
	p[0] = usual
	return policy(append(p, after...))
}

type policy []time.Duration

func (p policy) Timeout(e *request.Execution) time.Duration {
	if !e.Timeout() {
		return p[0]
	}

	i := e.AttemptTimeouts
	if i > len(p)-1 {
		i = len(p) - 1
	}

	return p[i]
}
