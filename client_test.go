// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"httpx/request"
	"httpx/retry"
	"httpx/timeout"
	"httpx/transient"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/mock"
)

func TestClient(t *testing.T) {
	t.Run("happy path", testClientHappyPath)
	t.Run("zero value", testClientZeroValue)
	t.Run("attempt timeout", testClientAttemptTimeout)
	t.Run("read body error", testClientBodyError)
	t.Run("retry", testClientRetry)
	t.Run("close idle connections", testClientCloseIdleConnections)
}

func TestURLErrorOp(t *testing.T) {
	assert.Equal(t, "Get", urlErrorOp(""))
	assert.Equal(t, "Get", urlErrorOp("GET"))
	assert.Equal(t, "G", urlErrorOp("G"))
	assert.Equal(t, "X", urlErrorOp("X"))
	assert.Equal(t, "Xyz", urlErrorOp("XYZ"))
	assert.Equal(t, "Put", urlErrorOp("PUT"))
}

func testClientHappyPath(t *testing.T) {
	// Declare happy path test cases. Each test case invokes one of the
	// exported methods on Client: Get, Head, Post, and PostForm.
	testCases := []struct {
		name        string
		action      func(c *Client) (*request.Execution, error)
		extraChecks func(*testing.T, *request.Execution)
	}{
		{
			name: "Get",
			action: func(c *Client) (*request.Execution, error) {
				return c.Get("test")
			},
		},
		{
			name: "Head",
			action: func(c *Client) (*request.Execution, error) {
				return c.Head("test")
			},
		},
		{
			name: "Post",
			action: func(c *Client) (*request.Execution, error) {
				return c.Post("test", "text/plain", "foo")
			},
			extraChecks: func(t *testing.T, e *request.Execution) {
				assert.Equal(t, "text/plain", e.Request.Header.Get("Content-Type"))
				assert.Equal(t, []byte("foo"), e.Plan.Body)
			},
		},
		{
			name: "PostForm",
			action: func(c *Client) (*request.Execution, error) {
				return c.PostForm("test", url.Values{"ham": {"eggs", "spam"}})
			},
			extraChecks: func(t *testing.T, e *request.Execution) {
				assert.Equal(t, "application/x-www-form-urlencoded", e.Request.Header.Get("Content-Type"))
				assert.Equal(t, []byte("ham=eggs&ham=spam"), e.Plan.Body)
			},
		},
	}

	// run happy path test cases.
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockDoer := newMockHTTPDoer(t)
			mockTimeoutPolicy := newMockTimeoutPolicy(t)
			mockRetryPolicy := newMockRetryPolicy(t)
			cl := &Client{
				HTTPDoer:      mockDoer,
				TimeoutPolicy: mockTimeoutPolicy,
				RetryPolicy:   mockRetryPolicy,
				Handlers:      &HandlerGroup{},
			}

			resp := &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("foo")),
			}

			mockDoer.On("Do", mock.Anything).Return(resp, nil).Once()
			mockTimeoutPolicy.On("Timeout", mock.Anything).Return(time.Hour).Once()
			mockRetryPolicy.On("Decide", mock.MatchedBy(func(e *request.Execution) bool {
				return e.StatusCode() == 200
			})).Return(false).Once()

			before := time.Now()

			cl.Handlers.mock(BeforeExecutionStart).On("Handle", BeforeExecutionStart, mock.MatchedBy(func(e *request.Execution) bool {
				return e.Start == time.Time{} &&
					e.Plan != nil && e.Request == nil && e.Response == nil && !e.Ended()
			})).Once()
			cl.Handlers.mock(BeforeAttempt).On("Handle", BeforeAttempt, mock.MatchedBy(func(e *request.Execution) bool {
				return !e.Start.Before(before) && !e.Start.After(time.Now()) &&
					e.Request != nil && e.Response == nil && !e.Ended()
			})).Once()
			cl.Handlers.mock(BeforeReadBody).On("Handle", BeforeReadBody, mock.MatchedBy(func(e *request.Execution) bool {
				return e.Request != nil && e.Response == resp && e.Err == nil && !e.Ended()
			})).Once()
			cl.Handlers.mock(AfterAttemptTimeout) // Add so we can assert it was never called.
			cl.Handlers.mock(AfterAttempt).On("Handle", AfterAttempt, mock.MatchedBy(func(e *request.Execution) bool {
				return e.Request != nil && e.Response == resp && e.Err == nil && !e.Ended()
			})).Once()
			cl.Handlers.mock(AfterPlanTimeout) // Add so we can assert it was never called.
			cl.Handlers.mock(AfterExecutionEnd).On("Handle", AfterExecutionEnd, mock.MatchedBy(func(e *request.Execution) bool {
				return e.Request != nil && e.Response == resp && e.Err == nil && e.Attempt == 0 && e.Ended()
			})).Once()

			e, err := testCase.action(cl)

			mockDoer.AssertExpectations(t)
			mockTimeoutPolicy.AssertExpectations(t)
			mockRetryPolicy.AssertExpectations(t)
			cl.Handlers.assertExpectations(t)
			cl.Handlers.mock(AfterAttemptTimeout).AssertNotCalled(t, "Handle", mock.Anything, mock.Anything)
			cl.Handlers.mock(AfterPlanTimeout).AssertNotCalled(t, "Handle", mock.Anything, mock.Anything)
			require.NotNil(t, e)
			assert.NoError(t, err)
			assert.NoError(t, e.Err)
			require.NotNil(t, e.Plan)
			assert.Equal(t, "test", e.Plan.URL.String())
			require.NotNil(t, e.Request)
			assert.Equal(t, 200, e.StatusCode())
			assert.Equal(t, []byte("foo"), e.Body)

			if testCase.extraChecks != nil {
				testCase.extraChecks(t, e)
			}
		})
	}
}

