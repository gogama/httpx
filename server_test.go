// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gogama/httpx/retry"

	"github.com/gogama/httpx/timeout"

	"github.com/gogama/httpx/request"
)

var httpServer = httptest.NewUnstartedServer(http.HandlerFunc(serverHandler))
var httpsServer = httptest.NewUnstartedServer(http.HandlerFunc(serverHandler))
var http2Server = httptest.NewUnstartedServer(http.HandlerFunc(serverHandler))
var servers = []*httptest.Server{httpServer, httpsServer, http2Server}

func TestMain(m *testing.M) {
	httpServer.Start()
	defer httpServer.Close()
	httpsServer.StartTLS()
	defer httpsServer.Close()
	http2Server.EnableHTTP2 = true
	http2Server.StartTLS()
	defer http2Server.Close()
	waitForServerStart(httpServer)
	waitForServerStart(httpsServer)
	os.Exit(m.Run())
}

func waitForServerStart(server *httptest.Server) {
	cl := &Client{
		HTTPDoer:      server.Client(),
		RetryPolicy:   retry.NewPolicy(retry.Before(10*time.Second).And(retry.TransientErr), retry.DefaultWaiter),
		TimeoutPolicy: timeout.Fixed(2 * time.Second),
	}
	p := (&serverInstruction{StatusCode: 200}).toPlan(context.Background(), "GET", server)
	e, err := cl.Do(p)
	if e.StatusCode() != 200 {
		panic(fmt.Sprintf("Test server startup failed with status %d and error %v",
			e.StatusCode(), err))
	}
}

func serverName(server *httptest.Server) string {
	switch server {
	case httpServer:
		return "http"
	case httpsServer:
		return "https"
	case http2Server:
		return "http2"
	default:
		panic("unknown server")
	}
}

type bodyChunk struct {
	Pause time.Duration
	Data  []byte
}

type serverInstruction struct {
	HeaderPause time.Duration
	StatusCode  int
	Body        []bodyChunk
}

func (i *serverInstruction) zero() bool {
	return i.HeaderPause == time.Duration(0) &&
		i.StatusCode == 0 &&
		i.Body == nil
}

func (i *serverInstruction) toJSON() []byte {
	if i.zero() {
		return nil
	}

	b, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}

	return b
}

func (i *serverInstruction) toPlan(ctx context.Context, method string, server *httptest.Server) *request.Plan {
	p, err := request.NewPlanWithContext(ctx, method, server.URL, i.toJSON())
	if err != nil {
		panic(err)
	}

	return p
}

func (i *serverInstruction) fromJSON(b []byte) error {
	return json.Unmarshal(b, i)
}

func (i *serverInstruction) fromRequest(req *http.Request) error {
	b, err := ioutil.ReadAll(req.Body)
	_ = req.Body.Close()

	if err != nil {
		return err
	}

	return i.fromJSON(b)
}

func serverHandler(w http.ResponseWriter, req *http.Request) {
	// Decode the instructions.
	var i serverInstruction
	err := i.fromRequest(req)
	if err != nil {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, fmt.Sprintf("failed to read request: %s", err.Error()))
		return
	}

	// Sleep for the duration indicated by the pause field. This is done
	// to allow the client to play with timeouts.
	time.Sleep(i.HeaderPause)

	// Get the Flusher, panicking if it's not available.
	f, ok := w.(http.Flusher)
	if !ok {
		panic("w does not implement Flusher")
	}

	// Return the HTTP response stipulated by the client.
	if i.StatusCode == 0 {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, fmt.Sprintf("bad StatusCode in instruction: %v", i))
		return
	}
	w.WriteHeader(i.StatusCode)
	f.Flush()
	for _, chunk := range i.Body {
		time.Sleep(chunk.Pause)
		_, _ = w.Write(chunk.Data)
		f.Flush()
	}
}
