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

func TestDisabled(t *testing.T) {
	assert.Equal(t, time.Duration(0), Disabled.Schedule(&request.Execution{}))
	assert.False(t, Disabled.Start(&request.Execution{}))
}

func TestNewPolicy(t *testing.T) {
	p := &testPolicy{}
	t.Run("Bad Args", func(t *testing.T) {
		assert.PanicsWithValue(t, "httpx/racing: nil scheduler", func() { NewPolicy(nil, p) })
		assert.PanicsWithValue(t, "httpx/racing: nil starter", func() { NewPolicy(p, nil) })
	})
	t.Run("Normal", func(t *testing.T) {
		P := NewPolicy(p, p)
		assert.Equal(t, time.Second, P.Schedule(&request.Execution{}))
		assert.Equal(t, 1, p.sc)
		assert.True(t, P.Start(&request.Execution{}))
		assert.Equal(t, 1, p.st)
	})
}

type testPolicy struct {
	sc int
	st int
}

func (p *testPolicy) Schedule(_ *request.Execution) time.Duration {
	p.sc++
	return time.Second
}

func (p *testPolicy) Start(_ *request.Execution) bool {
	p.st++
	return true
}
