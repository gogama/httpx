// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package request

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewPlan(t *testing.T) {
	for _, testCase := range newPlanTestCases {
		t.Run(testCase.name, func(t *testing.T) {
			p, err := NewPlan(testCase.method, testCase.url, resolveBody(t, testCase.body))
			testCase.asserts(t, p, err)
			if p != nil {
				assert.Same(t, context.Background(), p.ctx)
				assert.Same(t, context.Background(), p.Context())
			}
		})
	}
}

func TestNewPlanWithContext(t *testing.T) {
	for _, testCase := range newPlanTestCases {
		t.Run(testCase.name+" with context.Background()", func(t *testing.T) {
			p, err := NewPlanWithContext(context.Background(), testCase.method, testCase.url, resolveBody(t, testCase.body))
			testCase.asserts(t, p, err)
			if p != nil {
				assert.Same(t, context.Background(), p.ctx)
				assert.Same(t, context.Background(), p.Context())
			}
		})
		type foo struct{}
		ctx := context.WithValue(context.Background(), foo{}, "bar")
		require.NotSame(t, ctx, context.Background())
		t.Run(testCase.name+" with special context", func(t *testing.T) {
			p, err := NewPlanWithContext(ctx, testCase.method, testCase.url, resolveBody(t, testCase.body))
			testCase.asserts(t, p, err)
			if p != nil {
				assert.Same(t, ctx, p.ctx)
				assert.Same(t, ctx, p.Context())
			}
		})
		t.Run(testCase.name+" with nil context", func(t *testing.T) {
			p, err := NewPlanWithContext(nil, testCase.method, testCase.name, resolveBody(t, testCase.body))
			assert.Nil(t, p)
			assert.EqualError(t, err, nilCtxMsg)
		})
	}
}

var newPlanTestCases = []struct {
	name    string
	method  string
	url     string
	body    interface{}
	asserts func(*testing.T, *Plan, error)
}{
	{
		name:   "empty method means GET",
		method: "",
		url:    "https://foo.com",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, "GET", p.Method)
			assert.Equal(t, "https://foo.com", p.URL.String())
			assert.Nil(t, p.Body)
		},
	},
	{
		name:   "POST method",
		method: "POST",
		url:    "https://bar.com",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, "POST", p.Method)
			assert.Equal(t, "https://bar.com", p.URL.String())
			assert.Nil(t, p.Body)
		},
	},
	{
		name:   "fake valid extension method",
		method: "Fake",
		url:    "http://baz.com",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, "Fake", p.Method)
			assert.Equal(t, "http://baz.com", p.URL.String())
			assert.Nil(t, p.Body)
		},
	},
	{
		name:   "remove empty port",
		method: "GET",
		url:    "http://ham:",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, p.Host, "ham")
			assert.Equal(t, p.URL.Host, "ham")
			u, err := url.Parse("http://ham:")
			assert.NoError(t, err)
			assert.Equal(t, "ham:", u.Host,
				`If this assertion fails, you may be able to delete this
								 whole test case AND the removeEmptyPort function as it
								 probably indicates the URL parse is now stripping the
								 colon.`)
		},
	},
	{
		name: "body type string",
		body: "str",
		url:  "str",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, []byte("str"), p.Body)
		},
	},
	{
		name: "body type []byte",
		body: []byte{0x1, 0x2, 0x3},
		url:  "byte-slice",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, []byte{0x1, 0x2, 0x3}, p.Body)
		},
	},
	{
		name: "body type io.Reader",
		body: func(_ *testing.T) interface{} {
			return strings.NewReader("io.Reader")
		},
		url: "io.Reader",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, []byte("io.Reader"), p.Body)
		},
	},
	{
		name: "body type io.ReadCloser",
		body: func(_ *testing.T) interface{} {
			return ioutil.NopCloser(strings.NewReader("io.ReadCloser"))
		},
		url: "io.ReadCloser",
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.NoError(t, err)
			require.NotNil(t, p)
			assert.Equal(t, []byte("io.ReadCloser"), p.Body)
		},
	},
	{
		name:   "error invalid method",
		method: "\tGET",
		url:    "eggs",
		body:   strings.NewReader("spam"),
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.Nil(t, p)
			assert.EqualError(t, err, `httpx/request: invalid method "\tGET"`)
		},
	},
	{
		name:   "error invalid URL",
		method: "GET",
		url:    ":::",
		body:   nil,
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.Nil(t, p)
			assert.Error(t, err)
		},
	},
	{
		name:   "error invalid body type",
		method: "POST",
		url:    "spam",
		body:   map[string]int{},
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.Nil(t, p)
			assert.EqualError(t, err, badBodyTypeMsg)
		},
	},
	{
		name:   "error body read",
		method: "PUT",
		url:    "hello",
		body: func(t *testing.T) interface{} {
			m := &mockReadCloser{}
			m.Test(t)
			m.On("Read", mock.AnythingOfType("[]uint8")).
				Return(5, errors.New("problematic")).
				Once()
			return m
		},
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.Nil(t, p)
			assert.EqualError(t, err, "problematic")
		},
	},
	{
		name:   "error body close",
		method: "HEAD",
		url:    "hello",
		body: func(t *testing.T) interface{} {
			m := &mockReadCloser{}
			m.Test(t)
			m.On("Read", mock.AnythingOfType("[]uint8")).
				Return(0, io.EOF).
				Once()
			m.On("Close").
				Return(errors.New("difficult conversation")).
				Once()
			return m
		},
		asserts: func(t *testing.T, p *Plan, err error) {
			assert.Nil(t, p)
			assert.EqualError(t, err, "difficult conversation")
		},
	},
}