func testClientZeroValue(t *testing.T) {
	testCases := []struct {
		name        string
		inst        serverInstruction
		extraChecks func(*testing.T, *request.Execution, error)
	}{
		{
			name: "expect status 200",
			inst: serverInstruction{
				StatusCode: 200,
			},
			extraChecks: func(t *testing.T, e *request.Execution, err error) {
				assert.NoError(t, err)
				assert.NoError(t, e.Err)
				require.NotNil(t, e)
				assert.NotNil(t, e.Request)
				assert.NotNil(t, e.Response)
				assert.Equal(t, 200, e.StatusCode())
				assert.Empty(t, e.Body)
				assert.Equal(t, 0, e.Attempt)
			},
		},
		{
			name: "expect status 404",
			inst: serverInstruction{
				StatusCode: 404,
				Body: []bodyChunk{
					{
						Data: []byte("the thingy was not in the place"),
					},
				},
			},
			extraChecks: func(t *testing.T, e *request.Execution, err error) {
				assert.NoError(t, err)
				assert.NoError(t, e.Err)
				require.NotNil(t, e)
				assert.NotNil(t, e.Request)
				assert.NotNil(t, e.Response)
				assert.Equal(t, 404, e.StatusCode())
				assert.Equal(t, []byte("the thingy was not in the place"), e.Body)
				assert.Equal(t, 0, e.Attempt)
			},
		},
		{
			name: "expect status 503",
			inst: serverInstruction{
				StatusCode: 503,
				Body: []bodyChunk{
					{
						Data: []byte("ain't not service in these parts"),
					},
				},
			},
			extraChecks: func(t *testing.T, e *request.Execution, err error) {
				assert.NoError(t, err)
				assert.NoError(t, e.Err)
				require.NotNil(t, e)
				assert.NotNil(t, e.Request)
				assert.NotNil(t, e.Response)
				assert.Equal(t, 503, e.StatusCode())
				assert.Equal(t, []byte("ain't not service in these parts"), e.Body)
				assert.Equal(t, retry.DefaultTimes, e.Attempt)
				assert.Equal(t, 0, e.AttemptTimeouts)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cl := &Client{} // Must use zero value!

			p := testCase.inst.toPlan(context.Background(), "POST")

			e, err := cl.Do(p)

			testCase.extraChecks(t, e, err)
		})
	}
}

