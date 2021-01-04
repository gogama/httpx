// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"httpx/request"
)

// A HandlerGroup is a group of event handler chains which can be
// installed in a Client.
type HandlerGroup struct {
	handlers [][]Handler
}

// PushBack adds an event handler to the back of the event handler chain
// for a specific event type.
func (g *HandlerGroup) PushBack(evt Event, h Handler) {
	if h == nil {
		panic("httpx: nil handler")
	}

	if g.handlers == nil {
		g.handlers = make([][]Handler, numEvents)
	}

	g.handlers[evt] = append(g.handlers[evt], h)
}

func (g *HandlerGroup) run(evt Event, e *request.Execution) {
	i := int(evt)
	if i < len(g.handlers) {
		run(g.handlers[i], evt, e)
	}
}

func run(chain []Handler, evt Event, e *request.Execution) {
	for _, h := range chain {
		h.Handle(evt, e)
	}
}

// A Handler handles the occurrence of an event during a request plan
// execution.
type Handler interface {
	Handle(Event, *request.Execution)
}

// The HandlerFunc type is an adapter to allow the use of ordinary
// functions as event handlers. If f is a function with appropriate
// signature, then HandlerFunc(f)
type HandlerFunc func(Event, *request.Execution)

// Handle calls f(evt, e).
func (f HandlerFunc) Handle(evt Event, e *request.Execution) {
	f(evt, e)
}
