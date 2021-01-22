httpx - Reliable HTTP with retry and more
=========================================

[![Build Status](https://travis-ci.com/gogama/httpx.svg)](https://travis-ci.com/gogama/httpx) [![Go Report Card](https://goreportcard.com/badge/github.com/gogama/httpx)](https://goreportcard.com/report/github.com/gogama/httpx) [![PkgGoDev](https://pkg.go.dev/badge/github.com/gogama/httpx)](https://pkg.go.dev/github.com/gogama/httpx)

Package httpx provides a Go code (GoLang) HTTP client with
enterprise-level reliability and a familiar interface.

Features include:

- Flexible retry policies
- Flexible timeout policies (including adaptive timeouts)
- Fully buffered request and response bodies
- Optionally customizable behavior via event handlers

----

Getting Started
===============

Install httpx:

```sh
$ go get github.com/gogama/httpx
```

Import the httpx package and create a `Client` to begin making reliable
HTTP requests:

```go
package main

import "github.com/gogama/httpx"

func main() {
	client := &httpx.Client{} // Use default retry and timeout policies
	client.Get("http://example.com")
}
```

Check out the full API documentation: https://pkg.go.dev/github.com/gogama/httpx.

---

Usage Guide
===========

## Concepts

### Core types

The core types are `request.Plan` and `request.Execution`.

#### Plan

A [`request.Plan`](https://pkg.go.dev/github.com/gogama/httpx/request#Plan) is a
plan for making a successful HTTP request, if necessary using repeated attempts
(retries). The `Plan` structure looks like a standard `http.Request` structure
but with redundant (server only) fields removed, and with `Body` specified as a
`[]byte` rather than an `io.ReadCloser`. Under the hood, `Client` converts
`Plan` to `http.Request` when making individual request attempts.

Use `request.NewPlan` to make a new HTTP request plan:

```go
// Body may be provided as nil, or a string, []byte, io.Reader or io.ReadCloser.
p, err := request.NewPlan("POST", "http://example.com/upload", "body text")
...
e, err := client.Do(&p)
...
```

Use `request.NewPlanWithContext` if you need to pass in an outside context, for
example if you need to be able to cancel the overall plan execution:

```go
// Body may be provided as nil, or a string, []byte, io.Reader or io.ReadCloser.
p, err := request.NewPlanWithContext(ctx, "PUT", "http://example.com/collections/1", "body text")
...
e, err := client.Do(&p)
...
```

#### Execution

A [`request.Execution`](https://pkg.go.dev/github.com/gogama/httpx/request#Execution)
represents the state of a plan execution. While the plan is being executed,
`Execution` captures the execution state for sharing with retry policies,
timeout policies, and event handlers. After the plan execution is finished,
`Execution` represents the final result of executing the plan.

### Clients

Create an `httpx.Client` to start executing reliable HTTP requests. `Client` has
the familiar methods you are used to from `http.Client`, except that `Client.Do`
consumes a `*request.Plan` instead of an `*http.Request`, and all methods return
a `*request.Execution` instead of an `*http.Response`.

An `httpx.Client` requires an `HTTPDoer` to perform the individual request
attempts within an HTTP request plan execution. An `HTTPDoer` is any object that
implements a `Do(*http.Request) (*http.Response, error)` method with similar
semantics to `http.Client`. Typically, you will use an `http.Client` as the
`HTTPDoer` and, indeed, the zero value `httpx.Client` uses `http.DefaultClient`
for this purpose.

To use a custom-configured `HTTPDoer`:

```go
doer := &http.Client{
	...,    // See package "net/http" for detailed documentation
}
client := &httx.Client{
	HTTPDoer:      doer,                // If omitted/nil, http.DefaultClient is used
	RetryPolicy:   myRetryPolicy(),     // If omitted/nil, retry.DefaultPolicy is used
	TimeoutPolicy: myTimeoutPolicy(),   // If omitted/nil, timeout.DefaultPolicy is used
}
```

## Retry

A [`retry.Policy`](https://pkg.go.dev/github.com/gogama/httpx/retry#Policy) runs
after each request attempt with a plan execution to decide whether a retry should
be done and, if so, how long to wait before retrying (backoff).

The default retry policy, `retry.DefaultPolicy` is sufficient for many common
use cases. Use `retry.Never` as the policy to disable retries altogether.

Construct a custom retry policy using `retry.NewPolicy`:

```go
retryPolicy := retry.NewPolicy(
	retry.Before(20*time.Second).And(retry.TransientErr),
	retry.NewExpWaiter(250*time.Millisecond, 1*time.Second, time.Now() /* jitter seed */))
client := &httpx.Client{
	RetryPolicy: retryPolicy,
}
```

## Timeouts

### Attempt Timeouts

A [`timeout.Policy`](https://pkg.go.dev/github.com/gogama/httpx/timeout#Policy)
controls how client-side timeouts are set on individual HTTP request attempts
within a plan execution.

The default timeout policy, `timeout.DefaultPolicy` is sufficient for many
common use cases. Use `timeout.Infinite` to disable client-side timeouts at the
request attempt level.

To use the same timeout for all attempts (initial and retries), use the
`timeout.Fixed` policy generator:

```go
client := &httpx.Client{
	TimeoutPolicy: timeout.Fixed(2*time.Second),
}
```

More subtle timeout behavior can be achieved with the `timeout.Adaptive` policy
generator, or by implementing your own timeout policy.

### Plan Timeouts

An über-timeout may be set on the entire request plan by setting a deadline or
timeout on the plan context:

```go
ctx := context.WithTimeout(context.Background(), 30*time.Second)
p := request.NewPlanWithContext(ctx, "GET", "https://example.com", nil)
// Do will time out after 30 seconds even if the current attempt has not timed
// out or if the retry policy indicates further retry is possible. 
e, err := client.Do(p) 
```

Since the plan context is the parent context for each attempt within the plan
execution, a plan timeout will also cause a request attempt timeout if it
happens while an attempt's HTTP request is in flight.

## Event Handlers

Optional event handlers allow you to extend `httpx.Client` with custom logic,
for example, adding attempt-specific headers to the request or recording log
messages or metrics about the execution progress, request timings, &c.

Handlers for a specific event type form a chain and are executed in the order
they are added into the chain:

```go
handlers := &httpx.HandlerGroup{}
handlers.PushBack(httpx.BeforeAttempt, httpx.HandlerFunc(hf1))
handlers.PushBack(httpx.BeforeAttempt, httpx.HandlerFunc(hf2))
// When the BeforeAttempt event happens, client will execute hf1 first,
// and then hf2.
client := &httpx.Client{
	Handlers: handlers,     // If omitted/nil, an empty handler group is used
}
```

---

License
=======

This project is licensed under the terms of the MIT License.

---

FAQ
===

## What Go versions are supported?

Package httpx works on Go 1.13 and higher.

## How do I update to the latest httpx version?

```sh
go get -u github.com/gogama/httpx
```

## What alternatives to httpx are out there?

Alternatives include:

- [rehttp](https://github.com/PuerkitoBio/rehttp) - Adds a retrying `http.RoundTripper`
  for `http.Client`, supports configurable retry policies.
- [httpretry](https://github.com/ybbus/httpretry) - Adds a retrying `http.RoundTripper`
  for `http.Client`, supports configurable retry policies.
- [go-retryablehttp](https://github.com/hashicorp/go-retryablehttp) - Adds a retrying
  client, `retryablehttp.Client` with configurable retry policies, a few event hooks,
  and a way to transmorgrify the client into an `http.RoundTripper` so it can be
  wrapped in an `http.Client`.

## Why use httpx over the alternatives?

The alternative libraries tend to be missing three features that are fundamental
to providing top-tier HTTP reliability:

1. **Retry on failed response body read**. Because the response body is an
   integral part of an HTTP response, httpx reads the whole body before doing a
   retry check, so the retry policy has access to the response body and any error
   encountered while reading it. Other frameworks run their retry checks before
   considering the response body.
2. **Set per attempt timeouts**. Cancelling and retrying slow requests can be a
   powerful way to gain incremental reliability, so httpx supports setting
   timeouts on individual request attempts (httpx "attempt timeout"). Other
   frameworks use a single `http.Request` with some form of body-rewinding, and
   can only set a single global timeout on the whole transaction (httpx "plan
   timeout").

As well, httpx has the following differentiating features which add value on top
of its solid fundamental reliability:

1. **Better transience classification**. Package `transient` classifies errors
   based on whether they are likely to be transient from an HTTP perspective,
   limiting false negative and false positive retries when a request attempt
   ends in error. (Lightweight and reusable standalone, too!) Other frameworks
   tend to be more *ad hoc* in their classification.
2. **Event handlers**. httpx provides a comprehensive set of event hooks,
   allowing custom extensions and plugins.
3. **Adaptive timeouts**. httpx supports flexible timeout policies, including
   adaptive timeouts that adapt based on whether previous request attempts
   timed out.

## Why does httpx pre-buffer request bodies and response bodies?

The reasons for pre-buffering request bodies and response bodies are
slightly different.

For *requests*, the issue is that for the retry logic to work, requests need to
be repeatable, which means httpx can't consume a one-time request body stream
like the Go standard HTTP client does. (Of course httpx could consume a function
that returns an `io.Reader`, but this would push more complexity onto the
programmer when the goal is to simplify.)

For *responses*, the issue is that if the response body is important, then you
*have* to be able to read the response *reliably*. Since a transient error like
a client-side timeout or connection reset can happen after the server sends back
the response headers, but before the response body is fully received, the only way
to determine if the response was completed successfully is to read the entire
body without error.

Thus, in order to be able to retry request attempts where the response body was
not fully received, httpx reads and buffers the entire response body. Apart
from allowing more reliable HTTP transactions, this has the additional benefit
of slightly simplifying the programmer's life, since `httpx.Client` returns a
`[]byte` containing the entire response body, and ensures the response body
reader is drained and closed so the underlying HTTP connection can be reused.

## What are event handlers for?

Event handlers let you mix in your own logic at designated plug points in the
HTTP request plan execution logic.

Examples of handlers one might implement:

- an OAuth signer that ensures an up-to-date signature on each retry;
- a logging component that logs request attempts, outcomes, and timings;
- a metering component that sends detailed metrics to Amazon CloudWatch,
  AWS X-Ray, Azure Monitor, or the like;
- a response post-processor that unmarshals the response body so heavyweight
  unmarshaling does not need to be done separately by the retry policy and
  the end consumer.

## Why don't request plans support...?

The `request.Plan` structure is equivalent to the Go standard library
`http.Request` structure with the following fields removed:

- all server-only fields, because httpx is a client-only package;
- the `Cancel` channel, because it is deprecated;
- the `GetBody` function, because it is redundant: the request plan already has
  a fully-buffered body which it can reuse on redirects and retries; and
- the `Trailer` field, because trailers make less sense when the entire request
  body is buffered, and trailer support in servers is in any event uncommon.

## Can I turn off request/response buffering?

Yes. While we don't recommend it, as it won't be useful for most web service
interactions, httpx can be used without either request or response buffering.

- **Request**. To turn off request buffering, set a `nil` request body on the
  request plan, and write a handler for the `httpx.BeforeAttempt` event which
  sets the `Body`, `GetBody`, and `ContentLength` fields on the execution's
  request.
- **Response**. To turn off response buffering, write a handler for the
  `httpx.BeforeReadBody` event which replaces the body reader on the execution's
  response with a no-op reader. In this case, your code is responsible for
  draining and closing the original reader to avoid leaking the connection.
