// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gogama/httpx/racing"
	"github.com/gogama/httpx/request"
	"github.com/gogama/httpx/retry"
	"github.com/gogama/httpx/timeout"
)

// An HTTPDoer implements a Do method in the same manner as the GoLang
// standard library http.Client from the net/http package.
type HTTPDoer interface {
	// Do sends an HTTP request and returns an HTTP response following
	// policy (such as redirects, cookies, auth) configured on the
	// HTTPDoer.
	//
	// The Do method must follow the contract documented on the GoLang
	// standard library http.Client from the net/http package.
	Do(r *http.Request) (*http.Response, error)
}

var emptyHandlers = HandlerGroup{}

// A Client is a robust HTTP client with retry support. Its zero value
// is a valid configuration.
//
// The zero value client uses http.DefaultClient (from net/http) as the
// HTTPDoer, timeout.DefaultPolicy as the timeout policy, retry.DefaultPolicy
// as the retry policy, and an empty handler group (no event handlers/plug-ins).
//
// Client's HTTPDoer typically has an internal state (cached TCP
// connections) so Client instances should be reused instead of created
// as needed. Client is safe for concurrent use by multiple goroutines.
//
// A Client is higher-level than an HTTPDoer. The HTTPDoer is responsible
// for all details of sending the HTTP request and receiving the response,
// while Client builds on top of the HTTPDoer's feature set. For example,
// the HTTPDoer is responsible for redirects, so consult the HTTPDoer's
// documentation to understand how redirects are handled. Typically the
// Go standard HTTP client http.Client) will be used as the HTTPDoer,
// but this is not required.
//
// On top of the HTTP request features provided by the HTTPDoer, Client
// adds the following features:
//
// • Client reads and buffers the entire HTTP response body into a
// []byte (returned as the Execution.Body field);
//
// • Client retries failed request attempts using a customizable retry
// policy;
//
// • Client sets individual request attempt timeouts using a
// customizable timeout policy;
//
// • Client may race multiple concurrent HTTP request attempts against
// one another to improve performance using a customizable racing policy
// (see package racing for more detail);
//
// • Client invokes user-provided handler functions at designated plug-in
// points within the attempt/retry loop, allowing new features to be
// mixed in from outside libraries; and
//
// • Client implements the httpx.Executor interface.
//
// Client's HTTP methods should feel familiar to anyone who has used the
// Go standard HTTP client (http.Client). The methods use the same names,
// and follow the same rough parameter schema, as the Go standard client.
// The main differences are:
//
// • instead of consuming an http.Request, which is only suitable for
// making a one-off request attempt, Client.Do consumes a request.Plan
// which is suitable for making multiple attempts if necessary (the plan
// execution logic converts the plan into http.Request as
// needed); and
//
// • instead of producing an http.Response, all of Client's HTTP methods
// return a request.Execution, which contains some metadata about the
// plan execution as well as a fully-buffered response body.
type Client struct {
	// HTTPDoer specifies the mechanics of sending HTTP requests and
	// receiving responses.
	//
	// If HTTPDoer is nil, http.DefaultClient from the standard net/http
	// package is used.
	HTTPDoer HTTPDoer
	// TimeoutPolicy specifies how to set timeouts on individual request
	// attempts.
	//
	// If TimeoutPolicy is nil, timeout.DefaultPolicy is used.
	TimeoutPolicy timeout.Policy
	// RetryPolicy decides when to retry failed attempts and how long
	// to sleep after a failed attempt before retrying.
	//
	// If RetryPolicy is nil, retry.DefaultPolicy is used.
	RetryPolicy retry.Policy
	// RacingPolicy specifies how to race concurrent requests if this
	// advanced feature is desired.
	//
	// If RacingPolicy is nil, racing.Disabled, which never races
	// concurrent requests, is used.
	RacingPolicy racing.Policy
	// Handlers allows custom handler chains to be invoked when
	// designated events occur during execution of a request plan.
	//
	// If Handlers is nil, no custom handlers will be run.
	Handlers *HandlerGroup
}

