// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
Package httpx provides a robust HTTP client with retry support and other
advanced features within a simple and familiar interface.

Create a Client to begin making requests.

	client := &httpx.Client{}
	ex, err := client.Get("https://www.example.com")
	...
	ex, err := client.Post("https://www.example.com/upload",
		"application/json", &buf)
	...
	ex, err := client.PostForm("http://example.com/form",
		url.Values{"key": {"Value"}, "id": {"123"}})

For control over how the client sends HTTP requests and receives HTTP
responses, use a custom HTTPDoer. For example, use a GoLang standard
HTTP client:

	doer := &http.Client{
		..., // See package "net/http" for detailed documentation
	}
	client := &httpx.Client{
		HTTPDoer: doer,
	}

For control over the client's retry decisions and timing, create a
custom retry policy using components from package retry:

	retryWaiter := retry.NewExpWaiter(250*time.Millisecond, 5*time.Second(), time.Now())
	retryPolicy := retry.NewPolicy(retry.DefaultDecider, retryWaiter)
	client := httpx.Client{
		RetryPolicy: retryPolicy,
	}

For control over the client's individual attempt timeouts, set a custom
timeout policy using package timeout:

	client := &httpx.Client{
		TimeoutPolicy: timeout.Fixed(10*time.Second)
	}

To hook into the fine-grained details of the client's request execution
logic, install a handler into the appropriate handler chain:

	log := log.New(os.Stdout, "", log.LstdFlags)
	handlers := &httpx.HandlerGroup{}
	handlers.PushBack(httpx.BeforeAttempt, httpx.HandlerFunc(
		func(_ httpx.Event, e *request.Execution) {
			log.Printf("Attempt %d to %s", e.Attempt, e.Request.URL.String())
		})
	)
	client := &httpx.Client{
		HTTPDoer: doer,
		Handlers: handlers,
	}

Package httpx provides basic interfaces for each method of the robust
client (Doer, Getter, Header, Poster, FormPoster, and IdleCloser); a
combined interface that composes all the basic methods (Executor); and
utility functions for working with a Doer (Inflate, Get, Head, Post,
and PostForm).
*/
package httpx
