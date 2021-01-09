// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package request

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	urlpkg "net/url"
	"strings"
)

var (
	template, _ = http.NewRequest("GET", "", nil)
)

const (
	nilCtxMsg = "httpx/request: nil context"
)

// A Plan contains a logical HTTP request plan for execution by a
// client.
//
// The logical request described by a Plan will typically result in a
// lower-level http.Request (net/http) attempts being made, but may
// result in multiple request attempts, for example if the a failed
// attempt needs to be retried.
//
// The field structure of plan mirrors the structure of the lower-level
// http.Request with the following differences. Server-only fields are
// removed (for example Proto). Some fields are either simplified (Body)
// or removed (Trailer) because their definition in the net/http Request
// is highly general to support stream-oriented features which are
// deliberately not supported by this transaction-oriented library.
//
// Like the http.Request structure, a Plan has a context which controls
// the overall plan execution and can be used to cancel the inflight
// execution of a Plan at any time.
type Plan struct {
	// Method specifies the HTTP method (GET, POST, PUT, etc.).
	// An empty string means GET.
	Method string

	// URL specifies the URL to access.
	//
	// The URL's Host specifies the server to connect to, while
	// the Request's Host field optionally specifies the Host
	// header value to send in the HTTP request.
	URL *urlpkg.URL

	// Header contains the request header fields to be sent by the
	// client.
	//
	// For further details, see the documentation of Request.Header in
	// the net/http package.
	Header http.Header

	// Body is the pre-buffered request body to be sent. A nil or
	// empty body indicates no request body should be sent, for example
	// on a GET or DELETE request.
	Body []byte

	// TransferEncoding lists the transfer encodings from outermost to
	// innermost. An empty list denotes the "identity" encoding.
	// TransferEncoding can usually be ignored if using the Go standard
	// http.Client (net/http) for as the lower-level HTTPDoer; http.Client
	// automatically adds and removes chunked encoding as necessary when
	// sending requests.
	TransferEncoding []string

	// Close stipulates whether to close the connection after sending
	// each lower-level (net/http) Request and reading the response.
	// Setting this field prevents re-use of TCP connections between
	// request attempts to the same host (including two request attempts
	// coming from the same plan) as if Transport.DisableKeepAlives were
	// set.
	Close bool

	// Host optionally overrides the Host header to send. If empty, the
	// value of URL.Host will be sent. Host may contain an international
	// domain name.
	Host string

	// ctx allows the entire Plan exec to be cancelled. It should only
	// be modified by copying the whole Plan using WithContext.
	ctx context.Context
}

// NewPlan wraps NewPlanWithContext using the background context.
//
// Parameter body may be nil (empty body), or it may be a string,
// []byte, io.Reader, or io.ReadCloser. If body is an io.Reader, it is
// read to the end and buffered into a []byte. If body is an
// io.ReadCloser, it is closed after buffering.
func NewPlan(method, url string, body interface{}) (*Plan, error) {
	return NewPlanWithContext(context.Background(), method, url, body)
}

// NewPlanWithContext returns a new Plan given a method, URL, and
// optional body.
//
// Parameter body may be nil (empty body), or it may be a string,
// []byte, io.Reader, or io.ReadCloser. If body is an io.Reader, it is
// read to the end and buffered into a []byte. If body is an
// io.ReadCloser, it is closed after buffering.
func NewPlanWithContext(ctx context.Context, method, url string, body interface{}) (*Plan, error) {
	if ctx == nil {
		return nil, errors.New(nilCtxMsg)
	}
	if method == "" {
		method = "GET"
	}
	if !validMethod(method) {
		return nil, fmt.Errorf("httpx/request: invalid method %q", method)
	}
	u, err := urlpkg.Parse(url)
	if err != nil {
		return nil, err
	}
	u.Host = removeEmptyPort(u.Host)
	b, err := BodyBytes(body)
	if err != nil {
		return nil, err
	}
	return &Plan{
		ctx:    ctx,
		Method: method,
		URL:    u,
		Header: make(http.Header),
		Body:   b,
		Host:   u.Host,
	}, nil
}

// Context returns the request plan's context. The context controls
// cancellation of the overall request plan. To change the context, use
// WithContext.
//
// The returned context is always non-nil; it defaults to the
// background context.
func (p *Plan) Context() context.Context {
	if p.ctx != nil {
		return p.ctx
	}
	return context.Background()
}

// WithContext returns a shallow copy of p with its context changed to
// ctx, which must be non-nil.
//
// The context controls the entire lifetime of a logical request plan
// and its execution, including: making individual request attempts
// (obtaining a connection, sending the request, reading the response
// headers and body), running event handlers, and waiting for a retry
// wait period to expire.
//
// To create a new request plan with a context, use NewPlanWithContext.
func (p *Plan) WithContext(ctx context.Context) *Plan {
	if ctx == nil {
		panic(nilCtxMsg)
	}
	p2 := new(Plan)
	*p2 = *p
	p2.ctx = ctx
	return p2
}