func resolveBody(t *testing.T, body interface{}) interface{} {
	if f, ok := body.(func(*testing.T) interface{}); ok {
		body = f(t)
	}
	return body
}

func TestPlan_AddCookie(t *testing.T) {
	// Create a Plan for testing, and an http.Request to use as a shadow
	// test. We assert that the cookies on the Plan and the ones on the
	// http.Request should look the same.
	p, err := NewPlan("", "cookietown", nil)
	require.NoError(t, err)
	require.NotNil(t, p)
	r, err := http.NewRequest("", "cookietown", nil)
	require.NoError(t, err)
	require.NotNil(t, r)
	// Test logic is below this comment.
	var c http.Cookie
	t.Run("simple cookie", func(t *testing.T) {
		c = http.Cookie{Name: "foo", Value: "bar"}
		p.AddCookie(&c)
		r.AddCookie(&c)
		assert.Equal(t, p.Header.Get("Cookie"), "foo=bar")
		assert.Equal(t, r.Header["Cookie"], p.Header["Cookie"])
		c = http.Cookie{Name: "foo", Value: "baz"}
		p.AddCookie(&c)
		r.AddCookie(&c)
		assert.Equal(t, p.Header.Get("Cookie"), "foo=bar; foo=baz")
		assert.Equal(t, r.Header["Cookie"], p.Header["Cookie"])
	})
	t.Run("cookie with extraneous fields", func(t *testing.T) {
		c := http.Cookie{
			Name:    "ham",
			Value:   "eggs",
			Path:    "a/b/c",
			Domain:  "seuss.py",
			MaxAge:  10,
			Secure:  true,
			Expires: time.Now().Add(time.Hour),
		}
		p.AddCookie(&c)
		r.AddCookie(&c)
		assert.Equal(t, p.Header.Get("Cookie"), "foo=bar; foo=baz; ham=eggs")
		assert.Equal(t, r.Header["Cookie"], p.Header["Cookie"])
	})
}

func TestPlan_Context(t *testing.T) {
	t.Run("implicit context.Background", func(t *testing.T) {
		p := &Plan{}
		assert.Same(t, context.Background(), p.Context())
	})
	t.Run("explicit context.Background", func(t *testing.T) {
		p, err := NewPlan("DELETE", "http://managemystuff.com/stuff/1", nil)
		require.NotNil(t, p)
		assert.NoError(t, err)
		assert.Same(t, context.Background(), p.Context())
	})
	t.Run("explicit custom context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		q, err := NewPlanWithContext(ctx, "GET", "http://managemystuff.com/stuff/1", "")
		require.NotNil(t, q)
		assert.NoError(t, err)
		assert.Same(t, ctx, q.Context())
	})
}

func TestPlan_SetBasicAuth(t *testing.T) {
	// Create a Plan for testing, and an http.Request to use as a shadow
	// test. We assert that the Authorization header on the Plan and the
	// one on the http.Request should look the same.
	p, err := NewPlan("", "http://superdoopersecure.com", nil)
	require.NoError(t, err)
	require.NotNil(t, p)
	r, err := http.NewRequest("", "http://superdoopersecure.com", nil)
	require.NoError(t, err)
	require.NotNil(t, r)
	// Test logic is below this comment.
	p.SetBasicAuth("", "")
	r.SetBasicAuth("", "")
	assert.Equal(t, p.Header.Get("Authorization"), "Basic Og==")
	assert.Equal(t, r.Header["Authorization"], p.Header["Authorization"])
	p.SetBasicAuth("patsy", "password")
	r.SetBasicAuth("patsy", "password")
	assert.Equal(t, p.Header.Get("Authorization"), "Basic cGF0c3k6cGFzc3dvcmQ=")
	assert.Equal(t, r.Header["Authorization"], p.Header["Authorization"])
}