// Do executes an HTTP request plan and returns the results, following
// timeout and retry policy set on Client, and low-level policy set on
// the underlying HTTPDoer.
//
// The result returned is the result after the final HTTP request
// attempt made during the plan execution, as determined by the retry
// policy.
//
// An error is returned if, after doing any retries mandated by the
// retry policy, the final attempt resulted in an error. An attempt may
// end in error due to failure to speak HTTP (for example a network
// connectivity problem), or because of policy in the robust client
// (such as timeout), or because of policy on the underlying HTTPDoer
// (for example relating to redirects body). A non-2XX status code in
// the final attempt does not result in an error.
//
// The returned Execution is never nil, but may contain a nil Response
// and will contain a nil Body if an error occurred (if the initial
// HTTP request caused an error, both Response and Body are nil, but if
// the initial HTTP request succeeded and the error occurred while
// reading Body from the request, then Response is non-nil but body
// is nil). If an error was returned, the Err field of the Execution
// always references the same error.
//
// If the returned error is nil, the returned Execution will contain
// both a non-nil Response and a non-nil Body (although Body may have
// zero length).
//
// Any returned error will be of type *url.Error. The url.Error's
// Timeout method, and the Execution's Timeout method, will return
// true if the final request attempt timed out, or if the entire plan
// timed out.
//
// For simple use cases, the Get, Head, Post, and PostForm methods may
// prove easier to use than Do.
func (c *Client) Do(p *request.Plan) (*request.Execution, error) {
	es := c.newExecState(p)
	defer es.cleanup()

	es.handlers.run(BeforeExecutionStart, es.exec)
	es.exec.Start = time.Now()

	for es.wave() && es.wait() {
		es.exec.Wave++
	}

	var planErr *url.Error
	if es.planCancelled() {
		planErr = urlErrorWrap(es.plan(), es.planContext().Err())
	} else if es.planTimedOut() {
		planErr = urlErrorWrap(es.plan(), context.DeadlineExceeded)
	}
	if planErr != nil {
		es.exec.Err = planErr
		if planErr.Timeout() {
			es.handlers.run(AfterPlanTimeout, es.exec)
		}
	}

	es.exec.End = time.Now()
	es.handlers.run(AfterExecutionEnd, es.exec)
	return es.exec, es.exec.Err
}

type execState struct {
	exec *request.Execution

	// Resolved policy references. These values do not change once set.
	httpDoer      HTTPDoer
	timeoutPolicy timeout.Policy
	retryPolicy   retry.Policy
	racingPolicy  racing.Policy
	handlers      *HandlerGroup

	// Variable state.
	baseAttempt  int
	waveAttempts []*attemptState
	signal       chan *attemptState
	timer        *time.Timer
	ding         bool
}

func (es *execState) wave() bool {
	defer es.cleanupWave()
	attempt := es.newAttemptState(0)
	es.waveAttempts = append(es.waveAttempts, attempt)
	es.exec.Racing = 1
	es.handleCheckpoint(attempt)

	es.installAttempt(0, nil, nil, nil, nil)
	es.setTimer(es.racingPolicy.Schedule(es.exec))

	// Flag drain indicates whether to close out the wave, finishing in flight
	// attempts but not starting any new ones. It is set true as soon as any one
	// attempt in the wave finishes, whether the attempt is retryable or not.
	//
	// Flag halt indicates whether to stop the whole execution. It is set true
	// as soon as a non-retryable attempt is detected.
	drain, halt := false, false

	// Loop until all concurrent attempts have stopped.
	for es.exec.Racing > 0 {
		select {
		case attempt = <-es.signal: // Event received from a running attempt
			d, h := es.handleCheckpoint(attempt)
			drain = drain || d
			// If the execution should halt because a negative retry (stop)
			// decision has been returned from the retry policy, then all other
			// running attempts are redundant and must be cancelled.
			if h && !halt {
				halt = true
				for i, as := range es.waveAttempts {
					if i != attempt.index {
						as.cancel(true)
					}
				}
			}
		case <-es.timer.C: // Next concurrent attempt start scheduled
			es.ding = true
			es.installAttempt(len(es.waveAttempts)-1, nil, nil, nil, nil)
			if !drain && es.racingPolicy.Start(es.exec) {
				attempt = es.newAttemptState(len(es.waveAttempts))
				es.waveAttempts = append(es.waveAttempts, attempt)
				es.exec.Racing++
				es.handleCheckpoint(attempt)
				es.installAttempt(attempt.index, nil, nil, nil, nil)
				es.setTimer(es.racingPolicy.Schedule(es.exec))
			}
		}
	}

	// Pick the winning attempt among the concurrent attempts in the wave.
	for i, as := range es.waveAttempts {
		if !as.redundant {
			es.installAttempt(i, as.req, as.resp, as.err, as.body)
			return !halt
		}
	}

	// If we get here, there was no non-redundant attempt found.
	panic("httpx: no usable attempt")
}