func testClientCloseIdleConnections(t *testing.T) {
	t.Run("with HTTPDoer support", func(t *testing.T) {
		mockDoer := newMockHTTPDoer(t)
		cl := Client{HTTPDoer: mockDoer}
		cl.CloseIdleConnections()
		mockDoer.AssertExpectations(t)
	})
	t.Run("without HTTPDoer support", func(t *testing.T) {
		mockDoer := newMockHTTPDoerWithCloseIdleConnections(t)
		mockDoer.On("CloseIdleConnections").Once()
		cl := Client{HTTPDoer: mockDoer}
		cl.CloseIdleConnections()
		mockDoer.AssertExpectations(t)
	})
	t.Run("zero value", func(t *testing.T) {
		cl := Client{}
		cl.CloseIdleConnections()
	})
}

func testClientAttemptTimeout(t *testing.T) {
	testCases := []string{
		"from attempt deadline",
		"from plan deadline",
	}

	for i, testCase := range testCases {
		isPlanTimeout := i == 1
		t.Run(testCase, func(t *testing.T) {
			cl := &Client{
				HTTPDoer:      &http.Client{},
				TimeoutPolicy: timeout.Fixed(1 * time.Millisecond),
				RetryPolicy:   retry.Never,
				Handlers:      &HandlerGroup{},
			}
			cl.Handlers.mock(BeforeExecutionStart).On("Handle", BeforeExecutionStart, mock.Anything).Return().Once()
			cl.Handlers.mock(BeforeAttempt).On("Handle", BeforeAttempt, mock.Anything).Return().Once()
			cl.Handlers.mock(AfterAttemptTimeout).On("Handle", AfterAttemptTimeout, mock.Anything).Return().Once()
			if isPlanTimeout {
				cl.Handlers.mock(AfterPlanTimeout).On("Handle", AfterPlanTimeout, mock.Anything).Return().Once()
			}
			cl.Handlers.mock(AfterAttempt).On("Handle", AfterAttempt, mock.Anything).Return().Once()
			cl.Handlers.mock(AfterExecutionEnd).On("Handle", AfterExecutionEnd, mock.Anything).Return().Once()

			ctx := context.Background()
			var cancel context.CancelFunc
			if isPlanTimeout {
				ctx, cancel = context.WithTimeout(ctx, 5*time.Microsecond)
			}
			p := (&serverInstruction{StatusCode: 200, HeaderPause: 100 * time.Millisecond}).toPlan(ctx, "POST")
			e, err := cl.Do(p)
			if cancel != nil {
				cancel()
			}

			cl.Handlers.assertExpectations(t)
			require.NotNil(t, e)
			assert.Same(t, err, e.Err)
			assert.Equal(t, transient.Timeout, transient.Categorize(err))
			assert.IsType(t, &url.Error{}, err)
			assert.NotNil(t, e.Request)
			assert.Nil(t, e.Response)
			assert.Equal(t, e.Attempt, 0)
			assert.Equal(t, e.AttemptTimeouts, 1)
		})
	}
}