func TestPlan_ToRequest(t *testing.T) {
	t.Run("method not blank", func(t *testing.T) {
		p, err := NewPlan("HEAD", "test", "body")
		require.NotNil(t, p)
		require.NoError(t, err)
		assert.Equal(t, "HEAD", p.Method)
		r := p.ToRequest(context.Background())
		require.NotNil(t, r)
		assert.Equal(t, "HEAD", r.Method)
	})
	t.Run("method blank", func(t *testing.T) {
		p, err := NewPlan("", "test", "body")
		require.NotNil(t, p)
		require.NoError(t, err)
		assert.Equal(t, "GET", p.Method)
		p.Method = ""
		r := p.ToRequest(context.Background())
		require.NotNil(t, r)
		assert.Equal(t, "", r.Method)
	})
	t.Run("context background", func(t *testing.T) {
		p, err := NewPlan("POST", "test", "body")
		require.NotNil(t, p)
		require.NoError(t, err)
		r := p.ToRequest(context.Background())
		require.NotNil(t, r)
		assert.Same(t, context.Background(), r.Context())
	})
	t.Run("context other", func(t *testing.T) {
		p, err := NewPlan("PUT", "test", "body")
		require.NotNil(t, p)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := p.ToRequest(ctx)
		require.NotNil(t, r)
		assert.NotSame(t, context.Background(), r.Context())
		assert.Same(t, ctx, r.Context())
	})
	t.Run("body empty", func(t *testing.T) {
		testCases := []struct {
			name string
			body interface{}
		}{
			{name: "nil", body: nil},
			{name: "empty string", body: ""},
			{name: "empty byte slice", body: []byte{}},
			{name: "empty reader", body: strings.NewReader("")},
		}
		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				p, err := NewPlan("DELETE", "test", testCase.body)
				require.NotNil(t, p)
				require.NoError(t, err)
				r := p.ToRequest(context.Background())
				require.NotNil(t, r)
				assert.Nil(t, r.Body)
				assert.Nil(t, r.GetBody)
				assert.Equal(t, int64(0), r.ContentLength)
			})
		}
	})
	t.Run("body not empty", func(t *testing.T) {
		p, err := NewPlan("DELETE", "test", "foo")
		require.NotNil(t, p)
		require.NoError(t, err)
		r := p.ToRequest(context.Background())
		require.NotNil(t, r)
		assert.Equal(t, int64(3), r.ContentLength)
		require.NotNil(t, r.Body)
		require.NotNil(t, r.GetBody)
		b, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, string(b), "foo")
		rc, err := r.GetBody()
		require.NotNil(t, rc)
		assert.NoError(t, err)
		b, err = ioutil.ReadAll(rc)
		assert.NoError(t, err)
		assert.Equal(t, string(b), "foo")
	})
}

func TestPlan_WithContext(t *testing.T) {
	p, err := NewPlan("PATCH", "test", "body")
	require.NotNil(t, p)
	require.NoError(t, err)
	t.Run("nil context", func(t *testing.T) {
		assert.PanicsWithValue(t, nilCtxMsg, func() {
			p.WithContext(nil)
		})
	})
	t.Run("valid context", func(t *testing.T) {
		assert.Same(t, context.Background(), p.ctx)
		// Create first new Plan, q, from parent p
		type firstParent struct{}
		qctx := context.WithValue(context.Background(), firstParent{}, p)
		require.NotSame(t, qctx, context.Background())
		q := p.WithContext(qctx)
		assert.NotNil(t, q)
		assert.NotSame(t, q, p)
		assert.Same(t, context.Background(), p.ctx)
		assert.Same(t, qctx, q.ctx)
		assert.NotEqual(t, p, q)
		p.ctx = qctx
		assert.Equal(t, p, q)
		assert.Equal(t, &p.Body, &q.Body)
		// Create second new plan, r, from parent q
		type secondParent struct{}
		rctx := context.WithValue(context.Background(), secondParent{}, q)
		require.NotSame(t, rctx, context.Background())
		require.NotSame(t, rctx, qctx)
		r := p.WithContext(rctx)
		assert.NotNil(t, r)
		assert.NotSame(t, r, p)
		assert.NotSame(t, r, q)
		assert.Same(t, qctx, q.ctx)
		assert.Same(t, rctx, r.ctx)
		assert.NotEqual(t, q, r)
		q.ctx = rctx
		assert.Equal(t, q, r)
		assert.Equal(t, &p.Body, &r.Body)
	})
}
