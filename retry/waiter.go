// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"math/rand"
	"sync"
	"time"

	"github.com/gogama/httpx/request"
)

// A Waiter specifies how long to wait before retrying a failed HTTP request
// attempt.
//
// Implementations of Waiter must be safe for concurrent use by multiple
// goroutines.
//
// The robust HTTP client, httpx.Client, will not call the Waiter on a
// retry policy if the policy Decider returned false.
//
// This package provides one Waiter implementations, using the constructor
// function NewExpWaiter. In addition it provides a concrete instance
// suitable for many typical use cases, DefaultWaiter.
type Waiter interface {
	Wait(e *request.Execution) time.Duration
}

// DefaultWaiter is the default retry wait policy. It uses a jittered
// exponential backoff formula with a base wait of 50 milliseconds and a
// maximum wait of 1 second.
var DefaultWaiter = NewExpWaiter(50*time.Millisecond, 1*time.Second, time.Now())

// NewFixedWaiter constructs a Waiter that always returns the given
// duration.
//
// Use NewFixedWaiter to obtain a constant retry backoff.
func NewFixedWaiter(d time.Duration) Waiter {
	return fixedWaiter(d)
}

type fixedWaiter time.Duration

func (w fixedWaiter) Wait(_ *request.Execution) time.Duration {
	return time.Duration(w)
}

// NewExpWaiter constructs a Waiter implementing an exponential backoff
// formula with optional jitter.
//
// The formula implemented is the "Full Jitter" approach described in:
// https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter.
//
// Parameters base and max control the exponential calculation of the
// ceiling:
//
//	ceil := max(base * 2**attempt, max)
//
// Base and max must be positive values, and max must be at least equal
// to base.
//
// Parameter jitter is used to generate a random number between 0 and
// ceil. To make a waiter that does not jitter and simply returns
// ceil on each attempt, pass nil for jitter. Otherwise you may specify
// either a random number generator seed value (as a time.Time, int, or
// int64) or a random number generator (as a rand.Source). If a seed
// value is specified, it is used to seed a random number generator
// for calculating jitter. If a rand.Source is specified, it is used to
// calculate jitter.
func NewExpWaiter(base, max time.Duration, jitter interface{}) Waiter {
	if base < 1 {
		panic("httpx/retry: base must be positive")
	}
	if max < base {
		panic("httpx/retry: max must be at least base")
	}
	r := jitterToRand(jitter)
	return &jitterExpWaiter{
		base: base,
		max:  max,
		rand: r,
	}
}

type jitterExpWaiter struct {
	base time.Duration
	max  time.Duration
	rand *rand.Rand
	lock sync.Mutex
}

func (w *jitterExpWaiter) Wait(e *request.Execution) time.Duration {
	exp := int64(1) << e.Attempt
	if exp < 1 {
		exp = 1<<63 - 1
	}

	ceil := int64(w.base) * exp
	if ceil < int64(w.base) || int64(w.max) < ceil {
		ceil = int64(w.max)
	}

	duration := ceil
	if ceil > 0 {
		w.lock.Lock()
		defer w.lock.Unlock()
		if w.rand != nil {
			duration = w.rand.Int63n(ceil)
		}
	}

	return time.Duration(duration)
}

func jitterToRand(jitter interface{}) *rand.Rand {
	var s rand.Source
	switch j := jitter.(type) {
	case nil:
		return nil
	case time.Time:
		s = rand.NewSource(j.UnixNano())
	case int:
		s = rand.NewSource(int64(j))
	case int64:
		s = rand.NewSource(j)
	case *rand.Rand:
		if j == nil {
			panic("httpx/retry: jitter may not be a typed nil")
		}
		return j
	case rand.Source:
		s = j
	default:
		panic("httpx/retry: invalid jitter type")
	}
	return rand.New(s)
}
