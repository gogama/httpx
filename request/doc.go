// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
Package request contains the core types Plan (describes an HTTP request
plan) and Execution (describes a Plan execution). These two types are
fundamental to making reliable HTTP requests.

The first core type is Plan, which represents a HTTP request plan.

A Plan describes how to make a logical HTTP request, potentially
involving repeated HTTP request attempts if retry is necessary after a
failure. For those familiar with the Go standard HTTP library, net/http,
a Plan looks like a stripped-down http.Request structure with all
server-side fields removed, and the body fields replaced with a simple
[]byte, because Plan requires a pre-buffered request body. Plan fields
are named and typed consistently with http.Request wherever possible.

Create a plan to make a reliable HTTP request:

	p, err := request.NewPlan("GET", "https://example.com", nil)
	...
	e, err := client.Do(p)
	...

A plan may be assigned a context to allow timeouts to be set on the
entire plan execution, and to allow the plan execution to be cancelled:

	p, err := request.NewPlanWithContext(ctx, "POST", "https://example.com/upload", body)
	...

If a deadline is set on the plan context, it is separate from the
deadlines set on individual request attempts within the plan execution,
which are dictated by the client's timeout.Policy. The effect is that an
individual request attempt may fail due either to an attempt timeout or
a plan timeout. The former is potentially retryable, the latter is not.

The second core type is Execution, which represents the state of the
execution of an HTTP request plan. Execution is both the output type for
httpx.Client's plan executing methods, and the input type for callbacks
invoked during plan execution: timeout policies, retry policies, and
event handlers. You will typically not allocate Execution instances
yourself, but will instead work with the ones handed out by the client's
request plan execution logic.
*/
package request
