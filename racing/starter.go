// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"sync"
	"time"

	"github.com/gogama/httpx/request"
)

// A Starter starts or discards a previously scheduled parallel request.
//
// Implementations of Scheduler must be safe for concurrent use by
// multiple goroutines.
//
// Use NewPolicy to compose a Scheduler with a Starter to make a racing
// policy.
type Starter interface {
	// Start returns true if a previously scheduled request attempt
	// should be added to the race and false if it should be discarded.
	//
	// If the scheduled request attempt is discarded, the wave is closed
	// and no new request attempts will be scheduled until the next
	// wave.
	//
	// The execution contains the plan execution state existing at the
	// current time, which will usually be changed from the state
	// existing at the time the attempt was scheduled.
	Start(*request.Execution) bool
}

// AlwaysStart is a starter that starts every scheduled request.
var AlwaysStart = alwaysStarter(0)

type alwaysStarter int

func (st alwaysStarter) Start(_ *request.Execution) bool {
	return true
}

// A Limit specifies the maximum number of request attempts allowed per
// unit time.
type Limit struct {
	MaxAttempts int
	Period      time.Duration
}

// NewThrottleStarter constructs a starter which throttles new request
// attempts based one or more limits.
//
// For example, the following starter would block starting any new
// parallel request attempts if more than 10 parallel request attempts
// have been started in the last half second, or more than 15 have been
// started in the last second:
//
//	s := racing.NewThrottleStarter(
//		racing.Limit{MaxAttempts: 10, Period: 500*time.Millisecond},
//		racing.Limit{MaxAttempts: 15, Period: 1*time.Second))
//
// Note that as with all starters, the constructed throttling starter
// only affects the starting of concurrent request attempts. Every wave
// starts with one request attempt.
func NewThrottleStarter(limits ...Limit) Starter {
	st := &throttleStarter{
		limits: make([]limitQueue, len(limits)),
	}
	for i, l := range limits {
		st.limits[i] = newLimitQueue(l.Period, l.MaxAttempts)
	}
	return st
}

type throttleStarter struct {
	limits []limitQueue
	lock   sync.Mutex
}

func (st *throttleStarter) Start(_ *request.Execution) bool {
	st.lock.Lock()
	defer st.lock.Unlock()
	now := time.Now()
	start := true
	for i := range st.limits {
		start = start && st.limits[i].accept(&now)
	}
	return start
}

type limitQueue struct {
	antiPeriod time.Duration
	a          []time.Time
	start, len int
}

func newLimitQueue(period time.Duration, cap int) limitQueue {
	return limitQueue{
		antiPeriod: -period,
		a:          make([]time.Time, cap),
	}
}

func (q *limitQueue) accept(t *time.Time) bool {
	cutoff := t.Add(q.antiPeriod)
	// Remove all samples added at or before cutoff.
	n := min(q.start+q.len, len(q.a))
	for i := q.start; i < n; i++ {
		if !cutoff.Before(q.a[i]) {
			q.start++
			q.len--
		}
	}
	if q.start >= len(q.a) {
		q.start = 0
		n = q.len
		for j := 0; j < n; j++ {
			if !cutoff.Before(q.a[j]) {
				q.start++
				q.len--
			}
		}
	}
	// If there's room for the sample, add it in.
	if q.len < len(q.a) {
		i := (q.start + q.len) % len(q.a)
		q.a[i] = *t
		q.len++
		return true
	}
	// Otherwise, don't accept the sample.
	return false
}

func min(x, y int) int {
	if x <= y {
		return x
	}
	return y
}