func testClientBodyError(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		cl := &Client{
			HTTPDoer:      &http.Client{},
			TimeoutPolicy: timeout.Fixed(50 * time.Millisecond),
			RetryPolicy:   retry.Never,
			Handlers:      &HandlerGroup{},
		}
		trace := cl.addTraceHandlers()
		p := (&serverInstruction{
			StatusCode: 200,
			Body: []bodyChunk{
				{
					Data: []byte("hello"),
				},
				{
					Pause: 100 * time.Millisecond,
					Data:  []byte("world"),
				},
			},
		}).toPlan(context.Background(), "POST")

		e, err := cl.Do(p)

		require.NotNil(t, e)
		assert.Error(t, err)
		assert.Error(t, e.Err)
		assert.Same(t, err, e.Err)
		assert.IsType(t, &url.Error{}, err)
		assert.Equal(t, transient.Timeout, transient.Categorize(err))
		urlError := err.(*url.Error)
		assert.True(t, urlError.Timeout())
		assert.Equal(t, "Post", urlError.Op)
		assert.Equal(t, []string{
			"BeforeExecutionStart",
			"BeforeAttempt",
			"BeforeReadBody",
			"AfterAttemptTimeout",
			"AfterAttempt",
			"AfterExecutionEnd",
		}, trace.calls)
		require.NotNil(t, e.Request)
		assert.Equal(t, e.Request.URL.String(), urlError.URL)
		assert.NotNil(t, e.Response)
		assert.NotNil(t, e.Body) // ioutil.ReadAll returns non-nil []byte plus error
		assert.Equal(t, 0, e.Attempt)
		assert.Equal(t, 1, e.AttemptTimeouts)
		assert.Equal(t, 200, e.StatusCode())
	})

	t.Run("close", func(t *testing.T) {
		mockDoer := newMockHTTPDoer(t)
		cl := &Client{
			HTTPDoer: mockDoer,
			Handlers: &HandlerGroup{},
		}
		trace := cl.addTraceHandlers()
		mockReadCloser := newMockReadCloser(t)
		mockDoer.On("Do", mock.Anything).Return(&http.Response{
			StatusCode: 202,
			Body:       mockReadCloser,
		}, nil).Once()
		mockReadCloser.On("Read", mock.Anything).Return(0, io.EOF).Once()
		closeErr := errors.New("a very bad closing error")
		mockReadCloser.On("Close").Return(closeErr).Once()

		e, err := cl.Get("test")

		mockDoer.AssertExpectations(t)
		mockReadCloser.AssertExpectations(t)
		require.NotNil(t, e)
		assert.NoError(t, err)
		assert.NoError(t, err, e.Err)
		assert.False(t, e.Timeout())
		assert.NotNil(t, e.Request)
		assert.NotNil(t, e.Response)
		assert.Equal(t, 202, e.StatusCode())
		assert.Equal(t, []byte{}, e.Body)
		assert.Equal(t, []string{
			"BeforeExecutionStart",
			"BeforeAttempt",
			"BeforeReadBody",
			"AfterAttempt",
			"AfterExecutionEnd",
		}, trace.calls)
	})
}

func testClientRetry(t *testing.T) {
	t.Run("plan timeout during wait", testClientRetryPlanTimeout)
	t.Run("various", testClientRetryVarious)
}

func testClientRetryPlanTimeout(t *testing.T) {
	mockDoer := newMockHTTPDoer(t)
	mockRetryPolicy := newMockRetryPolicy(t)
	cl := Client{
		HTTPDoer:    mockDoer,
		RetryPolicy: mockRetryPolicy,
		Handlers:    &HandlerGroup{},
	}
	trace := cl.addTraceHandlers()
	mockDoer.On("Do", mock.Anything).Return(&http.Response{
		Body: ioutil.NopCloser(bytes.NewReader(nil)),
	}, nil).Once()
	mockRetryPolicy.On("Decide", mock.Anything).Return(true).Once()
	mockRetryPolicy.On("Wait", mock.Anything).Return(time.Hour).Once()
	cl.Handlers.mock(AfterPlanTimeout).On("Handle", AfterPlanTimeout, mock.MatchedBy(func(e *request.Execution) bool {
		err, ok := e.Err.(*url.Error)
		return e.Attempt == 0 && e.AttemptTimeouts == 0 &&
			e.Request != nil && e.Response != nil && e.Body != nil &&
			ok && err.Timeout()
	})).Return().Once()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	p, err := request.NewPlanWithContext(ctx, "GET", "test", nil)
	p.Method = "" // http.Client should interpret this as GET.
	require.NoError(t, err)
	e, err := cl.Do(p)
	cancel()

	mockDoer.AssertExpectations(t)
	mockRetryPolicy.AssertExpectations(t)
	require.NotNil(t, e)
	assert.Equal(t, []string{
		"BeforeExecutionStart",
		"BeforeAttempt",
		"BeforeReadBody",
		"AfterAttempt",
		"AfterPlanTimeout",
		"AfterExecutionEnd",
	}, trace.calls)
	assert.NotNil(t, e.Request)
	assert.NotNil(t, e.Response)
	assert.NotNil(t, e.Body)
	assert.Equal(t, 0, e.Attempt)
	assert.Equal(t, 0, e.AttemptTimeouts)
	assert.True(t, e.Timeout())
	assert.Error(t, err)
	assert.Error(t, e.Err)
	assert.Same(t, err, e.Err)
	require.IsType(t, &url.Error{}, err)
	urlError := err.(*url.Error)
	assert.Equal(t, "Get", urlError.Op)
	assert.Equal(t, "test", urlError.URL)
	assert.True(t, urlError.Timeout())
}

