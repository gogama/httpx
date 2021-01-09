// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	// RetryPolicy decides when to retry failed attempts and how long
	// to sleep after a failed attempt before retrying.
	//
	// If RetryPolicy is nil, retry.DefaultPolicy is used.
	RetryPolicy retry.Policy
	// TimeoutPolicy specifies how to set timeouts on individual request
	// attempts.
	//
	// If TimeoutPolicy is nil, timeout.DefaultPolicy is used.
	TimeoutPolicy timeout.Policy
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
	e := request.Execution{
		Plan: p,
	}

	doer := c.doer()

	timeoutPolicy := c.TimeoutPolicy
	if timeoutPolicy == nil {
		timeoutPolicy = timeout.DefaultPolicy
	}

	retryPolicy := c.RetryPolicy
	if retryPolicy == nil {
		retryPolicy = retry.DefaultPolicy
	}

	handlers := c.Handlers
	if handlers == nil {
		handlers = &emptyHandlers
	}
	handlers.run(BeforeExecutionStart, &e)
	e.Start = time.Now()

RetryLoop:
	for {
		sendAndReceive(p, &e, doer, handlers, timeoutPolicy)
		if e.Timeout() {
			e.AttemptTimeouts++
			handlers.run(AfterAttemptTimeout, &e)
		}
		handlers.run(AfterAttempt, &e)
		planCtxErr := p.Context().Err()
		if planCtxErr == context.DeadlineExceeded {
			handlers.run(AfterPlanTimeout, &e)
			break
		} else if planCtxErr != nil {
			e.Err = planCtxErr
			break
		} else if retryPolicy.Decide(&e) {
			wait := retryPolicy.Wait(&e)
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
				break
			case <-p.Context().Done():
				err := p.Context().Err()
				e.Err = urlErrorWrap(p, err)
				if err == context.DeadlineExceeded {
					handlers.run(AfterPlanTimeout, &e)
				}
				break RetryLoop
			}
			e.Response = nil
			e.Err = nil
			e.Body = nil
			e.Attempt++
		} else {
			break
		}
	}

	e.End = time.Now()
	handlers.run(AfterExecutionEnd, &e)
	return &e, e.Err
}

func sendAndReceive(p *request.Plan, e *request.Execution, doer HTTPDoer, handlers *HandlerGroup, timeoutPolicy timeout.Policy) {
	ctx, cancel := context.WithTimeout(p.Context(), timeoutPolicy.Timeout(e))
	defer cancel()
	e.Request = p.ToRequest(ctx)
	handlers.run(BeforeAttempt, e)
	var err error
	e.Response, err = doer.Do(e.Request)
	if err != nil {
		e.Err = urlErrorWrap(p, err)
	} else {
		readBody(p, e, handlers)
	}
}

func readBody(p *request.Plan, e *request.Execution, handlers *HandlerGroup) {
	defer func() {
		_ = e.Response.Body.Close()
	}()
	handlers.run(BeforeReadBody, e)
	var err error
	e.Body, err = ioutil.ReadAll(e.Response.Body)
	if err != nil {
		e.Err = urlErrorWrap(p, err)
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

func (c *Client) doer() HTTPDoer {
	if c.HTTPDoer == nil {
		return http.DefaultClient
	}

	return c.HTTPDoer
}

func urlErrorWrap(p *request.Plan, err error) error {
	if _, ok := err.(*url.Error); ok {
		return err
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

// MISSING TEST CASES.
//
// 1. An explicit HTTP/2 test case. (Smoke test.)
