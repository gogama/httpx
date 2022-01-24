// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package timeout

import (
	"errors"
	"math"
	"syscall"
	"testing"
	"time"

	"github.com/gogama/httpx/request"
	"github.com/stretchr/testify/assert"
)

func TestDefault(t *testing.T) {
	a := DefaultPolicy.Timeout(&request.Execution{})
	assert.Equal(t, 5*time.Second, a)
	b := DefaultPolicy.Timeout(&request.Execution{AttemptTimeouts: 3, Err: syscall.ETIMEDOUT, Body: []byte("foo")})
	assert.Equal(t, 5*time.Second, b)
}

func TestInfinite(t *testing.T) {
	a := Infinite.Timeout(&request.Execution{})
	assert.Equal(t, time.Duration(math.MaxInt64), a)
	b := Infinite.Timeout(&request.Execution{AttemptTimeouts: 10, Err: syscall.ETIMEDOUT})
	assert.Equal(t, time.Duration(math.MaxInt64), b)
}

func TestFixed(t *testing.T) {
	p := Fixed(33 * time.Hour)
	a := p.Timeout(&request.Execution{})
	assert.Equal(t, 33*time.Hour, a)
	b := p.Timeout(&request.Execution{AttemptTimeouts: 1, Err: syscall.ETIMEDOUT, Attempt: 1})
	assert.Equal(t, 33*time.Hour, b)
	c := p.Timeout(&request.Execution{AttemptTimeouts: 2, Err: syscall.ETIMEDOUT, Attempt: 2})
	assert.Equal(t, 33*time.Hour, c)
}

func TestAdaptive(t *testing.T) {
	p := Adaptive(5*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond)
	x := &request.Execution{}
	assert.Equal(t, 5*time.Millisecond, p.Timeout(x))
	x.Attempt = 0
	x.AttemptTimeouts = 1
	x.Err = syscall.ETIMEDOUT
	assert.Equal(t, 10*time.Millisecond, p.Timeout(x))
	x.Attempt = 1
	x.Err = errors.New("just a routine problem")
	assert.Equal(t, 5*time.Millisecond, p.Timeout(x))
	x.Attempt = 2
	x.AttemptTimeouts = 2
	assert.Equal(t, 5*time.Millisecond, p.Timeout(x))
	x.Err = syscall.ETIMEDOUT
	assert.Equal(t, 100*time.Millisecond, p.Timeout(x))
	x.Attempt = 3
	x.AttemptTimeouts = 3
	assert.Equal(t, 100*time.Millisecond, p.Timeout(x))
	x.Attempt = 4
	x.AttemptTimeouts = 3
	assert.Equal(t, 100*time.Millisecond, p.Timeout(x))
}
