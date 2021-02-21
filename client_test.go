// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gogama/httpx/request"
	"github.com/gogama/httpx/retry"
	"github.com/gogama/httpx/timeout"
	"github.com/gogama/httpx/transient"

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
	t.Run("panic", testClientPanic)
	t.Run("plan cancel", testClientPlanCancel)
	t.Run("plan replace", testClientPlanReplace)
	t.Run("close idle connections", testClientCloseIdleConnections)
	//t.Run("racing", testClientRacing) FIXME
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
	t.Parallel()
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

	// Run happy path test cases.
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
				return e.Request != nil && e.Response == resp && e.Err == nil && e.Attempt == 0 &&
					e.Racing == 0 && e.Wave == 0 && e.Ended()
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
			assert.Equal(t, 0, e.Attempt)
			assert.Equal(t, 0, e.Racing)
			assert.Equal(t, 0, e.Wave)

			if testCase.extraChecks != nil {
				testCase.extraChecks(t, e)
			}
		})
	}
}

func testClientZeroValue(t *testing.T) {
	t.Parallel()

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
				assert.Equal(t, 0, e.Racing)
				assert.Equal(t, 0, e.Wave)
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
				assert.Equal(t, 0, e.Racing)
				assert.Equal(t, 0, e.Wave)
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
				assert.Equal(t, 0, e.Racing)
				assert.Equal(t, retry.DefaultTimes, e.Wave)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cl := &Client{} // Must use zero value!

			p := testCase.inst.toPlan(context.Background(), "POST", httpServer)

			e, err := cl.Do(p)

			testCase.extraChecks(t, e, err)
		})
	}
}

func testClientAttemptTimeout(t *testing.T) {
	t.Parallel()

	testCases := []string{
		"from attempt deadline",
		"from plan deadline",
	}

	for i, testCase := range testCases {
		isPlanTimeout := i == 1
		t.Run(testCase, func(t *testing.T) {
			t.Parallel()

			for _, server := range servers {
				t.Run(serverName(server), func(t *testing.T) {
					cl := &Client{
						HTTPDoer:      server.Client(),
						TimeoutPolicy: timeout.Fixed(1 * time.Millisecond),
						RetryPolicy:   retry.Never,
						Handlers:      &HandlerGroup{},
					}
					cl.Handlers.mock(BeforeExecutionStart).On("Handle", BeforeExecutionStart, mock.Anything).Return().Once()
					cl.Handlers.mock(BeforeAttempt).On("Handle", BeforeAttempt, mock.Anything).Return().Once()
					cl.Handlers.mock(AfterAttemptTimeout).On("Handle", AfterAttemptTimeout, mock.Anything).Return().Once()
					if isPlanTimeout {
						cl.Handlers.mock(AfterPlanTimeout).On("Handle", AfterPlanTimeout, mock.Anything).Return().Once()
						// FIXME: Very rarely, I've seen the "from plan deadline" test
						//        fail when asserting mock expectations because the
						//        AfterPlanTimeout handler didn't get called. Need to
						//        make this test more reliable...
					}
					cl.Handlers.mock(AfterAttempt).On("Handle", AfterAttempt, mock.Anything).Return().Once()
					cl.Handlers.mock(AfterExecutionEnd).On("Handle", AfterExecutionEnd, mock.Anything).Return().Once()

					ctx := context.Background()
					var cancel context.CancelFunc
					if isPlanTimeout {
						ctx, cancel = context.WithTimeout(ctx, 5*time.Microsecond)
					}
					p := (&serverInstruction{StatusCode: 200, HeaderPause: 100 * time.Millisecond}).toPlan(ctx, "POST", server)
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
					assert.Equal(t, 0, e.Racing)
					assert.Equal(t, 0, e.Wave)
				})
			}
		})
	}
}

func testClientBodyError(t *testing.T) {
	t.Parallel()

	t.Run("timeout", func(t *testing.T) {
		for _, server := range servers {
			t.Run(serverName(server), func(t *testing.T) {
				t.Parallel()

				cl := &Client{
					HTTPDoer:      server.Client(),
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
				}).toPlan(context.Background(), "POST", server)

				e, err := cl.Do(p)

				require.NotNil(t, e)
				assert.Error(t, err)
				assert.Error(t, e.Err)
				assert.Same(t, err, e.Err)
				assert.Equal(t, transient.Timeout, transient.Categorize(err))
				require.IsType(t, &url.Error{}, err)
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
				assert.Equal(t, 0, e.Racing)
				assert.Equal(t, 0, e.Wave)
				assert.Equal(t, 200, e.StatusCode())
			})
		}
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
		// FIXME: I have occasionally seen a failure in this case because
		//        the "Close" call wasn't called on the mockReadCloser.

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
	t.Parallel()
	t.Run("plan timeout during wait", testClientRetryPlanTimeout)
	t.Run("various", testClientRetryVarious)
}

func testClientRetryPlanTimeout(t *testing.T) {
	t.Parallel()

	// Force a retry, then make the retry wait so long the plan times out!
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
	// FIXME: I have occasionally seen this test case fail because the Decide
	//        doesn't get called on the mock. This one is pretty bizarre, because
	//        the Wait is getting called even though the Decide isn't, which
	//        shouldn't be possible. This is related to the halt/stop confusion
	//        and the FIXME in handleCheckpoint(). But I think what is happening
	//        is that every so often, the plan timeout occurs before the retry
	//        evaluation can happen and then the short-circuit logic at the FIXME
	//        in handleCheckpoint() is triggered. So probably the thing to do is
	//        first fix the halt/stop bug/confusion, then increase the plan timeout
	//        in this test above 10 milliseconds.
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
	assert.Equal(t, 0, e.Racing)
	assert.Equal(t, 0, e.Wave)
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
	t.Parallel()

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
			assert.Equal(t, 0, e.Racing)
			assert.Equal(t, i, e.Wave)
			assert.True(t, e.Ended())
			assert.GreaterOrEqual(t, e.Duration(), time.Duration(0))
			assert.False(t, e.Start.Before(before))
			assert.False(t, e.End.After(after))
			assert.Equal(t, handlerCalls, tracer.calls)
			iter.assertFunc(t, e)
		})
	}
}

