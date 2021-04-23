// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"testing"
	"time"

	"github.com/gogama/httpx/request"
	"github.com/stretchr/testify/assert"
)

func TestNewStaticSchedulerTest(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		sc := NewStaticScheduler()
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{}))
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{Racing: 1000}))
	})
	t.Run("Offset=Zero", func(t *testing.T) {
		sc := NewStaticScheduler(0)
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{}))
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{Racing: 1000}))
	})
	t.Run("Size=1", func(t *testing.T) {
		sc := NewStaticScheduler(time.Hour)
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{Racing: 0}))
		assert.Equal(t, time.Hour, sc.Schedule(&request.Execution{Racing: 1}))
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{Racing: 2}))
	})
	t.Run("Size=2", func(t *testing.T) {
		sc := NewStaticScheduler(time.Millisecond, time.Second)
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{Racing: 0}))
		assert.Equal(t, time.Millisecond, sc.Schedule(&request.Execution{Racing: 1}))
		assert.Equal(t, time.Second, sc.Schedule(&request.Execution{Racing: 2}))
		assert.Equal(t, time.Duration(0), sc.Schedule(&request.Execution{Racing: 1000}))
	})
}