func (es *execState) handleCheckpoint(attempt *attemptState) (drain bool, halt bool) {
	es.installAttempt(attempt.index, attempt.req, attempt.resp, attempt.err, attempt.body)
	switch attempt.checkpoint {
	case createdRequest:
		es.handlers.run(BeforeAttempt, es.exec)
		attempt.req = es.exec.Request
		attempt.checkpoint = sendingRequest
		go attempt.sendAndReadBody()
		return
	case sentRequest:
		attempt.checkpoint = sentRequestHandle
		if attempt.err == nil {
			es.handlers.run(BeforeReadBody, es.exec)
			attempt.resp = es.exec.Response
			attempt.checkpoint = readingBody
			attempt.ready <- struct{}{}
			return
		}
		fallthrough
	case readBody:
		attempt.checkpoint = readBodyHandle
		if es.exec.Timeout() {
			es.exec.AttemptTimeouts++
			es.handlers.run(AfterAttemptTimeout, es.exec)
		}
		es.handlers.run(AfterAttempt, es.exec)
		es.exec.AttemptEnds++
		es.exec.Racing--
		attempt.resp = es.exec.Response
		attempt.body = es.exec.Body
		attempt.checkpoint = done
		drain = true
		halt = attempt.redundant || es.planCancelled() || !es.retryPolicy.Decide(es.exec)
		return
	case panicked:
		es.exec.AttemptEnds++
		es.exec.Racing--
		panic(attempt.panicVal)
	default:
		panic("httpx: bad attempt checkpoint")
	}
}

func (es *execState) wait() bool {
	d := es.retryPolicy.Wait(es.exec)
	es.setTimer(d)
	select {
	case <-es.timer.C:
		es.ding = true
		return true // Retry wait period expired
	case <-es.planContext().Done():
		return false // Plan cancelled or timed out
	}
}

func (es *execState) setTimer(d time.Duration) {
	if !es.ding && !es.timer.Stop() {
		<-es.timer.C
	}

	if d == 0 {
		d = 1<<63 - 1
	}
	es.timer.Reset(d)
	es.ding = false
}

func (es *execState) installAttempt(index int, req *http.Request, resp *http.Response, err error, body []byte) {
	e := es.exec
	e.Attempt = es.baseAttempt + index
	e.Request = req
	e.Response = resp
	e.Err = err
	e.Body = body
}

func (es *execState) plan() *request.Plan {
	p := es.exec.Plan
	if p == nil {
		panic("httpx: plan deleted from execution")
	}

	return p
}

func (es *execState) planContext() context.Context {
	return es.plan().Context()
}

func (es *execState) planCancelled() bool {
	err := es.planContext().Err()
	return err != nil
}

func (es *execState) planTimedOut() bool {
	ctx := es.planContext()
	if d, ok := ctx.Deadline(); ok {
		return !time.Now().Before(d)
	}
	return false
}

