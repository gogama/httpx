// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"errors"
	"fmt"
	"httpx/request"
	"net/http"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDecider(t *testing.T) {
	// Retryable status codes
	t.Run("Retryable status codes", func(t *testing.T) {
		codes := []int{429, 502, 503, 504}
		for i, code := range codes {
			e := request.Execution{
				Response: &http.Response{StatusCode: code},
			}
			t.Run(fmt.Sprintf("codes[%d]=%d", i, code), func(t *testing.T) {
				for j := 0; j < DefaultTimes; j++ {
					e.Attempt = j
					assert.True(t, DefaultDecider(&e), fmt.Sprintf("Expect true for attempt %d", j))
				}
				e.Attempt = DefaultTimes
				assert.False(t, DefaultDecider(&e), fmt.Sprintf("Expect false for attempt %d", e.Attempt))
			})
		}
	})
	// Non-retryable status codes
	t.Run("Non-retryable status codes", func(t *testing.T) {
		codes := []int{200, 201, 202, 203, 204, 205, 400, 401, 402, 403, 404, 500}
		for i, code := range codes {
			e := request.Execution{
				Response: &http.Response{StatusCode: code},
			}
			t.Run(fmt.Sprintf("codes[%d]=%d", i, code), func(t *testing.T) {
				e.Attempt = 0
				assert.False(t, DefaultDecider(&e), "Expect false for attempt 0")
				e.Attempt = 4
				assert.False(t, DefaultDecider(&e), "Expect false for attempt 4")
			})
		}
	})
	// Transient errors
	t.Run("Transient errors", func(t *testing.T) {
		for i, te := range transientErrs {
			e := request.Execution{
				Err: te,
			}
			t.Run(fmt.Sprintf("transientErrs[%d]=%v", i, te), func(t *testing.T) {
				for j := 0; j < DefaultTimes; j++ {
					e.Attempt = j
					assert.True(t, DefaultDecider(&e), fmt.Sprintf("Expect true for attempt %d", j))
				}
				e.Attempt = DefaultTimes
				assert.False(t, DefaultDecider(&e), fmt.Sprintf("Expect false for attempt %d", e.Attempt))
			})
		}
	})
	// Non-transient errors
	t.Run("Non-transient errors", func(t *testing.T) {
		for i, nte := range nonTransientErrs {
			e := request.Execution{
				Err: nte,
			}
			t.Run(fmt.Sprintf("nonTransientErrs[%d]=%v", i, nte), func(t *testing.T) {
				e.Attempt = 0
				assert.False(t, DefaultDecider(&e), "Expect false for attempt 0")
				e.Attempt = 4
				assert.False(t, DefaultDecider(&e), "Expect false for attempt 4")
			})
		}
	})
}

func TestTransientErr(t *testing.T) {
	e := request.Execution{}
	for i, te := range transientErrs {
		t.Run(fmt.Sprintf("transientErrs[%d]=%v", i, te), func(t *testing.T) {
			e.Err = te
			assert.True(t, transientErr(&e))
			e.Err = &url.Error{Err: te}
			assert.True(t, transientErr(&e))
		})
	}
	for j, nte := range nonTransientErrs {
		t.Run(fmt.Sprintf("nonTransientErrs[%d]=%v", j, nte), func(t *testing.T) {
			e.Err = nte
			assert.False(t, transientErr(&e))
			e.Err = &url.Error{Err: nte}
			assert.False(t, transientErr(&e))
		})
	}
}

func TestDeciderAnd(t *testing.T) {
	true_ := DeciderFunc(func(_ *request.Execution) bool { return true })
	false_ := DeciderFunc(func(_ *request.Execution) bool { return false })
	tt := true_.And(true_)
	tf := true_.And(false_)
	ft := false_.And(true_)
	ff := false_.And(false_)
	assert.True(t, tt(&request.Execution{}))
	assert.False(t, tf(&request.Execution{}))
	assert.False(t, ft(&request.Execution{}))
	assert.False(t, ff(&request.Execution{}))
}

func TestDeciderOr(t *testing.T) {
	true_ := DeciderFunc(func(_ *request.Execution) bool { return true })
	false_ := DeciderFunc(func(_ *request.Execution) bool { return false })
	tt := true_.Or(true_)
	tf := true_.Or(false_)
	ft := false_.Or(true_)
	ff := false_.Or(false_)
	assert.True(t, tt(&request.Execution{}))
	assert.True(t, tf(&request.Execution{}))
	assert.True(t, ft(&request.Execution{}))
	assert.False(t, ff(&request.Execution{}))
}

func TestTimes(t *testing.T) {
	zero := Times(0)
	assert.False(t, zero(&request.Execution{}))
	one := Times(1)
	assert.True(t, one(&request.Execution{}))
	assert.False(t, one(&request.Execution{Attempt: 1}))
	two := Times(2)
	assert.True(t, two(&request.Execution{Attempt: 1}))
	assert.False(t, two(&request.Execution{Attempt: 2}))
}

func TestBefore(t *testing.T) {
	e := request.Execution{Start: time.Now()}
	before := Before(time.Minute)
	for i := 0; i < 20; i++ {
		e.Attempt = 20
		assert.True(t, before(&e))
	}
	e.End = e.Start.Add(2 * time.Minute)
	assert.False(t, before(&e))
}

func TestStatusCode(t *testing.T) {
	empty := StatusCode()
	assert.False(t, empty(&request.Execution{}))
	one := StatusCode(602)
	assert.False(t, one(&request.Execution{}))
	r := http.Response{}
	e := request.Execution{Response: &r}
	assert.False(t, empty(&e))
	assert.False(t, one(&e))
	r.StatusCode = 602
	assert.True(t, one(&e))
	two := StatusCode(509, 602)
	assert.True(t, two(&e))
	r.StatusCode = 509
	assert.True(t, two(&e))
	r.StatusCode = 508
	assert.False(t, two(&e))
}

var (
	transientErrs = []error{
		syscall.ECONNREFUSED,
		syscall.ECONNRESET,
		syscall.ETIMEDOUT,
	}
	nonTransientErrs = []error{
		nil,
		errors.New("ain't transient"),
		syscall.EHOSTUNREACH,
		syscall.ENETDOWN,
	}
)
