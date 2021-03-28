// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package retry

import (
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/gogama/httpx/request"

	"github.com/stretchr/testify/assert"
)

func TestDefault(t *testing.T) {
	t.Run("Decider", func(t *testing.T) {
		s := []int{429, 502, 503, 504}
		for i := 0; i < DefaultTimes; i++ {
			assert.True(t, DefaultPolicy.Decide(&request.Execution{
				Attempt: i,
				Response: &http.Response{
					StatusCode: s[i%len(s)],
				},
			}))
			assert.True(t, DefaultPolicy.Decide(&request.Execution{
				Attempt: i,
				Err:     syscall.ECONNRESET,
			}))
		}
		assert.False(t, DefaultPolicy.Decide(&request.Execution{
			AttemptEnds: DefaultTimes + 1,
			Err:         syscall.ETIMEDOUT,
		}))
	})
	t.Run("Waiter", func(t *testing.T) {
		m := []int{50, 100, 200, 400, 800, 1000}
		total := time.Duration(0)
		for i, max := range m {
			e := request.Execution{Attempt: i}
			w := DefaultPolicy.Wait(&e)
			total += w
			assert.GreaterOrEqual(t, w, time.Duration(0))
			assert.LessOrEqual(t, w, time.Duration(max)*time.Millisecond)
		}
		assert.Greater(t, total, time.Duration(0))
	})
}

func TestNever(t *testing.T) {
	assert.True(t, Never.Decide(&request.Execution{}))
	assert.False(t, Never.Decide(&request.Execution{AttemptEnds: 1}))
}

func TestNewPolicy(t *testing.T) {
	p := &testPolicy{}
	t.Run("Bad Args", func(t *testing.T) {
		assert.PanicsWithValue(t, "httpx/retry: nil decider", func() { NewPolicy(nil, p) })
		assert.PanicsWithValue(t, "httpx/retry: nil waiter", func() { NewPolicy(p, nil) })
	})
	t.Run("Normal", func(t *testing.T) {
		P := NewPolicy(p, p)
		assert.True(t, P.Decide(&request.Execution{}))
		assert.Equal(t, 1, p.d)
		assert.Equal(t, time.Second, P.Wait(&request.Execution{}))
		assert.Equal(t, 1, p.w)
	})
}

type testPolicy struct {
	d int
	w int
}

func (p *testPolicy) Decide(_ *request.Execution) bool {
	p.d++
	return true
}

func (p *testPolicy) Wait(_ *request.Execution) time.Duration {
	p.w++
	return time.Second
}