func (es *execState) cleanupWave() {
	// Recover from any panic that may have been triggered by a misbehaving
	// event handler or client policy.
	r := recover()
	es.baseAttempt += len(es.waveAttempts)
	// Unblock and cancel all outstanding request attempts within the wave.
	for _, as := range es.waveAttempts {
		as.cancel(false)
		switch as.checkpoint {
		case createdRequest, readBodyHandle:
			es.exec.AttemptEnds++
			es.exec.Racing--
		case sentRequestHandle:
			as.ready <- struct{}{}
		}
	}
	// Drain the swamp. By which I mean wait until all request attempts in the
	// wave have completed.
	for es.exec.Racing > 0 {
		attempt := <-es.signal
		switch attempt.checkpoint {
		case sentRequest:
			if attempt.err == nil {
				attempt.ready <- struct{}{}
				continue
			}
			fallthrough
		case readBody, panicked:
			es.exec.AttemptEnds++
			es.exec.Racing--
		default:
			panic("httpx: bad attempt checkpoint")
		}
	}
	// Close any open channels.
	for _, as := range es.waveAttempts {
		close(as.ready)
	}
	// Re-slice the wave attempts back to empty so we can index from
	// zero next time.
	es.waveAttempts = es.waveAttempts[:0]
	// Re-panic, if we started with a panic.
	if r != nil {
		panic(r)
	}
}

func (es *execState) cleanup() {
	close(es.signal)

	if !es.ding && !es.timer.Stop() {
		<-es.timer.C
	}
}

func (es *execState) newAttemptState(index int) *attemptState {
	d := es.timeoutPolicy.Timeout(es.exec)
	ctx, cancel := context.WithTimeout(es.planContext(), d)
	return &attemptState{
		index:      index,
		checkpoint: createdRequest,
		ctx:        ctx,
		cancelFunc: cancel,
		es:         es,
		req:        es.plan().ToRequest(ctx),
		ready:      make(chan struct{}),
	}
}

type attemptCheckpoint int

const (
	createdRequest attemptCheckpoint = iota
	sendingRequest
	sentRequest
	sentRequestHandle
	readingBody
	readBodyClosing
	readBody
	readBodyHandle
	done
	panicked
)

type attemptState struct {
	index      int
	checkpoint attemptCheckpoint
	ctx        context.Context
	cancelFunc context.CancelFunc
	redundant  bool
	es         *execState
	ready      chan struct{}
	req        *http.Request
	resp       *http.Response
	err        error
	body       []byte
	panicVal   interface{}
}

func (as *attemptState) sendAndReadBody() {
	defer as.recoverPanic()

	// Send the request
	var err error
	as.resp, err = as.es.httpDoer.Do(as.req)
	if err != nil {
		as.err = urlErrorWrap(as.es.plan(), as.maybeRedundant(err))
	}
	as.checkpoint = sentRequest
	as.es.signal <- as

	// Read the body.
	if err == nil {
		<-as.ready
		resp := as.resp
		if resp == nil {
			panic("httpx: attempt response was nilled")
		}
		body := resp.Body
		if body == nil {
			panic("httpx: attempt response body was nilled")
		}
		as.body, err = ioutil.ReadAll(body)
		if err != nil {
			as.err = urlErrorWrap(as.es.plan(), as.maybeRedundant(err))
		}
		as.checkpoint = readBodyClosing
		_ = body.Close()
		as.checkpoint = readBody
		as.es.signal <- as
	}
}

func (as *attemptState) recoverPanic() {
	r := recover()
	if r == nil {
		return
	}

	// Communicate the panic.
	defer func() {
		as.panicVal = r
		as.checkpoint = panicked
		as.es.signal <- as
	}()

	// Close the body. If checkpoint is already readBodyClosing, it means
	// the panic likely emanated from calling Close() and there's no
	// point doing it again.
	if as.checkpoint < readBodyClosing {
		resp := as.resp
		if resp != nil {
			body := resp.Body
			if body != nil {
				_ = body.Close()
			}
		}
	}
}

