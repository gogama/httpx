// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"fmt"
	"testing"
	"time"

	"github.com/gogama/httpx/request"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestNewThrottleStarter(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		st := newThrottleStarter(t)
		assert.Len(t, st.limits, 0)
		e := &request.Execution{}
		for i := 0; i < 20; i++ {
			e.Attempt = i
			e.Racing = i
			assert.True(t, st.Start(e))
		}
	})
	t.Run("One Limit", func(t *testing.T) {
		t.Parallel()
		st := newThrottleStarter(t, Limit{Period: 100 * time.Millisecond, MaxAttempts: 2})
		assert.True(t, st.Start(&request.Execution{}))
		assert.True(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
		time.Sleep(105 * time.Millisecond)
		assert.True(t, st.Start(&request.Execution{}))
		assert.True(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
	})
	t.Run("Two Limits", func(t *testing.T) {
		t.Parallel()
		st := newThrottleStarter(t,
			Limit{Period: 25 * time.Millisecond, MaxAttempts: 1},
			Limit{Period: 50 * time.Millisecond, MaxAttempts: 2})
		assert.True(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
		time.Sleep(30 * time.Millisecond)
		assert.True(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
		time.Sleep(30 * time.Millisecond)
		assert.True(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
		assert.False(t, st.Start(&request.Execution{}))
	})
}

func TestLimitQueue(t *testing.T) {
	t.Run("Period=0", func(t *testing.T) {
		q := newLimitQueue(0, 1)
		x := time.Time{}
		assert.True(t, q.accept(&x))
		assert.True(t, q.accept(&x))
		assert.True(t, q.accept(&x))
		assert.True(t, q.accept(&x))
		y := x.Add(-time.Second)
		assert.False(t, q.accept(&y))
		assert.True(t, q.accept(&x))
	})
	t.Run("FillUp", func(t *testing.T) {
		for i := 0; i <= 2; i++ {
			t.Run(fmt.Sprintf("Len=%d", i), func(t *testing.T) {
				q := newLimitQueue(1*time.Hour, i)
				x := time.Time{}
				for j := 0; j < i; j++ {
					assert.True(t, q.accept(&x))
				}
				assert.False(t, q.accept(&x))
			})
		}
	})
	t.Run("FillEmptyFill", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			t.Run(fmt.Sprintf("Len=%d", i), func(t *testing.T) {
				q := newLimitQueue(2*time.Second, i)
				assert.Equal(t, 0, q.len)
				x := time.Time{}
				for j := 1; j <= 2; j++ {
					t.Run(fmt.Sprintf("Repeat=%d", j), func(t *testing.T) {
						// Fill up the queue
						for k := 1; k <= i; k++ {
							assert.True(t, q.accept(&x))
							assert.Equal(t, k, q.len)
						}
						assert.False(t, q.accept(&x))
						assert.Equal(t, i, q.len)
						// Advance time to the middle of the queue'sc interval.
						// None of the samples are expired, so adding must fail.
						x = x.Add(time.Second)
						assert.False(t, q.accept(&x))
						assert.Equal(t, i, q.len)
						// Advance time to the end of the queue'sc interval. Now
						// all the samples in the queue are expired, so adding
						// must succeed.
						x = x.Add(time.Second)
						assert.True(t, q.accept(&x))
						assert.Equal(t, 1, q.len)
						// Advance time far enough so the sample we just added
						// is now expired, and all subsequent adds will succeed.
						x = x.Add(2 * time.Second)
					})
				}
			})
		}
	})
}

func newThrottleStarter(t *testing.T, limits ...Limit) *throttleStarter {
	st := NewThrottleStarter(limits...)
	require.IsType(t, &throttleStarter{}, st)
	return st.(*throttleStarter)
}