func testClientRetryVarious(t *testing.T) {
	iterations := []struct {
		name         string
		doResp       *http.Response
		doErr        error
		handlerCalls []string
		assertFunc   func(*testing.T, *request.Execution)
	}{
		{
			name:   "timeout",
			doResp: nil,
			doErr: &url.Error{
				Op:  "Foop",
				URL: "boop",
				Err: syscall.ETIMEDOUT,
			},
			handlerCalls: []string{
				"BeforeAttempt",
				"AfterAttemptTimeout",
				"AfterAttempt",
			},
			assertFunc: func(t *testing.T, e *request.Execution) {
				require.IsType(t, &url.Error{}, e.Err)
				urlError := e.Err.(*url.Error)
				assert.True(t, urlError.Timeout())
				assert.Equal(t, 0, e.StatusCode())
				assert.Nil(t, e.Response)
				assert.Nil(t, e.Body)
			},
		},
		{
			name: "service unavailable",
			doResp: &http.Response{
				StatusCode: 503,
				Body:       ioutil.NopCloser(strings.NewReader("There just isn't a lot of service right now.")),
			},
			doErr: nil,
			handlerCalls: []string{
				"BeforeAttempt",
				"BeforeReadBody",
				"AfterAttempt",
			},
			assertFunc: func(t *testing.T, e *request.Execution) {
				assert.Nil(t, e.Err)
				assert.Equal(t, 503, e.StatusCode())
				assert.NotNil(t, e.Response)
				assert.Equal(t, []byte("There just isn't a lot of service right now."), e.Body)
			},
		},
		{
			name:   "connection reset",
			doResp: nil,
			doErr: &url.Error{
				Op:  "bloop",
				URL: "smoop",
				Err: syscall.ECONNRESET,
			},
			handlerCalls: []string{
				"BeforeAttempt",
				"AfterAttempt",
			},
			assertFunc: func(t *testing.T, e *request.Execution) {
				require.IsType(t, &url.Error{}, e.Err)
				urlError := e.Err.(*url.Error)
				assert.False(t, urlError.Timeout())
				assert.Equal(t, syscall.ECONNRESET, urlError.Err)
				assert.Equal(t, 0, e.StatusCode())
				assert.Nil(t, e.Response)
				assert.Nil(t, e.Body)
			},
		},
		{
			name: "no content",
			doResp: &http.Response{
				StatusCode: 204,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			},
			handlerCalls: []string{
				"BeforeAttempt",
				"BeforeReadBody",
				"AfterAttempt",
			},
			assertFunc: func(t *testing.T, e *request.Execution) {
				assert.Nil(t, e.Err)
				assert.Equal(t, 204, e.StatusCode())
				assert.NotNil(t, e.Response)
				assert.Equal(t, []byte{}, e.Body)
			},
		},
	}

	for i, iter := range iterations {
		name := fmt.Sprintf("0..%d (n=%d, last=%s)", i, i+1, iter.name)
		t.Run(name, func(t *testing.T) {
			mockDoer := newMockHTTPDoer(t)
			mockDoer.Test(t)
			handlerCalls := make([]string, 0, 2+5*i)
			handlerCalls = append(handlerCalls, "BeforeExecutionStart")
			for j := 0; j <= i; j++ {
				mockDoer.On("Do", mock.Anything).Return(iterations[j].doResp, iterations[j].doErr).Once()
				handlerCalls = append(handlerCalls, iterations[j].handlerCalls...)
			}
			handlerCalls = append(handlerCalls, "AfterExecutionEnd")
			retryPolicy := retry.NewPolicy(
				retry.Times(i).And(retry.TransientErr.Or(retry.StatusCode(503))),
				retry.NewExpWaiter(time.Nanosecond, time.Nanosecond, nil))
			cl := Client{
				HTTPDoer:    mockDoer,
				RetryPolicy: retryPolicy,
				Handlers:    &HandlerGroup{},
			}
			tracer := cl.addTraceHandlers()

			before := time.Now()
			e, err := cl.Post(iter.name, "text/plain", iter.name)
			after := time.Now()

			mockDoer.AssertExpectations(t)
			require.NotNil(t, e)
			if err == nil {
				require.Nil(t, e.Err)
			} else {
				require.Same(t, err, e.Err)
			}
			require.NotNil(t, e.Request)
			assert.Equal(t, i, e.Attempt)
			assert.Equal(t, 1, e.AttemptTimeouts)
			assert.True(t, e.Ended())
			assert.GreaterOrEqual(t, e.Duration(), time.Duration(0))
			assert.False(t, e.Start.Before(before))
			assert.False(t, e.End.After(after))
			assert.Equal(t, handlerCalls, tracer.calls)
			iter.assertFunc(t, e)
		})
	}
}

