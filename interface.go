// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"net/url"

	"github.com/gogama/httpx/request"
)

// Doer is the interface that wraps the basic Do method.
//
// Do executes an HTTP request plan and returns the final execution
// state (and error, if any). Client implements the Doer interface,
// and any other Doer implementation must behave substantially the same
// as Client.Do.
//
// Any Doer can be converted into an Executor via the Inflate function.
type Doer interface {
	Do(p *request.Plan) (*request.Execution, error)
}

// Getter is the interface that wraps the basic Get method.
//
// Get creates an HTTP request plan to issue a GET to the specified URL,
// executes the plan, and returns the final execution state (and error,
// if any). Client implements the Getter interface, and any other
// Getter implementation must behave substantially the same as Client.Get.
//
// Any Doer can be used to emulate a Getter via the Get function.
type Getter interface {
	Get(url string) (*request.Execution, error)
}

// Header is the interface that wraps the basic Head method.
//
// Head creates an HTTP request plan to issue a HEAD to the specified
// URL, executes the plan, and returns the final execution state (and
// error, if any). Client implements the Header interface, and any other
// Header implementation must behave substantially the same as
// Client.Head.
//
// Any Doer can be used to emulate a Header via the Head function.
type Header interface {
	Head(url string) (*request.Execution, error)
}

// Poster is the interface that wraps the basic Post method.
//
// Post creates an HTTP request plan to issue a POST to the specified
// URL, executes the plan, and returns the final execution state (and
// error, if any). Client implements the Poster interface, and any other
// Poster implementation must behave substantially the same as
// Client.Post.
//
// The body parameter may be nil for an empty body, or may be any of the
// types supported by request.NewPlan, request.BodyBytes, and httpx.Post,
// namely: string; []byte; io.Reader; and io.ReadCloser.
//
// Any Doer can be used to emulate a Poster via the Post function.
type Poster interface {
	Post(url, contentType string, body interface{}) (*request.Execution, error)
}

// FormPoster is the interface that wraps the basic PostForm method.
//
// PostForm creates an HTTP request plan to issue a form POST to the
// specified URL, executes the plan, and returns the final execution
// state (and error, if any). Client implements the FormPoster interface,
// and any other FormPoster implementation must behave substantially the
// same as Client.PostForm.
//
// The request plan body is set to the URL-encoded keys and values from
// data, and the content type is set to application/x-www-form-urlencoded.
//
// Any Doer can be used to emulate a FormPoster via the PostForm
// function.
type FormPoster interface {
	PostForm(url string, data url.Values) (*request.Execution, error)
}

// IdleCloser is the interface that wraps the basic CloseIdleConnections
// method.
//
// If the underlying implementation supports it, CloseIdleConnections
// closes any idle which were previously connected from previous
// requests but are now sitting idle in a "keep-alive" state. It does
// not interrupt any connections currently in use.
//
// If the underlying implementation does not support this ability,
// CloseIdleConnections does nothing.
type IdleCloser interface {
	CloseIdleConnections()
}

// Executor is the interface that groups the basic Do, Get, Head, Post,
// PostForm, and CloseIdleConnections methods.
//
// Any Doer can be converted into an Executor via the Inflate function.
type Executor interface {
	Doer
	Getter
	Header
	Poster
	FormPoster
	IdleCloser
}

// Get uses the specified Doer to issue a GET to the specified URL,
// using the same policies as d.Do.
//
// To make a request plan with custom headers, use request.NewPlan and
// d.Do.
func Get(d Doer, url string) (*request.Execution, error) {
	p, err := request.NewPlan("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return d.Do(p)
}

// Head uses the specified Doer to issue a HEAD to the specified URL,
// using the same policies as d.Do.
//
// To make a request plan with custom headers, use request.NewPlan and
// d.Do.
func Head(d Doer, url string) (*request.Execution, error) {
	p, err := request.NewPlan("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return d.Do(p)
}

// Post uses the specified Doer to issue a POST to the specified URL,
// using the same policies as d.Do.
//
// The body parameter may be nil for an empty body, or may be any of the
// types supported by Client.Post, request.NewPlan, and request.BodyBytes,
// namely: string; []byte; io.Reader; and io.ReadCloser.
//
// To make a request plan with custom headers, use request.NewPlan and
// d.Do.
func Post(d Doer, url, contentType string, body interface{}) (*request.Execution, error) {
	b, err := request.BodyBytes(body)
	if err != nil {
		return nil, err
	}
	p, err := request.NewPlan("POST", url, b)
	if err != nil {
		return nil, err
	}
	p.Header.Set("Content-Type", contentType)
	return d.Do(p)
}

// PostForm uses the specified Doer to issue a POST to the specified URL,
// with data's keys and values URL-encoded as the request body.
//
// The Content-Type header is set to application/x-www-form-urlencoded.
// To set other headers, use request.NewPlan and d.Do.
func PostForm(d Doer, url string, data url.Values) (*request.Execution, error) {
	return Post(d, url, "application/x-www-form-urlencoded", data.Encode())
}

// Inflate converts any non-nil Doer into an Executor. This may be
// helpful for interop across library boundaries, i.e. if code that only
// has access to a Doer needs to call a function that requires an
// Executor.
func Inflate(d Doer) Executor {
	if d == nil {
		panic("httpx: nil doer")
	}

	if e, ok := d.(Executor); ok {
		return e
	}

	return inflated{d}
}

type inflated struct {
	doer Doer
}

func (i inflated) Do(p *request.Plan) (*request.Execution, error) {
	return i.doer.Do(p)
}

func (i inflated) Get(url string) (*request.Execution, error) {
	return Get(i.doer, url)
}

func (i inflated) Head(url string) (*request.Execution, error) {
	return Head(i.doer, url)
}

func (i inflated) Post(url, contentType string, body interface{}) (*request.Execution, error) {
	return Post(i.doer, url, contentType, body)
}

func (i inflated) PostForm(url string, data url.Values) (*request.Execution, error) {
	return PostForm(i.doer, url, data)
}

func (i inflated) CloseIdleConnections() {
	if ic, ok := i.doer.(IdleCloser); ok {
		ic.CloseIdleConnections()
	}
}
