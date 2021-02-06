// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"time"

	"github.com/gogama/httpx/request"
)

// A Scheduler schedules when the next concurrent request attempt should
// join the wave.
//
// Implementations of Scheduler must be safe for concurrent use by
// multiple goroutines.
//
// Use NewPolicy to compose a Scheduler with a Starter to make a racing
// policy.
type Scheduler interface {
	// Schedule returns the duration to wait before starting the next
	// parallel request attempt in the current wave. A zero return value
	// halts the race for the wave, meaning no new parallel requests
	// will be started until the next wave.
	Schedule(*request.Execution) time.Duration
}

// NewStaticScheduler constructs a scheduler producing a fixed-size wave
// of request attempts, each starting at a fixed offset from the previous
// attempt.
//
// Each entry in offset specifies the delay to wait, after starting the
// previous request in the wave before, starting the next one. So if
// delay[0] is 250ms, then the second attempt in the wave will be
// started 250ms after the first attempt; if delay[1] is 500ms, the
// third attempt will be started 500ms after the second one (750ms after
// the first one) and so on.
func NewStaticScheduler(offset ...time.Duration) Scheduler {
	p := make([]time.Duration, len(offset))
	copy(p, offset)
	return staticScheduler(p)
}

type staticScheduler []time.Duration

func (sc staticScheduler) Schedule(e *request.Execution) time.Duration {
	if e.Racing >= len(sc) {
		return 0
	}

	return sc[e.Racing]
}
