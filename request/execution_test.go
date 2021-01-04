// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package request

import (
	"errors"
	"net/http"
	"net/url"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecution_StatusCode(t *testing.T) {
	e := &Execution{}
	t.Run("no Response", func(t *testing.T) {
		require.Nil(t, e.Response)
		assert.Equal(t, 0, e.StatusCode())
	})
	t.Run("with Response", func(t *testing.T) {
		e.Response = &http.Response{StatusCode: 999}
		assert.Equal(t, 999, e.StatusCode())
	})
}

func TestExecution_Header(t *testing.T) {
	e := &Execution{}
	t.Run("no Response", func(t *testing.T) {
		require.Nil(t, e.Response)
		assert.Nil(t, e.Header())
		assert.Empty(t, e.Header().Get("foo"))
	})
	t.Run("with Response", func(t *testing.T) {
		h := http.Header{
			"Foo": []string{"bar"},
			"Ham": []string{"eggs", "spam"},
		}
		e.Response = &http.Response{
			Header: h,
		}
		g := e.Header()
		assert.Equal(t, &h, &g)
		assert.Equal(t, h, g)
		assert.Equal(t, []string{"eggs", "spam"}, e.Header()["Ham"])
	})
}

func TestExecution_TimeMethods(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		e := &Execution{}
		assert.False(t, e.Started())
		assert.False(t, e.Ended())
		assert.Equal(t, time.Duration(0), e.Duration())
	})
	t.Run("started but not ended", func(t *testing.T) {
		e := &Execution{}
		e.Start = time.Now()
		assert.True(t, e.Started())
		assert.False(t, e.Ended())
		time.Sleep(2*time.Millisecond + 50*time.Microsecond)
		d := e.Duration()
		assert.LessOrEqual(t, d, time.Now().Sub(e.Start))
		assert.GreaterOrEqual(t, d, 2*time.Millisecond)
	})
	t.Run("ended", func(t *testing.T) {
		e := &Execution{}
		e.Start = time.Now()
		time.Sleep(2*time.Millisecond + 50*time.Microsecond)
		e.End = time.Now()
		d := e.Duration()
		assert.Greater(t, d, 2*time.Millisecond)
		assert.LessOrEqual(t, d, time.Now().Sub(e.Start))
		assert.True(t, e.Ended())
		time.Sleep(2*time.Millisecond + 50*time.Microsecond)
		d2 := e.Duration()
		assert.Equal(t, d, d2)
	})
}

func TestExecution_Timeout(t *testing.T) {
	t.Run("no error", func(t *testing.T) {
		e := &Execution{}
		assert.False(t, e.Timeout())
	})
	t.Run("generic error not timeout", func(t *testing.T) {
		e := &Execution{
			Err: errors.New("foo"),
		}
		assert.False(t, e.Timeout())
	})
	t.Run("direct timeout", func(t *testing.T) {
		e := &Execution{
			Err: syscall.ETIMEDOUT,
		}
		assert.True(t, e.Timeout())
	})
	t.Run("indirect timeout", func(t *testing.T) {
		e := &Execution{
			Err: &url.Error{
				Err: syscall.ETIMEDOUT,
			},
		}
		assert.True(t, e.Timeout())
	})
}

func TestExecution_Value(t *testing.T) {
	t.Run("new Execution", func(t *testing.T) {
		e := &Execution{}
		assert.Nil(t, e.Value("foo"))
		e.SetValue("foo", "bar")
		assert.Equal(t, "bar", e.Value("foo"))
	})
	t.Run("different keys", func(t *testing.T) {
		e := &Execution{}
		assert.Nil(t, e.Value("funky"))
		assert.Nil(t, e.Value(funKey{}))
		assert.Nil(t, e.Value(funkyKey{}))
		e.SetValue("funky", "foo")
		e.SetValue(funKey{}, "bar")
		e.SetValue(funkyKey{}, "baz")
		assert.Equal(t, "foo", e.Value("funky"))
		assert.Equal(t, "bar", e.Value(funKey{}))
		assert.Equal(t, "baz", e.Value(funkyKey{}))
	})
	t.Run("same key multiple times", func(t *testing.T) {
		// People shouldn't put the same key twice into the same Execution,
		// because it results in a proliferation of contexts in the chain.
		// But it should still work, so we test it.
		e := &Execution{}
		assert.Nil(t, e.Value(funKey{}))
		assert.Nil(t, e.Value(funkyKey{}))
		e.SetValue(funKey{}, "ham")
		e.SetValue(funkyKey{}, "eggs")
		assert.Equal(t, "ham", e.Value(funKey{}))
		assert.Equal(t, "eggs", e.Value(funkyKey{}))
		e.SetValue(funKey{}, "spam")
		assert.Equal(t, "spam", e.Value(funKey{}))
		assert.Equal(t, "eggs", e.Value(funkyKey{}))
	})
}

type funKey struct{}

type funkyKey struct{}
