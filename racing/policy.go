// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"time"

	"github.com/gogama/httpx/request"
)

// TODO: document me
var Disabled = disabled{}

// TODO: document me
type Policy interface {
	Scheduler
	Confirmer
}

type policy struct {
	scheduler Scheduler
	confirmer Confirmer
}

func NewPolicy(s Scheduler, c Confirmer) Policy {
	if s == nil {
		panic("httpx/racing: nil scheduler")
	}
	if c == nil {
		panic("httpx/racing: nil confirmer")
	}
	return policy{scheduler: s, confirmer: c}
}

func (p policy) Schedule(e *request.Execution) time.Duration {
	return p.scheduler.Schedule(e)
}

func (p policy) Confirm(e *request.Execution) bool {
	return p.confirmer.Confirm(e)
}

type disabled struct{}

func (_ disabled) Schedule(_ *request.Execution) time.Duration {
	return 0
}

func (_ disabled) Confirm(_ *request.Execution) bool {
	return false
}
