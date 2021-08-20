httpx - Best HTTP client for reliability (retry, plugins, racing, and more!) 
============================================================================

[![Build Status](https://travis-ci.org/gogama/httpx.svg)](https://travis-ci.com/gogama/httpx) [![Go Report Card](https://goreportcard.com/badge/github.com/gogama/httpx)](https://goreportcard.com/report/github.com/gogama/httpx) [![PkgGoDev](https://pkg.go.dev/badge/github.com/gogama/httpx)](https://pkg.go.dev/github.com/gogama/httpx)

Package httpx is the best-in-class GoLang HTTP client for making HTTP
requests with enterprise-level reliability.

Features include:

- Flexible retry policies
- Flexible timeout policies (including adaptive timeouts)
- Optional concurrent request support smooths over rough service patches ("racing")
- Plugin and customization support via event handlers

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

---

Quick Hits
==========

## Retry

The `httpx.Client` provides a reasonable default retry policy. To replace it
with your own custom policy, use package retry, for example

```go
client := &httpx.Client{
	RetryPolicy: retry.NewPolicy(
		retry.Times(10).And(
			retry.StatusCode(501, 502, 504).Or(retry.TransientErr)
        ),
        retry.NewExpWaiter(500*time.Millisecond, 30*time.Second, nil)
    )
}
```

For more elaborate policies, write your own `retry.Decider` or `retry.Waiter`
implementation. To disable retry altogether, use the built-in policy `retry.Never`:

## Timeouts

The `httpx.Client` provides a reasonable default timeout policy. To replace it
with your own constant timeout, adaptive timeout, or custom policy, use package
timeout, for example:

```go
client := &httpx.Client{
	TimeoutPolicy: timeout.Fixed(30*time.Second) // Constant 30 second timeout
}
```

For more elaborate timeouts, use `timeout.Adaptive` or write your own
`timeout.Policy` implementation. To disable timeouts altogether, use the built-in
policy `timeout.Infinite`:

## Concurrent requests ("racing")

The `httpx.Client` advanced racing feature is disabled by default. Enable it by
specifying a racing policy using built in components from package racing, or by
writing your own `racing.Scheduler` and `racing.Starter` implementations. Here
is a simple example using the built-ins:

```go
client := &httpx.Client{
	// Use up to two extra parallel request attempts. Start the first extra attempt
	// if the response to the initial attempt is not received within 300ms. Start
	// the second extra attempt if neither the initial attempt nor the first extra
	// attempt have received a response after one second. 
	RacingPolicy: racing.NewPolicy( 
		racing.NewStaticScheduler(300*time.Millisecond, 1*time.Second),
		racing.AlwaysStart)
}
```

---

More Info
=========

See the [USAGE.md](USAGE.md) for a more detailed usage guide and
[FAQ.md](FAQ.md) for answers to frequently asked questions, or
[click here](https://pkg.go.dev/github.com/gogama/httpx) for the full
httpx API reference documentation.

---

Plugins
=======

Customize the behavior of `httpx.Client` by adding event handlers to the
client's handler group.

```go
handlers := &httpx.HandlerGroup{}
handlers.PushBack(httpx.BeforeReadBody, myReadBodyHandler)
client := &httpx.Client{Handlers: handlers}
```

Besides writing your own, you can add install one of the following open source
httpx plugins into your client:

1. [aws-xray-httpx](https://github.com/gogama/aws-xray-httpx) - Adds AWS X-Ray
   tracing support into `httpx.Client`.
2. [reconnx](https://github.com/gogama/reconnx) - Discards slow HTTP connections
   from connection pool.

---

License
=======

This project is licensed under the terms of the MIT License.

Acknowledgements
================

Developer happiness on this project was boosted by JetBrains' generous donation
of an [open source license](https://www.jetbrains.com/opensource/) for their
lovely GoLand IDE. ❤