func testClientPanic(t *testing.T) {
	t.Parallel()
	t.Run("ensure cancel called", testClientPanicEnsureCancelCalled)
	t.Run("ensure Body closed", testClientPanicEnsureBodyClosed)
}

func testClientPanicEnsureCancelCalled(t *testing.T) {
	// Ensure that if the event handler panics, the request context
	// cancel function is called.
	for _, evt := range []Event{BeforeAttempt, BeforeReadBody} {
		t.Run(evt.Name(), func(t *testing.T) {
			doer := newMockHTTPDoer(t)
			handlers := &HandlerGroup{}
			cl := &Client{
				HTTPDoer: doer,
				Handlers: handlers,
			}
			resp := &http.Response{
				Body: ioutil.NopCloser(bytes.NewReader(nil)),
			}
			doer.On("Do", mock.Anything).Return(resp, nil).Once()
			var e *request.Execution
			handlers.mock(evt).On("Handle", evt, mock.MatchedBy(func(x *request.Execution) bool {
				e = x
				return true
			})).Panic("omg omg").Once()

			require.Panics(t, func() { _, _ = cl.Get("test") })
			require.NotNil(t, e)
			assert.Equal(t, 0, e.Attempt)
			assert.Equal(t, 0, e.Racing)
			assert.Equal(t, 0, e.Wave)
			require.NotNil(t, e.Request)
			assert.Same(t, context.Canceled, e.Request.Context().Err())
		})
	}
}

func testClientPanicEnsureBodyClosed(t *testing.T) {
	doer := newMockHTTPDoer(t)
	handlers := &HandlerGroup{}
	cl := &Client{
		HTTPDoer: doer,
		Handlers: handlers,
	}
	readCloser := newMockReadCloser(t)
	resp := &http.Response{
		Body: readCloser,
	}
	doer.On("Do", mock.Anything).Return(resp, nil).Once()
	readCloser.On("Read", mock.Anything).Return(0, context.Canceled)
	readCloser.On("Close").Return(nil).Once()
	handlers.mock(BeforeReadBody).On("Handle", BeforeReadBody, mock.Anything).Panic("bah").Once()

	require.Panics(t, func() { _, _ = cl.Get("test") })
	doer.AssertExpectations(t)
	readCloser.AssertExpectations(t)
}

func testClientPlanCancel(t *testing.T) {
	t.Run("plan cancelled during request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		doer := newMockHTTPDoer(t)
		doer.On("Do", mock.AnythingOfType("*http.Request")).
			Run(func(_ mock.Arguments) { cancel() }).
			Return(nil, context.Canceled).
			Once()
		cl := &Client{
			HTTPDoer: doer,
		}
		p, err := request.NewPlanWithContext(ctx, "", "test", nil)
		require.NoError(t, err)

		e, err := cl.Do(p)

		doer.AssertExpectations(t)
		require.NotNil(t, e)
		assert.Error(t, err)
		var urlError *url.Error
		require.ErrorAs(t, err, &urlError)
		assert.Same(t, context.Canceled, urlError.Err)
		assert.Same(t, err, e.Err)
		assert.Same(t, p, e.Plan)
	})
	t.Run("plan cancelled after request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		doer := newMockHTTPDoer(t)
		resp := &http.Response{
			StatusCode: 99,
			Body:       ioutil.NopCloser(strings.NewReader("bar")),
		}
		doer.On("Do", mock.AnythingOfType("*http.Request")).
			Return(resp, nil).
			Once()
		handlers := &HandlerGroup{}
		handlers.mock(AfterAttempt).
			On("Handle", AfterAttempt, mock.Anything).
			Run(func(_ mock.Arguments) { cancel() }).
			Once()
		cl := &Client{
			HTTPDoer: doer,
			Handlers: handlers,
		}
		p, err := request.NewPlanWithContext(ctx, "POST", "test", "foo")
		require.NoError(t, err)

		e, err := cl.Do(p)

		doer.AssertExpectations(t)
		handlers.assertExpectations(t)
		require.NotNil(t, e)
		assert.Error(t, err)
		var urlError *url.Error
		require.ErrorAs(t, err, &urlError)
		assert.Same(t, context.Canceled, urlError.Err)
		assert.Same(t, err, e.Err)
		assert.Same(t, p, e.Plan)
	})
}