// AddCookie adds a cookie to the request. Per RFC 6265 section 5.4,
// AddCookie does not attach more than one Cookie header field. That
// means all cookies, if any, are written into the same line,
// separated by semicolons.
//
// AddCookie only sanitizes c's name and value, and does not sanitize
// a Cookie header already present in the request.
func (p *Plan) AddCookie(c *http.Cookie) {
	c2 := &http.Cookie{Name: c.Name, Value: c.Value}
	s := c2.String()
	if h := p.Header.Get("Cookie"); h != "" {
		p.Header.Set("Cookie", h+"; "+s)
	} else {
		p.Header.Set("Cookie", s)
	}
}

// SetBasicAuth sets the request plan's Authorization header to use HTTP
// Basic Authentication with the provided username and password.
//
// With HTTP Basic Authentication the provided username and password
// are not encrypted.
//
// Some protocols may impose additional requirements on pre-escaping the
// username and password. For instance, when used with OAuth2, both arguments
// must be URL encoded first with url.QueryEscape.
func (p *Plan) SetBasicAuth(username, password string) {
	p.Header.Set("Authorization", "Basic "+basicAuth(username, password))
}

// ToRequest creates an HTTP request corresponding to the given request
// plan. The context of the new request is set to ctx, which may not be
// nil.
func (p *Plan) ToRequest(ctx context.Context) *http.Request {
	r := template.WithContext(ctx)
	r.Method = p.Method
	r.URL = p.URL
	r.Header = p.Header
	if len(p.Body) > 0 {
		r.Body = ioutil.NopCloser(bytes.NewReader(p.Body))
		r.GetBody = func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(p.Body)), nil
		}
		r.ContentLength = int64(len(p.Body))
	}
	r.TransferEncoding = p.TransferEncoding
	r.Close = p.Close
	r.Host = p.Host
	return r
}

// basicAuth is lifted verbatim from net/http/client.go.
//
// See 2 (end of page 4) https://www.ietf.org/rfc/rfc2617.txt
// "To receive authorization, the client sends the userid and password,
// separated by a single colon (":") character, within a base64
// encoded string in the credentials."
// It is not meant to be urlencoded.
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func validMethod(method string) bool {
	/*
	     Method         = "OPTIONS"                ; Section 9.2
	                    | "GET"                    ; Section 9.3
	                    | "HEAD"                   ; Section 9.4
	                    | "POST"                   ; Section 9.5
	                    | "PUT"                    ; Section 9.6
	                    | "DELETE"                 ; Section 9.7
	                    | "TRACE"                  ; Section 9.8
	                    | "CONNECT"                ; Section 9.9
	                    | extension-method
	   extension-method = token
	     token          = 1*<any CHAR except CTLs or separators>

	   We don't need to check for length more than 1 because we always
	   interpret the empty string as "GET".
	*/
	return strings.IndexFunc(method, isNotToken) == -1
}

func isNotToken(r rune) bool {
	return !isTokenRune(r)
}

// isTokenRune is lifted verbatim from x/net/http/httpguts/httplex.go
// (but converted to non-exported). It classifies a rune as being valid
// for a token as defined in https://tools.ietf.org/html/rfc7230#section-3.2.6
func isTokenRune(r rune) bool {
	i := int(r)
	return i < len(isTokenTable) && isTokenTable[i]
}

var isTokenTable = [127]bool{
	'!':  true,
	'#':  true,
	'$':  true,
	'%':  true,
	'&':  true,
	'\'': true,
	'*':  true,
	'+':  true,
	'-':  true,
	'.':  true,
	'0':  true,
	'1':  true,
	'2':  true,
	'3':  true,
	'4':  true,
	'5':  true,
	'6':  true,
	'7':  true,
	'8':  true,
	'9':  true,
	'A':  true,
	'B':  true,
	'C':  true,
	'D':  true,
	'E':  true,
	'F':  true,
	'G':  true,
	'H':  true,
	'I':  true,
	'J':  true,
	'K':  true,
	'L':  true,
	'M':  true,
	'N':  true,
	'O':  true,
	'P':  true,
	'Q':  true,
	'R':  true,
	'S':  true,
	'T':  true,
	'U':  true,
	'W':  true,
	'V':  true,
	'X':  true,
	'Y':  true,
	'Z':  true,
	'^':  true,
	'_':  true,
	'`':  true,
	'a':  true,
	'b':  true,
	'c':  true,
	'd':  true,
	'e':  true,
	'f':  true,
	'g':  true,
	'h':  true,
	'i':  true,
	'j':  true,
	'k':  true,
	'l':  true,
	'm':  true,
	'n':  true,
	'o':  true,
	'p':  true,
	'q':  true,
	'r':  true,
	's':  true,
	't':  true,
	'u':  true,
	'v':  true,
	'w':  true,
	'x':  true,
	'y':  true,
	'z':  true,
	'|':  true,
	'~':  true,
}

// hasPort is lifted verbatim from net/http/http.go
//
// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }

// removeEmptyPort is lifted verbatim from net/http/http.go
//
// removeEmptyPort strips the empty port in ":port" to ""
// as mandated by RFC 3986 Section 6.2.3.
func removeEmptyPort(host string) string {
	if hasPort(host) {
		return strings.TrimSuffix(host, ":")
	}
	return host
}
