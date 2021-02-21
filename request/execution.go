// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package request

import (
	"context"
	"net/http"
	"time"

	"github.com/gogama/httpx/transient"
)

// An Execution represents the state of a single Plan execution.
//
// When an HTTP request plan execution is requested, an Execution is
// created for it. The Execution is updated as the plan execution
// progresses (for example when the HTTP response becomes available,
// or when a retry is needed) and is ultimately returned as return value
// of the plan execution.
//
// Timeout and retry policies and event handlers may set values on an
// Execution using its SetValue method and read them back using the Value
// method. However, they should treat the structure's exported field
// values as immutable and leave them unmodified, as the execution state
// is vital to the correct functioning of the plan execution logic.
// Limited exceptions to this rule include making reasonable changes to
// the http.Request before it is sent (for example, to support an OAuth
// or AWS signing use case), or to unzip or a request body after it is
// successfully read.
type Execution struct {
	// Plan specifies the HTTP request plan being executed. It is never
	// nil.
	Plan *Plan

	// Start is the start time of the HTTP request plan execution. It
	// is assigned a non-zero value when the plan execution starts, and
	// this value remains constant thereafter.
	Start time.Time

	// End is the end time of the HTTP request plan execution. It
	// contains the zero value until the plan execution is ends, when
	// it is set to the current time.
	End time.Time

	// Attempt is the zero-based number of the current HTTP request
	// attempt during the plan execution. It is set to zero on the
	// initial attempt, one on the first retry, and so on.
	//
	// When the execution is ended, Attempt contains the zero-based
	// number of the last attempt made during the execution. So for
	// example an execution that ends after an initial attempt plus two
	// retries will have an attempt number of 2.
	Attempt int

	// AttemptTimeouts is the count of the number of times an HTTP
	// request attempt timed out during the execution.
	//
	// Plan timeouts (when the plan's own context deadline is exceeded)
	// do not contribute to the attempt timeout counter, but if an
	// attempt timeout and a plan timeout coincide, the attempt timeout
	// counter will be incremented by one due to the attempt timeout.
	AttemptTimeouts int

	// Request specifies the HTTP request to be made in the current
	// attempt, or already made in the last attempt.
	Request *http.Request

	// Response specifies the HTTP response received in the most recent
	// request attempt. It will be nil if the most recent attempt ended
	// in an error, or if a current attempt is underway, or before the
	// execution starts.
	Response *http.Response

	// Err indicates the error received while making the most recent
	// request attempt. It will be nil if the most recent attempt ended
	// without an error, or if a current attempt is underway, or before
	// the execution starts.
	//
	// Whenever Err is non-nil, it has the type *url.Error.
	//
	// While an execution is in-flight, Err may fluctuate between nil
	// and various non-nil error values. Once the execution has Ended,
	// Err will not change and has the same value as the error value
	// returned by the robust client's executing method.
	Err error

	// Body is the complete response body read from the response after
	// the most recent request attempt. It will be nil if the most
	// recent attempt ended in an error, or if a current attempt is
	// underway.
	//
	// Note that it is possible that both Body and Err are non-nil, if
	// a read of the body was partially successful. The Body field
	// of a completed execution should be treated as invalid unless Err
	// is nil.
	Body []byte

	// Wave is zero-based number of the current wave of request
	// attempts.
	//
	// If a racing policy is not installed on the robust HTTP client,
	// then each wave only contains a single attempt, and Wave is always
	// equal to Attempt. If racing is enabled, however, then a new wave
	// is started only when all parallel attempts racing in the previous
	// wave have finished or been cancelled, so Wave is less than or
	// equal to Attempt.
	Wave int

	// Racing is the count of request attempts racing in the current
	// wave. The value is zero before and after a wave, for example
	// during the BeforeExecutionStart and AfterExecutionEnd events,
	// and always at least one during the wave. The count is increased
	// by one whenever a new request attempt is started and reduced by
	// one whenever a request attempt ends.
	//
	// Unless a racing policy has been installed on the robust HTTP
	// client, the wave size is always one, so the value will be one
	// during events relating to request attempts and zero otherwise.
	Racing int

	// Data contains arbitrary user data. The httpx library will not
	// touch this field, and it will typically be nil unless used by
	// event handler writers.
	//
	// Event handlers may interact with this via the Value and SetValue
	// methods.
	data context.Context
}