func (as *attemptState) maybeRedundant(err error) error {
	if as.redundant && errors.Is(err, context.Canceled) {
		return racing.Redundant
	}

	return err
}

func (as *attemptState) cancel(redundant bool) {
	cancel := as.cancelFunc
	if cancel != nil {
		as.redundant = redundant
		as.cancelFunc = nil
		cancel()
	}
}

// Get issues a GET to the specified URL, using the same policies
// followed by Do.
//
// To make a request plan with custom headers, use request.NewPlan and
// Client.Do.
func (c *Client) Get(url string) (*request.Execution, error) {
	return Get(c, url)
}

// Head issues a HEAD to the specified URL, using the same policies
// followed by Do.
//
// To make a request plan with custom headers, use request.NewPlan and
// Client.Do.
func (c *Client) Head(url string) (*request.Execution, error) {
	return Head(c, url)
}

// Post issues a POST to the specified URL, using the same policies
// followed by Do.
//
// The body parameter may be nil for an empty body, or may be any of the
// types supported by request.NewPlan, request.BodyBytes, and httpx.Post,
// namely: string; []byte; io.Reader; and io.ReadCloser.
//
// To make a request plan with custom headers, use request.NewPlan and
// Client.Do.
func (c *Client) Post(url, contentType string, body interface{}) (*request.Execution, error) {
	return Post(c, url, contentType, body)
}

// PostForm issues a POST to the specified URL, with data's keys and
// values URL-encoded as the request body.
//
// The Content-Type header is set to application/x-www-form-urlencoded.
// To set other headers, use request.NewPlan and Client.Do.
func (c *Client) PostForm(url string, data url.Values) (*request.Execution, error) {
	return PostForm(c, url, data)
}

// CloseIdleConnections invokes the same method on the client's
// underlying HTTPDoer.
//
// If the HTTPDoer has no CloseIdleConnections method, this method does
// nothing.
//
// If the HTTPDoer does have a CloseIdleConnections method, then the
// effect of this method depends entirely on its implementation in the
// HTTPDoer. For example, the http.Client type forwards the call to its
// Transport, but only if the Transport itself has a CloseIdleConnections
// method (otherwise it does nothing).
func (c *Client) CloseIdleConnections() {
	doer := c.doer()
	if ic, ok := doer.(IdleCloser); ok {
		ic.CloseIdleConnections()
	}
}

func (c *Client) newExecState(p *request.Plan) execState {
	timeoutPolicy := c.TimeoutPolicy
	if timeoutPolicy == nil {
		timeoutPolicy = timeout.DefaultPolicy
	}

	retryPolicy := c.RetryPolicy
	if retryPolicy == nil {
		retryPolicy = retry.DefaultPolicy
	}

	racingPolicy := c.RacingPolicy
	if racingPolicy == nil {
		racingPolicy = racing.Disabled
	}

	handlers := c.Handlers
	if handlers == nil {
		handlers = &emptyHandlers
	}

	return execState{
		exec: &request.Execution{
			Plan: p,
		},

		httpDoer:      c.doer(),
		timeoutPolicy: timeoutPolicy,
		retryPolicy:   retryPolicy,
		racingPolicy:  racingPolicy,
		handlers:      handlers,

		signal: make(chan *attemptState),
		timer:  time.NewTimer(1<<63 - 1),
	}
}

func (c *Client) doer() HTTPDoer {
	if c.HTTPDoer == nil {
		return http.DefaultClient
	}

	return c.HTTPDoer
}

func urlErrorWrap(p *request.Plan, err error) *url.Error {
	if urlError, ok := err.(*url.Error); ok {
		return urlError
	}

	return &url.Error{
		Op:  urlErrorOp(p.Method),
		URL: p.URL.String(),
		Err: err,
	}
}

// urlErrorOp is lifted verbatim from net/http/client.go
func urlErrorOp(method string) string {
	if method == "" {
		return "Get"
	}
	return method[:1] + strings.ToLower(method[1:])
}