func testClientPlanReplace(t *testing.T) {
	// TODO: Test that when you change the plan, it "sticks".
}

func testClientCloseIdleConnections(t *testing.T) {
	t.Parallel()
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

func TestClientRacing(t *testing.T) {
	// What are some test cases we can do?
	//
	// 1. Enhance TestClientRacingRetry to look capture event handler
	//    names. (Using addTraceHandlers.)
	//
	// 2. Schedule an extra attempt. Return success, take a long time.
	//    Ensure long time taker cancelled as redundant AND that all
	//    the handlers are called as expected.
	//
	// 3. Ensure plan cancelled in the middle of the wave loop doesn't
	//    keep creating new attempts.
	//
	// 4. Ensure plan cancelled while multiple concurrent requests running
	//    is correctly detected.
	//
	// 5. Panic in a handler while a bunch of parallel request attempts
	//    are running and ensure the cleanup code runs.
}

func TestClientRacingNeverStart(t *testing.T) {
	// This test schedules one concurrent attempt but never starts it.
	retryPolicy := newMockRetryPolicy(t)
	racingPolicy := newMockRacingPolicy(t)
	cl := Client{
		RetryPolicy:  retryPolicy,
		RacingPolicy: racingPolicy,
		Handlers:     &HandlerGroup{},
	}
	retryPolicy.On("Decide", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave == 0 && e.StatusCode() == 204
	})).Return(false)
	racingPolicy.On("Schedule", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Attempt == 0 && e.Racing == 1
	})).Return(time.Nanosecond).Once()
	racingPolicy.On("Start", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Attempt == 0 && e.Racing == 1
	})).Return(false).Once()

	inst := serverInstruction{
		HeaderPause: 50 * time.Millisecond,
		StatusCode:  204,
	}
	p := inst.toPlan(context.Background(), "POST", httpServer)

	e, err := cl.Do(p)

	retryPolicy.AssertExpectations(t)
	racingPolicy.AssertExpectations(t)
	cl.Handlers.assertExpectations(t)
	require.NotNil(t, e)
	assert.NoError(t, err)
	assert.NoError(t, e.Err)
	assert.Equal(t, 204, e.StatusCode())
	assert.Equal(t, 0, e.Attempt)
	assert.Equal(t, 0, e.Racing)
	assert.Equal(t, 0, e.Wave)
}

func TestClientRacingRetry(t *testing.T) {
	// This test schedules two additional attempts to race the first one,
	// and all three requests "fail" (result in a positive retry decision).
	// On the next wave, no new concurrent attempts are scheduled and the
	// single second wave request succeeds.
	doer := newMockHTTPDoer(t)
	retryPolicy := newMockRetryPolicy(t)
	racingPolicy := newMockRacingPolicy(t)
	cl := Client{
		HTTPDoer:     doer,
		RetryPolicy:  retryPolicy,
		RacingPolicy: racingPolicy,
		Handlers:     &HandlerGroup{},
	}
	doer.On("Do", mock.Anything).
		Run(func(_ mock.Arguments) { time.Sleep(50 * time.Millisecond) }).
		Return(&http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader("racing-retry"))}, nil).
		Times(4)
	retryPolicy.On("Decide", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave == 0 && e.Attempt <= 2
	})).Return(true).Times(3)
	retryPolicy.On("Decide", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave == 1 && e.Attempt == 3
	})).Return(false).Once()
	retryPolicy.On("Wait", mock.Anything).Return(time.Nanosecond).Once()
	racingPolicy.On("Schedule", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave == 0 && e.Attempt <= 1
	})).Return(5 * time.Microsecond).Twice()
	racingPolicy.On("Schedule", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave >= 0 && e.Attempt >= 2
	})).Return(time.Duration(0)).Twice()
	racingPolicy.On("Start", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave == 0 && e.Attempt <= 1
	})).Return(true).Twice()

	e, err := cl.Get("foo")

	doer.AssertExpectations(t)
	retryPolicy.AssertExpectations(t)
	racingPolicy.AssertExpectations(t)
	require.NotNil(t, e)
	require.NoError(t, err)
	require.NoError(t, e.Err)
	assert.Equal(t, 1, e.Wave)
	assert.Equal(t, 3, e.Attempt)
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
	err := args.Error(1)
	if resp, ok := args.Get(0).(*http.Response); ok {
		return resp, err
	}
	return nil, err
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

type mockRacingPolicy struct {
	mock.Mock
}

func newMockRacingPolicy(t *testing.T) *mockRacingPolicy {
	m := &mockRacingPolicy{}
	m.Test(t)
	return m
}

func (m *mockRacingPolicy) Schedule(e *request.Execution) time.Duration {
	args := m.Called(e)
	return args.Get(0).(time.Duration)
}

func (m *mockRacingPolicy) Start(e *request.Execution) bool {
	args := m.Called(e)
	return args.Bool(0)
}