// StatusCode returns the status code of the HTTP response from the
// most recent request attempt in the execution. If there is no HTTP
// response, 0 is returned.
//
// A zero value due to no HTTP response will be returned if the most
// recent attempt ended in error, or if a current attempt is underway,
// or before the execution starts.
func (e *Execution) StatusCode() int {
	if e.Response == nil {
		return 0
	}

	return e.Response.StatusCode
}

// Header returns the HTTP response headers from the most recent request
// attempt in the execution. If there is no HTTP response, the nil
// header is returned.
//
// A nil header due to no HTTP response will be returned if the most
// recent attempt ended in error, or if a current attempt is underway,
// or before the execution starts.
//
// Note that a nil return value is always safe for read-only operations,
// since http.Header is a map type. There should never be a reason to
// write to the returned value, since it represents the response headers.
func (e *Execution) Header() http.Header {
	if e.Response == nil {
		var nilHeader http.Header
		return nilHeader
	}

	return e.Response.Header
}

// Duration returns the duration of the execution.
//
// If the execution has not yet started, the duration is zero. If the
// execution has Ended, the duration returned is equal to End minus
// Start. Otherwise, it is equal to the current time minus Start. The
// return value is thus monotonically increasing over the life of
// the execution, and becomes static when the execution has ended.
func (e *Execution) Duration() time.Duration {
	if !e.Started() {
		return time.Duration(0)
	} else if !e.Ended() {
		return time.Now().Sub(e.Start)
	}

	return e.End.Sub(e.Start)
}

// Started indicates whether the execution has started.
//
// If the return value is false, the execution has not started yet. If
// the return value is true, then the execution has started, and Start
// is a non-zero time, indicating the execution start time.
func (e *Execution) Started() bool {
	return e.Start != (time.Time{})
}

// Ended indicates whether the execution has ended.
//
// If the return value is false, the execution is still in-flight. If
// the return value is true, then the execution is over, End is a
// non-zero time, and there will be no further changes to the execution.
func (e *Execution) Ended() bool {
	return e.End != (time.Time{})
}

// Timeout indicates whether Err currently contains a non-nil value
// which indicates a timeout. The timeout may have been caused by an
// HTTP request attempt timeout, or by a plan timeout detected after
// the most recent request attempt.
//
// Note that Timeout may return false even if AttemptTimeouts > 0 (if
// the most recent attempt did not end in a timeout, or a current
// attempt is underway); and it may return true even if AttemptTimeouts
// is zero (if a plan timeout is detected after the end of the most
// recent request attempt).
func (e *Execution) Timeout() bool {
	cat := transient.Categorize(e.Err)
	return cat == transient.Timeout
}

// SetValue allows event handlers to store arbitrary data in the request
// plan execution.
//
// The key must follow the same rules as the key parameter in
// context.WithValue, namely it:
//
// • it may not be nil;
//
// • it must be comparable;
//
// • it should not be of type string or any other built-in type to avoid
// collisions between different event handlers putting data into the
// same request execution.
func (e *Execution) SetValue(key, value interface{}) {
	ctx := e.data
	if ctx == nil {
		ctx = context.Background()
	}

	e.data = context.WithValue(ctx, key, value)
}

// Value returns the data value associated with this execution for key,
// or nil if there is no value associated with key.
func (e *Execution) Value(key interface{}) interface{} {
	ctx := e.data
	if ctx == nil {
		return nil
	}

	return ctx.Value(key)
}
