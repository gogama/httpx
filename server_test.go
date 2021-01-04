// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"httpx/request"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"
)

var server = http.Server{
	Addr:    "127.0.0.1:10449",
	Handler: http.HandlerFunc(serverHandler),
}

func TestMain(m *testing.M) {
	go func() {
		_ = server.ListenAndServe()
	}()
	defer func() {
		_ = server.Close()
	}()
	os.Exit(m.Run())
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

func (i *serverInstruction) toPlan(ctx context.Context, method string) *request.Plan {
	p, err := request.NewPlanWithContext(ctx, method, serverURL(), i.toJSON())
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

func serverURL() string {
	return "http://" + server.Addr
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
