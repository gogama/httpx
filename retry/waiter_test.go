// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"fmt"
	"httpx/request"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultWaiter(t *testing.T) {
	max := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1 * time.Second,
		1 * time.Second,
		1 * time.Second,
		1 * time.Second,
		1 * time.Second,
	}
	for i := 0; i < len(max); i++ {
		wait := DefaultWaiter.Wait(&request.Execution{Attempt: i})
		assert.GreaterOrEqual(t, wait, time.Duration(0))
		assert.LessOrEqual(t, wait, max[i])
	}
}

func TestNewExpWaiter(t *testing.T) {
	base, max := 1*time.Millisecond, 1*time.Hour
	t.Run("invalid base", func(t *testing.T) {
		assert.Panics(t, func() {
			NewExpWaiter(time.Duration(-1), max, nil)
		}, "negative base")
		assert.Panics(t, func() {
			NewExpWaiter(time.Duration(0), max, nil)
		}, "zero base")
	})
	t.Run("invalid max", func(t *testing.T) {
		assert.Panics(t, func() {
			NewExpWaiter(time.Duration(2), time.Duration(1), nil)
		}, "max less than base")
	})
	t.Run("invalid jitter", func(t *testing.T) {
		assert.Panics(t, func() {
			NewExpWaiter(base, max, float64(1))
		}, "float64")
		var nilRand *rand.Rand
		assert.Panics(t, func() {
			NewExpWaiter(base, max, nilRand)
		}, "nil *rand.Rand")
	})
	t.Run("no jitter", func(t *testing.T) {
		var j *jitterExpWaiter
		j = newJitterExpWaiter(t, base, max, nil, "explicit nil")
		assert.Nil(t, j.rand, "explicit nil")
		var s rand.Source
		j = newJitterExpWaiter(t, base, max, s, "nil rand.Source")
		assert.Nil(t, j.rand, "nil rand.Source")
		for i := 0; i < 10; i++ {
			ceil := 1 << i
			assert.Equal(t, time.Duration(ceil)*time.Millisecond, j.Wait(&request.Execution{Attempt: i}))
		}
		assert.Equal(t, max, j.Wait(&request.Execution{Attempt: 25}))
		assert.Equal(t, max, j.Wait(&request.Execution{Attempt: 1000}))
		assert.Equal(t, max, j.Wait(&request.Execution{Attempt: math.MaxInt64}))
	})
	t.Run("with jitter", func(t *testing.T) {
		jitters := []struct {
			name  string
			value interface{}
		}{
			{"zero time.Time", time.Time{}},
			{"time.Now()", time.Now()},
			{"int", 1},
			{"int64", int64(1)},
			{"rand.Source", rand.NewSource(0)},
			{"*rand.Rand", rand.New(rand.NewSource(0))},
		}
		for i, jitter := range jitters {
			t.Run(fmt.Sprintf("jitters[%d]=%s", i, jitter.name), func(t *testing.T) {
				w := NewExpWaiter(base, max, jitter.value)
				for j := 0; j < 100; j++ {
					d := w.Wait(&request.Execution{Attempt: j})
					assert.GreaterOrEqual(t, d, time.Duration(0))
					assert.LessOrEqual(t, d, max)
				}
			})
		}
	})
	t.Run("concurrent rand.Source usage", func(t *testing.T) {
		n := 1000
		w := NewExpWaiter(base, max, 0)
		waitChan := make(chan struct {
			goroutine int
			attempt   int
			wait      time.Duration
		},
		)
		doneChan := make(chan int)
		for i := 0; i < n; i++ {
			goroutine := i
			go func() {
				for j := 0; j < 22; j++ {
					waitChan <- struct {
						goroutine int
						attempt   int
						wait      time.Duration
					}{
						goroutine: goroutine,
						attempt:   j,
						wait:      w.Wait(&request.Execution{Attempt: j}),
					}
				}
				doneChan <- goroutine
			}()
		}
		done := map[int]bool{}
		total := time.Duration(0)
		for len(done) < n {
			select {
			case x := <-doneChan:
				done[x] = true
			case y := <-waitChan:
				var max time.Duration
				if y.attempt < 22 {
					max = (1 << y.attempt) * time.Millisecond
				} else {
					max = time.Hour
				}
				m := fmt.Sprintf("goroutine[%d].attempt[%d]: wait should be between 0 and %d",
					y.goroutine, y.attempt, max)
				total += y.wait
				assert.GreaterOrEqual(t, y.wait, time.Duration(0), m)
				assert.LessOrEqual(t, y.wait, max, m)
			}
		}
		close(waitChan)
		close(doneChan)
		assert.Greater(t, total, time.Duration(0))
	})
}

func newJitterExpWaiter(t *testing.T, base, max time.Duration, jitter interface{}, message string) *jitterExpWaiter {
	j := NewExpWaiter(base, max, jitter)
	assert.IsType(t, &jitterExpWaiter{}, j, message)
	return j.(*jitterExpWaiter)
}
