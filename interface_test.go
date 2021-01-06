// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/gogama/httpx/request"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/mock"
)

func TestGet(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		expected := &request.Execution{}
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "GET" && p.URL.String() == "foo"
		})).Return(expected, nil).Once()
		e, err := Get(m, "foo")
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("error invalid URL", func(t *testing.T) {
		m := newMockDoer(t)
		e, err := Get(m, ":::")
		assert.Nil(t, e)
		assert.Error(t, err)
		m.AssertNotCalled(t, "Do", mock.Anything)
	})
}

func TestHead(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		expected := &request.Execution{}
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "HEAD" && p.URL.String() == "bar"
		})).Return(expected, nil).Once()
		e, err := Head(m, "bar")
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("error invalid URL", func(t *testing.T) {
		m := newMockDoer(t)
		e, err := Head(m, ":::")
		assert.Nil(t, e)
		assert.Error(t, err)
		m.AssertNotCalled(t, "Do", mock.Anything)
	})
}

func TestPost(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		expected := &request.Execution{}
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "POST" && p.URL.String() == "baz" &&
				p.Header.Get("Content-Type") == "ham" &&
				bytes.Equal(p.Body, []byte("eggs"))
		})).Return(expected, nil).Once()
		e, err := Post(m, "baz", "ham", "eggs")
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("error invalid URL", func(t *testing.T) {
		m := newMockDoer(t)
		e, err := Post(m, ":::", "text/plain", []byte{'a', 'b', 'c'})
		assert.Nil(t, e)
		assert.Error(t, err)
		m.AssertNotCalled(t, "Do", mock.Anything)
	})
	t.Run("error invalid body", func(t *testing.T) {
		m := newMockDoer(t)
		e, err := Post(m, ":::", "text/plain", 123)
		assert.Nil(t, e)
		assert.EqualError(t, err, "httpx/request: invalid type (for body use nil, string, []byte, io.Reader or io.ReadCloser)")
		m.AssertNotCalled(t, "Do", mock.Anything)
	})
}

func TestPostForm(t *testing.T) {
	expected := &request.Execution{}
	m := newMockDoer(t)
	m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
		return p.Method == "POST" && p.URL.String() == "poster%20boy" &&
			p.Header.Get("Content-Type") == "application/x-www-form-urlencoded" &&
			len(p.Body) == 0
	})).Return(expected, nil).Once()
	e, err := PostForm(m, "poster boy", url.Values{})
	assert.Same(t, expected, e)
	assert.NoError(t, err)
	m.AssertExpectations(t)
}

func TestInflate(t *testing.T) {
	t.Run("Inflate", func(t *testing.T) {
		t.Run("nil doer", func(t *testing.T) {
			assert.PanicsWithValue(t, "httpx: nil doer", func() {
				Inflate(nil)
			})
		})
		t.Run("already an Executor", func(t *testing.T) {
			cl := &Client{}
			x := Inflate(cl)
			assert.Same(t, cl, x)
		})
		t.Run("not yet an Executor", func(t *testing.T) {
			m := newMockDoer(t)
			x := Inflate(m)
			assert.NotSame(t, m, x)
		})
	})
	expected := &request.Execution{}
	t.Run("Do", func(t *testing.T) {
		p, err := request.NewPlan("PUT", "http://www.randomcollections.com/widgets/1", "foo")
		require.NotNil(t, p)
		require.NoError(t, err)
		m := newMockDoer(t)
		m.On("Do", p).Return(expected, nil).Once()
		x := Inflate(m)
		e, err := x.Do(p)
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("Get", func(t *testing.T) {
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "GET" && p.URL.String() == "bar"
		})).Return(expected, nil).Once()
		x := Inflate(m)
		e, err := x.Get("bar")
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("Head", func(t *testing.T) {
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "HEAD" && p.URL.String() == "baz"
		})).Return(expected, nil).Once()
		x := Inflate(m)
		e, err := x.Head("baz")
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("Post", func(t *testing.T) {
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "POST" && p.URL.String() == "ham" &&
				p.Header.Get("Content-Type") == "eggs" &&
				p.Body == nil
		})).Return(expected, nil).Once()
		x := Inflate(m)
		e, err := x.Post("ham", "eggs", nil)
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("PostForm", func(t *testing.T) {
		m := newMockDoer(t)
		m.On("Do", mock.MatchedBy(func(p *request.Plan) bool {
			return p.Method == "POST" && p.URL.String() == "form" &&
				p.Header.Get("Content-Type") == "application/x-www-form-urlencoded" &&
				bytes.Equal(p.Body, []byte("x=y"))
		})).Return(expected, nil).Once()
		x := Inflate(m)
		e, err := x.PostForm("form", url.Values{"x": []string{"y"}})
		assert.Same(t, expected, e)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("CloseIdleConnections", func(t *testing.T) {
		t.Run("Doer implements IdleCloser", func(t *testing.T) {
			m := newMockDoer(t)
			x := Inflate(m)
			x.CloseIdleConnections()
			m.AssertNotCalled(t, "CloseIdleConnections")
		})
		t.Run("Doer does not implement IdleCloser", func(t *testing.T) {
			m := newMockDoerWithCloseIdleConnections(t)
			m.On("CloseIdleConnections").Once()
			x := Inflate(m)
			x.CloseIdleConnections()
			m.AssertExpectations(t)
		})
	})
}

type mockDoer struct {
	mock.Mock
}

func newMockDoer(t *testing.T) *mockDoer {
	m := &mockDoer{}
	m.Test(t)
	return m
}

func (m *mockDoer) Do(p *request.Plan) (*request.Execution, error) {
	args := m.Called(p)
	e := args.Get(0)
	err := args.Error(1)
	if e == nil {
		return nil, err
	}
	return e.(*request.Execution), err
}

type mockDoerWithCloseIdleConnections struct {
	mockDoer
}

func newMockDoerWithCloseIdleConnections(t *testing.T) *mockDoerWithCloseIdleConnections {
	m := &mockDoerWithCloseIdleConnections{}
	m.Test(t)
	return m
}

func (m *mockDoerWithCloseIdleConnections) CloseIdleConnections() {
	m.Called()
}
