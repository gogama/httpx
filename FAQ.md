httpx - Frequently Asked Questions
==================================

## A. Getting started

### 1. What Go versions are supported?

Package httpx works on Go 1.13 and higher.

### 2. How do I update to the latest httpx version?

```sh
go get -u github.com/gogama/httpx
```

## B. Alternative libraries

### 1. What alternatives to httpx are out there?

Alternatives include:

- [rehttp](https://github.com/PuerkitoBio/rehttp) - Adds a retrying `http.RoundTripper`
  for `http.Client`, supports configurable retry policies.
- [httpretry](https://github.com/ybbus/httpretry) - Adds a retrying `http.RoundTripper`
  for `http.Client`, supports configurable retry policies.
- [go-retryablehttp](https://github.com/hashicorp/go-retryablehttp) - Adds a retrying
  client, `retryablehttp.Client` with configurable retry policies, a few event hooks,
  and a way to transmorgrify the client into an `http.RoundTripper` so it can be
  wrapped in an `http.Client`.

TODO:
- https://github.com/tescherm/go-http-retry
- https://nicedoc.io/gojek/heimdall

#### 2. Why use httpx over the alternatives?

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

## C. Feature FAQs

### 1. Why does httpx pre-buffer request bodies and response bodies?

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

### 2. What are event handlers for?

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

### 3. Why don't request plans support...?

The `request.Plan` structure is equivalent to the Go standard library
`http.Request` structure with the following fields removed:

- all server-only fields, because httpx is a client-only package;
- the `Cancel` channel, because it is deprecated;
- the `GetBody` function, because it is redundant: the request plan already has
  a fully-buffered body which it can reuse on redirects and retries; and
- the `Trailer` field, because trailers make less sense when the entire request
  body is buffered, and trailer support in servers is in any event uncommon.

### 4. Can I turn off request/response buffering?

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
