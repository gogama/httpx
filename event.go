// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

// An Event identifies the event type when installing or running a
// Handler. Install event handlers in a Client to extend it with custom
// functionality.
type Event int

const (
	// BeforeExecutionStart identifies the event that occurs before the
	// plan execution starts.
	//
	// When Client fires BeforeExecutionStart, the execution is
	// non-nil but the only field that has been set is the plan.
	BeforeExecutionStart Event = iota
	// BeforeAttempt identifies the event that occurs before each
	// individual HTTP request attempt during the plan execution.
	//
	// When Client fires BeforeAttempt, the execution's request
	// field is set to the HTTP request that WILL BE sent after all
	// BeforeAttempt handlers have finished.
	//
	// BeforeAttempt Handlers may modify the execution's request, or
	// some of its fields, thus changing the HTTP request that will be
	// sent. However, BeforeAttempt handlers should clone request fields
	// which have reference types (URL and Header) before changing them
	// to avoid side effects, as these fields initially reference the
	// same-named fields in the plan.
	BeforeAttempt
	// BeforeReadBody identifies the event that occurs after an HTTP
	// request attempt has resulted in an HTTP response (as opposed to
	// an error) but before the response body is read and buffered.
	//
	// When Client fires BeforeReadyBody, the execution's
	// response field is set to the HTTP response whose body WILL BE
	// read after all BeforeReadBody handlers have finished (however,
	// handlers may modify this field).
	//
	// Note that BeforeReadBody never fires if the HTTP request attempt
	// ended in error, but always fires if an HTTP response is received,
	// regardless of HTTP response status code, and regardless of
	// whether there is a non-empty body in the request.
	BeforeReadBody
	// AfterAttemptTimeout identifies the event that occurs after an
	// HTTP request attempt failed because of a timeout error.
	//
	// When Client fires AfterAttemptTimeout, the execution's
	// error field is set to the timeout error, and its attempt timeout
	// counter has been incremented.
	AfterAttemptTimeout
	// AfterAttempt identifies the event that occurs after an HTTP
	// request attempt is concluded, regardless of whether it concluded
	// successfully or not.
	//
	// When Client fires AfterAttempt, either the execution's
	// response field or its error field OR BOTH may be set to non-nil
	// values, but it will never be the case that both are nil. The
	// response will only be non-nil when the error is also non-nil if
	// there was an error reading the response body.
	//
	// Note that AfterAttempt always fires on every HTTP request attempt,
	// regardless of whether it ended in error, and that it runs before
	// the retry policy is consulted for a retry decision.
	AfterAttempt
	// AfterPlanTimeout identifies the event that occurs after a timeout
	// on the request plan level, not just the request attempt level
	// (i.e. the context deadline on the plan's context is exceeded).
	// A plan timeout can be detected either at the same time as an
	// attempt timeout, or during the retry wait period.
	//
	// When Client fires AfterPlanTimeout, the execution's
	// response and body fields are both nil.
	//
	// Note that AfterPlanTimeout always occurs after AfterAttempt,
	// even if the plan timeout was actually detected at the same time
	// as an attempt timeout.
	AfterPlanTimeout
	// AfterExecutionEnd identifies the event that occurs after the plan
	// execution ends.
	//
	// When Client fires AfterExecutionEnd, the execution is in
	// the same state it was in after the final HTTP request attempt
	// (and last AfterAttempt event) EXCEPT that the end time is set to
	// the time the execution ended.
	AfterExecutionEnd
	// eventSentinel provides the total number of events typed as an
	// Event.
	eventSentinel

	// numEvents provides the total number of events types as an int.
	numEvents = int(eventSentinel)
)

var eventNames = []string{
	"BeforeExecutionStart",
	"BeforeAttempt",
	"BeforeReadBody",
	"AfterAttemptTimeout",
	"AfterAttempt",
	"AfterPlanTimeout",
	"AfterExecutionEnd",
}

// Events returns a slice containing all events which can occur in an
// HTTP request plan execution by Client, in the order in which
// they would occur.
func Events() []Event {
	return []Event{
		BeforeExecutionStart,
		BeforeAttempt,
		BeforeReadBody,
		AfterAttemptTimeout,
		AfterAttempt,
		AfterPlanTimeout,
		AfterExecutionEnd,
	}
}

// Name returns the name of the event.
func (evt Event) Name() string {
	return eventNames[int(evt)]
}

// String returns the name of the event.
func (evt Event) String() string {
	return evt.Name()
}