type mockHTTPDoer struct {
	mock.Mock
}

func newMockHTTPDoer(t *testing.T) *mockHTTPDoer {
	m := &mockHTTPDoer{}
	m.Test(t)
	return m
}

func (m *mockHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

type mockHTTPDoerWithCloseIdleConnections struct {
	mockHTTPDoer
}

func newMockHTTPDoerWithCloseIdleConnections(t *testing.T) *mockHTTPDoerWithCloseIdleConnections {
	m := &mockHTTPDoerWithCloseIdleConnections{}
	m.Test(t)
	return m
}

func (m *mockHTTPDoerWithCloseIdleConnections) CloseIdleConnections() {
	m.Called()
}

type mockTimeoutPolicy struct {
	mock.Mock
}

func newMockTimeoutPolicy(t *testing.T) *mockTimeoutPolicy {
	m := &mockTimeoutPolicy{}
	m.Test(t)
	return m
}

func (m *mockTimeoutPolicy) Timeout(e *request.Execution) time.Duration {
	args := m.Called(e)
	return args.Get(0).(time.Duration)
}

type mockRetryPolicy struct {
	mock.Mock
}

func newMockRetryPolicy(t *testing.T) *mockRetryPolicy {
	m := &mockRetryPolicy{}
	m.Test(t)
	return m
}

func (m *mockRetryPolicy) Decide(e *request.Execution) bool {
	args := m.Called(e)
	return args.Bool(0)
}

func (m *mockRetryPolicy) Wait(e *request.Execution) time.Duration {
	args := m.Called(e)
	return args.Get(0).(time.Duration)
}

func (g *HandlerGroup) mock(evt Event) *mockHandler {
	var m *mockHandler
	if len(g.handlers) <= int(evt) || len(g.handlers[evt]) < 1 {
		m = &mockHandler{}
		g.PushBack(evt, m)
		return m
	}

	for _, h := range g.handlers[evt] {
		if m, ok := h.(*mockHandler); ok {
			return m
		}
	}

	m = &mockHandler{}
	g.PushBack(evt, m)
	return m
}

func (g *HandlerGroup) assertExpectations(t *testing.T) {
	if g.handlers == nil {
		return
	}

	for _, evt := range Events() {
		handlers := g.handlers[evt]
		for _, h := range handlers {
			if m, ok := h.(*mockHandler); ok {
				m.AssertExpectations(t)
			}
		}
	}
}

type mockHandler struct {
	mock.Mock
}

func (m *mockHandler) Handle(evt Event, e *request.Execution) {
	m.Called(evt, e)
}

type trace struct {
	calls []string
}

func (c *Client) addTraceHandlers() *trace {
	tr := &trace{}
	f := func(evt Event, _ *request.Execution) {
		tr.calls = append(tr.calls, evt.Name())
	}
	h := HandlerFunc(f)
	for _, evt := range Events() {
		c.Handlers.PushBack(evt, h)
	}
	return tr
}

type mockReadCloser struct {
	mock.Mock
}

func newMockReadCloser(t *testing.T) *mockReadCloser {
	m := &mockReadCloser{}
	m.Test(t)
	return m
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	args := m.Called(p)
	n = args.Int(0)
	err = args.Error(1)
	return
}

func (m *mockReadCloser) Close() error {
	args := m.Called()
	return args.Error(0)
}

// TODO: Need to add an HTTP2 test here to make sure that works.
