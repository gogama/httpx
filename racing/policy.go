// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"time"

	"github.com/gogama/httpx/request"
)

// Disabled is a policy that disables racing, causing the robust HTTP
// client to send all requests serially, and never in parallel.
var Disabled = disabled{}

// A Policy controls if and how concurrent requests may be raced against
// each other. In particular, after every request attempt is started, a
// Policy schedules the start of the next attempt in the wave and, when
// the scheduled time arrives, confirms whether the attempt should
// start.
//
// Implementations of Policy must be safe for concurrent use by multiple
// goroutines.
//
// A Policy is composed of the Scheduler and Starter interfaces. Use
// NewPolicy to construct a Policy given existing Scheduler and Starter
// implementations.
type Policy interface {
	Scheduler
	Starter
}

// NewPolicy composes a Scheduler and a Starter into a racing Policy.
func NewPolicy(sc Scheduler, st Starter) Policy {
	if sc == nil {
		panic("httpx/racing: nil scheduler")
	}
	if st == nil {
		panic("httpx/racing: nil starter")
	}
	return policy{scheduler: sc, starter: st}
}

type policy struct {
	scheduler Scheduler
	starter   Starter
}

func (p policy) Schedule(e *request.Execution) time.Duration {
	return p.scheduler.Schedule(e)
}

func (p policy) Start(e *request.Execution) bool {
	return p.starter.Start(e)
}

type disabled struct{}

func (_ disabled) Schedule(_ *request.Execution) time.Duration {
	return 0
}

func (_ disabled) Start(_ *request.Execution) bool {
	return false
}
