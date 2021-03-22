httpx - Frequently Asked Questions
==================================

Contents:

1. [Why use httpx?](#1-why-use-httpx)
2. [Getting started](#2-getting-started)
3. [Retry FAQ](#3-retry-faq)
4. [Timeout FAQ](#4-timeouts-faq)
5. [Racing FAQ (concurrent requests)](#5-racing-faq-concurrent-requests)
6. [Plugins (event handlers)](#6-plugins-event-handlers)
7. [Detailed feature FAQ](#7-detailed-feature-faq)
8. [Alternative HTTP client libraries](#8-alternative-http-client-libraries)

See Also: [Usage Guide](USAGE.md) | [README](README.md) | [Full API Documentation](https://pkg.go.dev/github.com/gogama/httpx)

## 1. Why use httpx?

1. [What is httpx?](#1-what-is-httpx)
2. [Who is the target user?](#2-who-is-the-target-user)
3. [What does httpx do?](#3-what-does-httpx-do)
4. [Is httpx the best option for me?](#4-is-httpx-the-best-option-for-me)

### 1. What is httpx?

Package httpx is a client-side HTTP library providing enterprise level
HTTP transaction reliability.

### 2. Who is the target user?

Package httpx is targeted primarily at GoLang web services and web
servers.

### 3. What does httpx do?

Package httpx allows web applications written in Go to invoke their
dependencies over HTTP in a reliable way, by: retrying failed HTTP
requests (retry); applying flexible time policies (timeout); and
sending concurrent HTTP requests to the same endpoint (racing).

### 4. Is httpx the best option for me?

If you are building a web application in Go then yes, we think so!

Consult the [Alternative HTTP client libraries](#8-alternative-http-client-libraries)
section to how httpx compares with other GoLang HTTP client libraries.

## 2. Getting started

1. [What Go versions are supported?](#1-what-go-versions-are-supported)
2. [How do I update to the latest httpx version?](#2-how-do-i-update-to-the-latest-httpx-version)

### 1. What Go versions are supported?

Package httpx works on Go 1.13 and higher.

### 2. How do I update to the latest httpx version?

```sh
go get -u github.com/gogama/httpx
```

## 3. Retry FAQ

1. [What is the default retry behavior?](#1-what-is-the-default-retry-behavior)
2. [How do I set the retry backoff period?](#2-how-do-i-set-the-retry-backoff-period)
3. [How do I specify when to retry?](#3-how-do-i-specify-when-to-retry)
4. [Can I make my own custom retry policy?](#4-can-i-make-my-own-custom-retry-policy)
5. [How do I turn off retry altogether?](#5-how-do-i-turn-off-retry-altogether)
6. [Can a retry policy modify the request execution?](#6-can-a-retry-policy-modify-the-request-execution)
7. [What goroutine executes the retry policy methods?](#7-what-goroutine-executes-the-retry-policy-methods)

### 1. What is the default retry behavior?

If you use a zero-value `&httpx.Client{}` or set its `RetryPolicy`
member to `nil`, then a sensible default retry policy is applied.

The default policy is called `retry.DefaultPolicy`. Consult the GoDoc
for more detailed information.

### 2. How do I set the retry backoff period?

Provide an instance of the `retry.Waiter` interface to `retry.NewPolicy`.

- For constant backoff, use the built-in waiter constructor
`retry.NewFixedWaiter`.
- For exponential backoff with optional jitter, use the built-in waiter
constructor `retry.NewExpWaiter`.
- For custom behavior, provide your own waiter implementation.

### 3. How do I specify when to retry?

Provide an instance of the `retry.Decider` interface to `retry.NewPolicy`.

You can assemble a custom decider by combining built-in decider functions
using the `And` and `Or` operators, for example:

```go
myDecider := retry.Times(10).And(
	retry.StatusCode(503).Or(
		retry.TransientErr)
	)
```

For custom behavior, provide your own decider implementation.

### 4. Can I make my own custom retry policy?

If your retry decision and backoff computation are uncoupled, write your
own `retry.Decider` or `retry.Waiter` or both and combine them using
`retry.NewPolicy`. If they are coupled, implement the `retry.Policy`
interface.

### 5. How do I turn off retry altogether?

Use the built-in policy `retry.Never`.

### 6. Can a retry policy modify the request execution?

No. The retry policy must treat the execution as immutable, with one
exception: if the retry policy needs to save state, it may use the
execution's `SetValue` method.

### 7. What goroutine executes the retry policy methods?

The retry policy is always called from the goroutine which called the
request execution method (`Do`, `Get`, `Post`, *etc.*) on
`httpx.Client`. This is always true, even when the retry policy is
being used alongside the racing feature.

## 4. Timeouts FAQ

1. [What is the default timeout behavior?](#1-what-is-the-default-timeout-behavior)
2. [How do I set a constant timeout?](#2-how-do-i-set-a-constant-timeout)
3. [What is an adaptive timeout?](#3-what-is-an-adaptive-timeout)
4. [Why would I use an adaptive timeout?](#4-why-would-i-use-an-adaptive-timeout)
5. [How do I set an adaptive timeout?](#5-how-do-i-set-an-adaptive-timeout)
6. [Can I make my own custom timeout policy?](#6-can-i-make-my-own-custom-timeout-policy)
7. [How do I turn off timeouts?](#7-how-do-i-turn-off-timeouts)
8. [Can a timeout policy modify the request execution?](#8-can-a-timeout-policy-modify-the-request-execution)
9. [What goroutine executes the timeout policy methods?](#9-what-goroutine-executes-the-timeout-policy-methods)

### 1. What is the default timeout behavior?

If you use a zero-value `&httpx.Client{}` or set its `TimeoutPolicy`
member to `nil`, then a sensible default timeout policy is applied.

The default policy is called `timeout.DefaultPolicy`. Consult the GoDoc
for more detailed information.

### 2. How do I set a constant timeout?

Use the built-in timeout policy constructor `timeout.Fixed`.

### 3. What is an adaptive timeout?

Instead of being a static or constant value, an adaptive timeout changes
based on whether the previous request attempt timed out.

### 4. Why would I use an adaptive timeout?

An adaptive timeout may be helpful in smoothing over a rough patch of
higher response latencies from a downstream dependency.

By setting a longer timeout value when a previous request attempt timed
out, you can set tight initial timeouts while being confident you won't
brown out the dependency or suffer an availability drop if the
dependency goes through a slow period.

If you are interested in adaptive timeouts, you may also find the
[racing feature](#5-racing-faq-concurrent-requests) applies to your use
case.

### 5. How do I set an adaptive timeout?

Use the built-in timeout policy constructor `timeout.Adaptive`.

### 6. Can I make my own custom timeout policy?

Of course! Just implement the `timeout.Policy` interface.

### 7. How do I turn off timeouts?

Use the built-in timeout policy `timeout.Infinite`.

### 8. Can a timeout policy modify the request execution?

No. The timeout policy must treat the execution as immutable, with one
exception: if the timeout policy needs to save state, it may use the
execution's `SetValue` method.

### 9. What goroutine executes the timeout policy methods?

The timeout policy is always called from the goroutine which called the
request execution method (`Do`, `Get`, `Post`, *etc.*) on
`httpx.Client`. This is always true, even when the timeout policy is
being used alongside the racing feature.

## 5. Racing FAQ (concurrent requests)

1. [What is racing (concurrent requests)?](#1-what-is-racing-concurrent-requests)
2. [Why would I use racing?](#2-why-would-i-use-racing)
3. [Does racing have any associated costs or risks?](#3-does-racing-have-any-associated-costs-or-risks)
4. [What is the default racing behavior?](#4-what-is-the-default-racing-behavior)
5. [What is a wave?](#5-what-is-a-wave)
6. [When an attempt finishes within a wave, what happens to the other in-flight concurrent attempts?](#6-when-an-attempt-finishes-what-happens-to-the-other-in-flight-concurrent-attempts-in-the-same-wave)
7. [How do I start parallel requests at predetermined intervals?](#7-how-do-i-start-parallel-requests-at-predetermined-intervals)
8. [Can I set a circuit breaker to disable racing?](#8-can-i-set-a-circuit-breaker-to-disable-racing)
9. [Can I make my own custom racing policy?](#9-can-i-make-my-own-custom-racing-policy)
10. [Can a racing policy modify the request execution?](#10-can-a-racing-policy-modify-the-request-execution)
11. [How does retry work with the racing feature?](#11-how-does-retry-work-with-the-racing-feature)
12. [How do timeouts work with the racing feature?](#12-how-do-timeouts-work-with-the-racing-feature)
13. [How do event handlers work with the racing feature?](#13-how-do-event-handlers-work-with-the-racing-feature)
14. [What goroutine executes the racing policy methods?](#14-what-goroutine-executes-the-racing-policy-methods)

### 1. What is racing (concurrent requests)?

Racing means making multiple parallel HTTP request attempts to satisfy
one logical HTTP request plan, and using the result from the fastest
attempt (first to complete) as the final HTTP result.

For example, you request `GET /dogs/german-shepherd` from `pets.com`.
You haven't received the response after 200ms, so you send a second
`GET /dogs/german-shepherd` without cancelling the first one. Now the
first and second request attempts are "racing" each other, and the first
to complete will satisfy the logical request.

### 2. Why would I use racing?

The use case for racing is similar to the use case for [adaptive
timeouts](#4-why-would-i-use-an-adaptive-timeout): racing concurrent
requests can help smooth over pockets of high latency from a downstream
web service, enabling you to get a successful response to your customer
more rapidly even when your dependency is experiencing transient
slowness.

### 3. Does racing have any associated costs or risks?

Before using racing, and when configuring your racing policy, you should
consider the following factors:

- **Cost**. A racing policy may result in you sending extra requests to
  the downstream web service. If you pay another business for these
  requests, your bill may increase. If your downstream dependency is
  another service in your own organization, the increased traffic may
  indirectly increase your organization's costs.
- **Brownout**. A carelessly-designed racing policy may result in a
  surge in traffic to your downstream dependency at precisely the moment
  when the dependency is struggling to handle its existing traffic, let
  alone added load.
- **Idempotency**. If the operation you are requesting on the remote
  web service is not idempotent, it may not be a good idea to send
  multiple parallel requests to the service. For non-idempotent
  requests, do the analysis to determine if racing is right for you.

Fortunately a well-designed racing policy will not materially increase
cost or brownout risk (see *e.g.*
[Can I set a circuit breaker to disable racing?](#8-can-i-set-a-circuit-breaker-to-disable-racing))
and can be disabled for non-idempotent requests.

### 4. What is the default racing behavior?

Racing is off by default. If you use the zero-value `httpx.Client`, or
any `httpx.Client` with a `nil` racing policy, all HTTP requests
attempts for a given request execution will be made serially.

### 5. What is a wave?

A wave is a group of request attempts that are racing one another
(overlap in time due to concurrent execution). Since racing is disabled
unless an explicit racing policy is specified, by default every wave
contains only one attempt.

When racing is enabled, concurrent request attempts are grouped in
waves. If all request attempts within the wave finish and are retryable,
the client waits for the pause duration determined by the retry policy
and then begins a new wave.

### 6. When an attempt finishes, what happens to the other in-flight concurrent attempts in the same wave?

As soon as one request attempt finishes, either due to successfully reading the
whole response body or due to error, the wave is closed out: no new parallel
attempts are added in to the wave.

What happens to the other in-flight request attempts within the wave depends on
the retry policy. (See
[How does retry work with the racing feature?](#11-how-does-retry-work-with-the-racing-feature))

### 7. How do I start parallel requests at predetermined intervals?

Use the built-in scheduler constructor `racing.NewStaticScheduler`.

### 8. Can I set a circuit breaker to disable racing?

Yes. Implement the `racing.Starter` interface to allow/deny starting new
parallel requests.

The built-in constructor `racing.NewThrottleStarter` creates a starter
which can throttle new racing attempts if too many racing attempts were
recently started, effectively returning request plan execution to serial
attempt mode until a cooling-off period has elapsed. This built-in
starter may already satisfy your circuit-breaking needs.

### 9. Can I make my own custom racing policy?

Yes. A racing policy is composed of a concurrent attempt scheduler
and a concurrent attempt starter. If your scheduling and start functions
are uncoupled, write your own `racing.Scheduler` or `racing.Starter`
or both and combine them using `racing.NewPolicy`. If the two functions
are coupled, implement the `racing.Policy` interface.

### 10. Can a racing policy modify the request execution?

No. The racing policy must treat the execution as immutable, with one
exception: if the racing policy needs to save state, it may use the
execution's `SetValue` method.

### 11. How does retry work with the racing feature?

Your retry policy works with racing request attempts in the intuitively
correct manner, roughly as if the racing attempts had been executed
serially in the order in which they *ended*.

When a racing request attempt ends, either due to being finished or due to
error, the retry policy's `Decide` method is called for a retry decision.

Just as in the serial attempt case, a positive retry decision means "keep
trying". Since other in-flight concurrent attempts also represent tries,
these in-flight attempts are allowed to finish and tested for retryability
with `Decide`. If all in-flight attempts in the wave have finished with
a positive retry decision, `httpx.Client` waits for the time indicated
by the retry policy's `Wait` method and then starts a new wave.

Again as in the serial case, a negative retry decision means "stop
trying". As soon as one attempt finishes with a negative retry decision,
all other in-flight attempts in the race are cancelled with the special
error value `racing.Redundant` and the attempt which finished with the
negative retry decision represents the final state of the HTTP request
plan execution.

### 12. How do timeouts work with the racing feature?

Your timeout policy works with racing request attempts exactly as it
does in the case of serially executed request attempts. The timeout
policy is called once before each request attempt, to determine the
timeout applicable to that attempt. This is true whether or not the
attempt is racing other concurrent attempts.

### 13. How do event handlers work with the racing feature?

Event handlers work the same way whether racing is enabled or not.

Event handlers are always called from the goroutine which called the
request execution method (`Do`, `Get`, `Post`, *etc.*) on `httpx.Client`.

Event handlers therefore execute serially even when request attempts are
executing concurrently. Events for different attempts racing in the same
wave may be interleaved, and their order is generally undefined, but the
following invariants are true:

- The `BeforeAttempt` handler for attempt `i` always executes before the
  `BeforeAttempt` handler for attempt `i+1`.
- The relative order of events for attempt `i` is always the same:
  `BeforeAttempt`, `BeforeReadBody` (optional), `AfterAttemptTimeout`
  (optional), `AfterAttempt`.

### 14. What goroutine executes the racing policy methods?

The racing policy is always called from the goroutine which called the
request execution method (`Do`, `Get`, `Post`, *etc.*) on
`httpx.Client`.

## 6. Plugins (event handlers)

1. [What are event handlers useful for?](#1-what-are-event-handlers-useful-for)
2. [How do I add an event handler to an `httpx.Client`?](#2-how-do-i-add-an-event-handler-to-an-httpxclient)
3. [What event handlers are available?](#3-what-event-handlers-are-available)
4. [Can an event handler modify the request execution?](#4-can-an-event-handler-modify-the-request-execution)
5. [What are plugins?](#5-what-are-plugins)
6. [Are there any pre-made plugins I can leverage?](#6-are-there-any-pre-made-plugins-i-can-leverage)
7. [What goroutine executes the event handler methods?](#7-what-goroutine-executes-the-event-handler-methods)

### 1. What are event handlers useful for?

Event handlers let you mix in your own logic at designated plug points
in the HTTP request plan execution logic.

Examples of handlers one might implement:

- an OAuth decorator that ensures a non-expired bearer token header on
  each request attempt, including retries;
- a logging component that logs request attempts, outcomes, and timings;
- a metering component that sends detailed metrics to Amazon CloudWatch,
  AWS X-Ray, Azure Monitor, or the like;
- a response post-processor that unmarshals the response body so heavyweight
  unmarshaling does not need to be done separately by the retry policy and
  the end consumer.

### 2. How do I add an event handler to an `httpx.Client`?

Add a handler group to your `httpx.Client`, if it doesn't have one
already, and push your event handler into the handler group.

```go
func main() {
	client := &httpx.Client{
		Handlers: &httpx.HandlerGroup{},
	}
	client.Handlers.PushBack(httpx.BeforeExecutionStart, httpx.HandlerFunc(myHandler))

	e, err := client.Get("https://example.com")
	...
}

func myHandler(evt httpx.Event, e *httpx.Execution) {
	fmt.Println("Hello from an event handler!")
}
```

### 3. What event handlers are available?

- `BeforeExecutionStart` - once per execution, always
- `BeforeAttempt` - once per attempt, always
- `BeforeReadBody` - once per attempt, only if response headers were
  received without error
- `AfterAttemptTimeout` - once per attempt, only if the attempt timed
  out
- `AfterAttempt` - once per attempt, always
- `AfterPlanTimeout` - once per execution, only if the request plan
  context timed out, distinct from an individual attempt timeout
- `AfterExecutionEnd` - once per execution, always

### 4. Can an event handler modify the request execution?

Event handlers may change the execution in the ways listed below, but
must otherwise treat the execution as immutable.

- Any event handler may use `SetValue` to store a value into the
  execution.
- The `BeforeExecutionStart` event handler may replace the execution
  plan with an equivalent plan. The new plan's context must be equal
  to, or a child of, the old plan's context.
- The `BeforeAttempt` event handler may replace the current attempt's
  request with an equivalent request or modify the request fields. The
  new request's context must be equal to, or a child of, the old
  request's context.
- The `BeforeReadBody` event handler may replace the current attempt's
  response with an equivalent response or modify the response fields.
  If the response body reader is altered, it must be replaced by a new,
  unclosed, reader *and* the event handler is responsible for ensuring
  the old body reader is fully read and closed to avoid leaking
  connections.

### 5. What are plugins?

A plugin is a group of one or more event handlers working together to
add a feature to `httpx.Client`.

### 6. Are there any pre-made plugins I can leverage?

Yes. See the [Plugins](README.md#Plugins) section in [README.md](README.md).

### 7. What goroutine executes the event handler methods?

Event handlers are called from the goroutine which called the request
execution method (`Do`, `Get`, `Post`, *etc.*) on `httpx.Client`. This
is always true, even when the timeout policy is being used alongside the
racing feature.

## G. HTTPDoer Configuration

1. [What is the default `HTTPDoer` used by an `httpx.Client`?](#1-what-is-the-default-httpdoer-used-by-an-httpxclient)
2. [I use `http.Client` as my `HTTPDoer`. How do I configure it?](#2-i-use-httpclient-as-my-httpdoer-how-do-i-configure-it)
3. [What client-side timeouts should I use on `http.Client`?](#3-what-client-side-timeouts-should-i-use-on-httpclient)

### 1. What is the default `HTTPDoer` used by an `httpx.Client`?

An `httpx.Client` with a `nil` valued `HTTPDoer` (including the zero value
client) uses `http.DefaultClient` as its `HTTPDoer`.

### 2. I use `http.Client` as my `HTTPDoer`. How do I configure it?

Configure it as you normally would, bearing in mind that it is usually
preferable not to set any timeouts on the underlying `http.Client` (see
below).

### 3. What client-side timeouts should I use on `http.Client`?

If using the Go standard `http.Client` as the `HTTPDoer`, it is preferable not
to set any client-side timeouts on your underlying `http.Client` unless you set
the `httpx.Client` timeout policy to `timeout.Infinite`.

To be slightly more nuanced, you may leverage any timeouts on the underlying
`http.Client`, including on its transport, *provided* you are aware of how they
will play with your timeout and retry policies. For example, it may make sense
to set the dial or TLS handshake timeouts on the transport, but if you do so,
you will likely want to have them set to a lower value than the lowest timeout
your httpx timeout policy can return.

## 7. Detailed feature FAQ

1. [What is a request plan?](#1-what-is-a-request-plan)
2. [What is a request execution?](#2-what-is-a-request-execution)
3. [Why don't request plans support...?](#3-why-dont-request-plans-support)
4. [Why does httpx consume pre-buffered request bodies?](#4-why-does-httpx-consume-pre-buffered-request-bodies)
5. [Why does httpx produce pre-buffered response bodies?](#5-why-does-httpx-produce-pre-buffered-response-bodies)
6. [Can I turn off request body pre-buffering?](#6-can-i-turn-off-request-body-pre-buffering)
7. [Can I turn off response body pre-buffering?](#7-can-i-turn-off-response-body-pre-buffering)
8. [Can I wrap `httpx.Client` with a Go standard `http.Client`?](#8-can-i-wrap-httpxclient-with-a-go-standard-httpclient)

### 1. What is a request plan?

A `request.Plan` is one of the two core data types in httpx (alongside
`request.Execution`). A request plan declares a plan for executing an
HTTP transaction reliably, using multiple attempts (via retry, racing,
or both) if necessary.

A request plan is analogous to Go's `http.Request` and in fact has a
very similar structure. Notable differences include that `request.Plan`
removes server-side fields and fully buffers the request body into a
`[]byte`.

### 2. What is a request execution?

A `request.Execution` is the other core data type in httpx (along with
`request.Plan`). A request execution represents the intermediate state
involved in executing the request plan, and the final state after the
request plan execution has ended.

During the execution of a request plan initiated by a call to one of
the executing methods (`Do`, `Get`, `Post`, *etc.*) from `httpx.Client`,
the `request.Execution` representing execution state is passed to all
policy methods (retry, timeout, racing) and all event handler methods.
This allows policies and event handlers to act according to the detailed
and current execution state...

### 3. Why don't request plans support...?

The `request.Plan` structure is equivalent to the Go standard library
`http.Request` structure with the following fields removed:

- all server-only fields, because httpx is a client-only package;
- the `Cancel` channel, because it is deprecated;
- the `GetBody` function, because it is redundant: the request plan already has
  a fully-buffered body which it can reuse on redirects and retries; and
- the `Trailer` field, because trailers make less sense when the entire request
  body is buffered, and trailer support in servers is in any event uncommon.

### 4. Why does httpx consume pre-buffered request bodies?

Requests are buffered to allow retry and racing logic to work correctly.

For the retry logic to work, requests need to be repeatable, which means httpx
can't consume a one-time request body stream like the Go standard HTTP client
does. (Of course httpx could consume a function that returns an `io.Reader`,
but this would push more complexity onto the programmer when the goal is to
simplify.)

### 5. Why does httpx produce pre-buffered response bodies?

- **Retryable errors can happen while reading the response body**. If
  the retry framework doesn't read the whole response body, it can't
  assist by retrying errors occurring during body read. This is a major
  flaw in other HTTP retry frameworks written in Go since they require
  the user to read the body outside the retry loop.

- **Response body may contain information relevant to a retry
  decision.** For example, Esri's ArcGIS web service always returns an
  HTTP response header containing status HTTP 200 OK. The response body,
  however, may be a JSON error object with its own status code
  indicating retryability. The entire response body must be read to make
  a correct retry decision.

### 6. Can I turn off request body pre-buffering?

To turn off request buffering, set a `nil` request body on the request
plan, and write a handler for the `httpx.BeforeAttempt` event which sets
the `Body`, `GetBody`, and `ContentLength` fields on the execution's
request.

### 7. Can I turn off response body pre-buffering?

To turn off response buffering, write a handler for the
`httpx.BeforeReadBody` event which replaces the response body reader on
the execution's response object it with a no-op reader. Your code is
responsible for draining and closing the original reader to avoid
leaking the connection.

### 8. Can I wrap `httpx.Client` with a Go standard `http.Client`?

You *can* make an `httpx.Client` look like a standard GoLang HTTP
client, if absolutely necessary, but we *do not recommend it* and don't
provide an out-of-the-box implementation. Instead, we recommend building
your code around single-method interfaces like `httpx.Doer`.

The stock technique for converting to an `http.Client` is to wrap the
target (`httpx.Client`) in an implementation of the `http.RoundTripper`
interface and then use your wrapper `RoundTripper` implementation as the
`Transport` for the `http.Client`. The advantage of doing this is it's
relatively simple and allows you to avoid changing code that consumes
`*http.Client`. The disadvantages are that it explicitly violates just
about every promise made in the documented `RoundTripper` interface
contract: "a single HTTP transaction", "should not attempt to interpret
the request", *etc.*

## 8. Alternative HTTP client libraries

The feature matrix below shows how httpx stacks up against other common
HTTP retry libraries for Go:

| | httpx | [heimdall](https://github.com/gojek/heimdall) | [rehttp](https://github.com/PuerkitoBio/rehttp) | [httpretry](https://github.com/ybbus/httpretry) | [go-http-retry](https://github.com/tescherm/go-http-retry) | [go-retryablehttp](https://github.com/hashicorp/go-retryablehttp) |
|-|-|-|-|-|-|-|
| Basic retry | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| Response buffering | ✔ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Flexible retry policies **(1)** | ✔ | ❌ | ✔ | ✔ | ✔ | ✔ |
| Accurate transient error classification **(2)** | ✔ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Simple event handlers | ✔ | ✔ | ❌ | ❌ | ❌ | ✔ |
| Comprehensive event handlers/plugins | ✔ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Flexible and adaptive timeouts | ✔ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Racing concurrent requests | ✔ | ❌ | ❌ | ❌ | ❌ | ❌ |
| License | MIT | Apache 2.0 | BSD 3-Clause | MIT | MIT | MPL-2.0 |
| Dependencies **(3)** | 0 | 10+ | N/A **(4)** | 0 | N/A **(4)** | 2 |

Notes.

1. *Flexible retry policy* means the user can configure both the retry
   decision and the backoff.
2. See [package transient](https://pkg.go.dev/github.com/gogama/httpx/transient)
   and [retry.TransientErr](https://pkg.go.dev/github.com/gogama/httpx/retry#TransientErr).
3. *Dependencies* means non-test dependencies.
4. *N/A* for dependencies means not a Go module.
