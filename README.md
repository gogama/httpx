httpx - Reliable HTTP with retry and more
=========================================

[![Build Status](https://travis-ci.com/gogama/httpx.svg)](https://travis-ci.com/gogama/httpx) [![Go Report Card](https://goreportcard.com/badge/github.com/gogama/httpx)](https://goreportcard.com/report/github.com/stretchr/testify) [![PkgGoDev](https://pkg.go.dev/badge/github.com/gogama/httpx)](https://pkg.go.dev/github.com/gogama/httpx)

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
