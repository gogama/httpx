httpx - Usage Guide
===================

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

## Racing (Parallel Requests)

A [`racing.Policy`](https://pkg.go.dev/github.com/gogama/httpx/racing#Policy)
allows multiple HTTP request attempts to be made to satisfy the same request
plan, "racing" the concurrent attempts and using the result from the attempt
that finishes first.

There is no default racing policy. By default racing is disabled.

Construct a custom racing policy using `racing.NewPolicy`:

```go
racingPolicy := racing.NewPolicy(
	racing.NewStaticScheduler(350*time.Millisecond, 900*time.Millisecond),
	racing.AlwaysStart)
client := &httpx.Client{
	RacingPolicy: racingPolicy,
}
```

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
