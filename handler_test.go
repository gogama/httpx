// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"fmt"
	"testing"

	"github.com/gogama/httpx/request"
	"github.com/stretchr/testify/assert"
)

func TestHandlerGroup(t *testing.T) {
	var evts []string
	var execs []*request.Execution
	h1 := &testHandler{seq: 1, evts: &evts, execs: &execs}
	h2 := &testHandler{seq: 2, evts: &evts, execs: &execs}
	g := &HandlerGroup{}
	t.Run("PushBack", func(t *testing.T) {
		assert.Panics(t, func() { g.PushBack(BeforeExecutionStart, nil) })
		assert.Panics(t, func() { g.PushBack(Event(123), h1) })
		g.PushBack(BeforeExecutionStart, h1)
		g.PushBack(BeforeExecutionStart, h2)
		g.PushBack(AfterAttempt, h1)
	})
	t.Run("run", func(t *testing.T) {
		e1 := &request.Execution{Attempt: 1}
		e2 := &request.Execution{Attempt: 2}
		assert.Empty(t, evts)
		assert.Empty(t, execs)
		g.run(AfterPlanTimeout, e1)
		assert.Empty(t, evts)
		assert.Empty(t, execs)
		g.run(BeforeExecutionStart, e1)
		assert.Equal(t, []string{"1.BeforeExecutionStart", "2.BeforeExecutionStart"}, evts)
		assert.Equal(t, []*request.Execution{e1, e1}, execs)
		evts = evts[:0]
		execs = execs[:0]
		g.run(AfterAttempt, e2)
		assert.Equal(t, []string{"1.AfterAttempt"}, evts)
		assert.Equal(t, []*request.Execution{e2}, execs)
		evts = evts[:0]
		execs = execs[:0]
		g.run(BeforeExecutionStart, e2)
		assert.Equal(t, []string{"1.BeforeExecutionStart", "2.BeforeExecutionStart"}, evts)
		assert.Equal(t, []*request.Execution{e2, e2}, execs)
	})
}

type testHandler struct {
	seq   int
	evts  *[]string
	execs *[]*request.Execution
}

func (h *testHandler) Handle(evt Event, e *request.Execution) {
	*h.evts = append(*h.evts, fmt.Sprintf("%d.%s", h.seq, evt))
	*h.execs = append(*h.execs, e)
}

func TestHandlerFunc(t *testing.T) {
	var _evt Event
	var _e *request.Execution
	var f = func(evt Event, e *request.Execution) {
		_evt = evt
		_e = e
	}
	h := HandlerFunc(f)
	e := &request.Execution{}
	h.Handle(BeforeReadBody, e)

	assert.Equal(t, BeforeReadBody, _evt)
	assert.Same(t, e, _e)
}
