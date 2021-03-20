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
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gogama/httpx/racing"

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
	t.Run("plan replace", testClientPlanChange)
	t.Run("close idle connections", testClientCloseIdleConnections)
	t.Run("racing", testClientRacing)
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
						TimeoutPolicy: timeout.Fixed(250 * time.Microsecond),
						RetryPolicy:   retry.Never,
						Handlers:      &HandlerGroup{},
					}
					cl.Handlers.mock(BeforeExecutionStart).On("Handle", BeforeExecutionStart, mock.Anything).Return().Once()
					cl.Handlers.mock(BeforeAttempt).On("Handle", BeforeAttempt, mock.Anything).Return().Once()
					cl.Handlers.mock(BeforeReadBody).On("Handle", BeforeReadBody, mock.Anything).Return().Maybe()
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
					p := (&serverInstruction{
						StatusCode:  201,
						HeaderPause: 25 * time.Millisecond,
						Body: []bodyChunk{
							{Pause: 50 * time.Millisecond, Data: []byte("Here is your first chunk.")},
							{Pause: 100 * time.Millisecond, Data: []byte("And here is your second and longer chunk.")},
							{Pause: 200 * time.Millisecond, Data: []byte("And here is your third and yet longer chunk.")},
							{Pause: 400 * time.Millisecond, Data: []byte("Et voilà, un quatrième morceau qui est encore plus longue.")},
							{Pause: 800 * time.Millisecond, Data: []byte(`Fifth, what is, one might say, the penultimate piece of the "protoplasm" of the response, longer than the previous one.`)},
							{Pause: 1600 * time.Millisecond, Data: []byte("And sixth, and last (but not least) is the longest chunk of all. In order to make it so, evidently, we need to pad it with an additional nonsense sentence such as this one.")},
						},
					}).toPlan(ctx, "POST", server)
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
					readBody := !cl.Handlers.mock(BeforeReadBody).
						IsMethodCallable(t, "Handle", BeforeReadBody, mock.Anything)
					if !readBody {
						assert.Nil(t, e.Response)
						assert.Equal(t, 0, e.StatusCode())
					} else {
						assert.NotNil(t, e.Response)
						assert.Equal(t, 201, e.StatusCode())
						assert.NotNil(t, e.Body)
					}
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
					TimeoutPolicy: timeout.Fixed(25 * time.Millisecond),
					RetryPolicy:   retry.Never,
					Handlers:      &HandlerGroup{},
				}
				trace := cl.addTraceHandlers()
				p := (&serverInstruction{
					StatusCode: 200,
					Body: []bodyChunk{
						{
							Pause: 3 * time.Millisecond,
							Data: []byte(`Lorem ipsum dolor sit amet,
											consectetur adipiscing elit.`),
						},
						{
							Pause: 30 * time.Millisecond,
							Data:  []byte(`Duis vel ullamcorper nibh.`),
						},
						{
							Pause: 300 * time.Millisecond,
							Data: []byte(`Pellentesque condimentum ipsum ipsum,
											facilisis elementum metus commodo sed.`),
						},
						{
							Pause: 3000 * time.Millisecond,
							Data: []byte(`Donec in sapien vitae eros tincidunt
											ehicula. Donec quis augue orci.
											Curabitur tincidunt turpis et lectus
											porta ornare. Curabitur fermentum
											aliquet rutrum.`),
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
				// Typically this test case will provoke a timeout while reading
				// the response body, so the BeforeReadBody handler will be
				// called. However in a small number of cases, the timeout
				// actually occurs while awaiting the response headers, before
				// the body read. So we need to handle both cases.
				n := len(trace.calls)
				assert.GreaterOrEqual(t, n, 5)
				assert.LessOrEqual(t, n, 6)
				assert.Equal(t, []string{
					"BeforeExecutionStart",
					"BeforeAttempt",
				}, trace.calls[0:2])
				if n == 6 {
					assert.Equal(t, "BeforeReadBody", trace.calls[2])
				}
				assert.Equal(t, []string{
					"AfterAttemptTimeout",
					"AfterAttempt",
					"AfterExecutionEnd",
				}, trace.calls[n-3:])
				require.NotNil(t, e.Request)
				assert.Equal(t, e.Request.URL.String(), urlError.URL)
				// Again typically this test case will provoke a timeout after
				// having read the headers and during the process of reading the
				// response body. However sometimes due to the vagaries of timing,
				// the timeout will occur before the headers can be read.
				if n == 6 {
					assert.NotNil(t, e.Response)
					assert.NotNil(t, e.Body) // ioutil.ReadAll returns non-nil []byte plus error
					assert.Equal(t, 200, e.StatusCode())
				} else {
					assert.Nil(t, e.Response)
					assert.Nil(t, e.Body)
					assert.Equal(t, 0, e.StatusCode())
				}
				assert.Equal(t, 0, e.Attempt)
				assert.Equal(t, 1, e.AttemptTimeouts)
				assert.Equal(t, 0, e.Racing)
				assert.Equal(t, 0, e.Wave)
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
	mockRetryPolicy.On("Decide", mock.Anything).Return(true).Maybe()
	mockRetryPolicy.On("Wait", mock.Anything).Return(time.Hour).Maybe()
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
	t.Run("in event handler", func(t *testing.T) {
		t.Run("ensure Cancel called", testClientEventHandlerPanicEnsureCancelCalled)
		t.Run("ensure Body closed", testClientEventHandlerPanicEnsureBodyClosed)
	})
	t.Run("in sendAndReceive goroutine", func(t *testing.T) {
		t.Run("core", testClientGoroutinePanicCore)
		t.Run("caused by event handler", testClientGoroutineCausedByEventHandler)
	})
}

func testClientEventHandlerPanicEnsureCancelCalled(t *testing.T) {
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

func testClientGoroutinePanicCore(t *testing.T) {
	panicVal := "boo!"
	testCases := []struct {
		name              string
		setupMockHTTPDoer func(t *testing.T, mockDoer *mockHTTPDoer) *mockReadCloser
	}{
		{
			name: "in Doer.Do",
			setupMockHTTPDoer: func(_ *testing.T, mockDoer *mockHTTPDoer) *mockReadCloser {
				mockDoer.On("Do", mock.AnythingOfType("*http.Request")).
					Panic(panicVal).
					Once()
				return nil
			},
		},
		{
			name: "reading Body",
			setupMockHTTPDoer: func(t *testing.T, mockDoer *mockHTTPDoer) *mockReadCloser {
				mockReadCloser := newMockReadCloser(t)
				mockDoer.On("Do", mock.AnythingOfType("*http.Request")).
					Return(&http.Response{StatusCode: 200, Body: mockReadCloser}, nil).
					Once()
				mockReadCloser.On("Read", mock.Anything).
					Panic(panicVal).
					Once()
				mockReadCloser.On("Close").
					Return(nil).
					Once()
				return mockReadCloser
			},
		},
		{
			name: "closing Body",
			setupMockHTTPDoer: func(t *testing.T, mockDoer *mockHTTPDoer) *mockReadCloser {
				mockReadCloser := newMockReadCloser(t)
				mockDoer.On("Do", mock.AnythingOfType("*http.Request")).
					Return(&http.Response{StatusCode: 200, Body: mockReadCloser}, nil).
					Once()
				mockReadCloser.On("Read", mock.Anything).
					Return(0, io.EOF).
					Once()
				mockReadCloser.On("Close").
					Panic(panicVal).
					Once()
				return mockReadCloser
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockDoer := newMockHTTPDoer(t)
			mockReadCloser := testCase.setupMockHTTPDoer(t, mockDoer)
			cl := Client{
				HTTPDoer:      mockDoer,
				TimeoutPolicy: timeout.Infinite,
			}
			p, err := request.NewPlan("", "test", nil)
			require.NotNil(t, p)
			require.NoError(t, err)

			assert.PanicsWithValue(t, panicVal, func() { _, _ = cl.Do(p) })

			mockDoer.AssertExpectations(t)
			if mockReadCloser != nil {
				mockReadCloser.AssertExpectations(t)
			}
		})
	}
}

func testClientGoroutineCausedByEventHandler(t *testing.T) {
	testCases := []struct {
		name                 string
		panicVal             string
		handleBeforeReadBody func(e *request.Execution)
	}{
		{
			name:     "response nilled",
			panicVal: "httpx: attempt response was nilled",
			handleBeforeReadBody: func(e *request.Execution) {
				e.Response = nil
			},
		},
		{
			name:     "response body nilled",
			panicVal: "httpx: attempt response body was nilled",
			handleBeforeReadBody: func(e *request.Execution) {
				e.Response.Body = nil
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockDoer := newMockHTTPDoer(t)
			cl := Client{
				HTTPDoer:      mockDoer,
				TimeoutPolicy: timeout.Infinite,
				Handlers:      &HandlerGroup{},
			}
			mockDoer.On("Do", mock.AnythingOfType("*http.Request")).
				Return(&http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader("never gonna be read"))}, nil).
				Once()
			cl.Handlers.mock(BeforeReadBody).
				On("Handle", BeforeReadBody, mock.AnythingOfType("*request.Execution")).
				Run(func(args mock.Arguments) {
					e := args.Get(1).(*request.Execution)
					testCase.handleBeforeReadBody(e)
				}).
				Once()
			p, err := request.NewPlan("", "test", nil)
			require.NotNil(t, p)
			require.NoError(t, err)

			assert.PanicsWithValue(t, testCase.panicVal, func() { _, _ = cl.Do(p) })

			mockDoer.AssertExpectations(t)
			cl.Handlers.assertExpectations(t)
		})
	}
}

func testClientEventHandlerPanicEnsureBodyClosed(t *testing.T) {
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

func testClientPlanChange(t *testing.T) {
	t.Parallel()

	p0, err0 := request.NewPlan("GET", "test", nil)
	require.NotNil(t, p0)
	require.NoError(t, err0)

	t.Run("to valid plan", func(t *testing.T) {
		p1, err1 := request.NewPlan("PUT", "test", nil)
		require.NotNil(t, p1)
		require.NoError(t, err1)

		doer := newMockHTTPDoer(t)
		cl := Client{
			HTTPDoer: doer,
			Handlers: &HandlerGroup{},
		}
		nonRetryableErr := errors.New("not at all retryable")
		doer.On("Do", mock.Anything).Return(nil, nonRetryableErr)
		cl.Handlers.mock(BeforeExecutionStart).On("Handle", BeforeExecutionStart, mock.MatchedBy(func(e *request.Execution) bool {
			return e.Plan == p0
		})).Run(func(args mock.Arguments) {
			e := args.Get(1).(*request.Execution)
			e.Plan = p1
		}).Once()
		p1Matcher := mock.MatchedBy(func(e *request.Execution) bool {
			return e.Plan == p1
		})
		cl.Handlers.mock(BeforeAttempt).On("Handle", BeforeAttempt, p1Matcher).Once()
		cl.Handlers.mock(AfterAttempt).On("Handle", AfterAttempt, p1Matcher).Once()
		cl.Handlers.mock(AfterExecutionEnd).On("Handle", AfterExecutionEnd, p1Matcher).Once()

		e, err := cl.Do(p0)

		doer.AssertExpectations(t)
		cl.Handlers.assertExpectations(t)
		require.NotNil(t, e)
		assert.Error(t, err)
		var urlError *url.Error
		require.ErrorAs(t, err, &urlError)
		assert.Same(t, nonRetryableErr, urlError.Unwrap())
	})
	t.Run("to nil (panic)", func(t *testing.T) {
		doer := newMockHTTPDoer(t)
		cl := Client{
			HTTPDoer: doer,
			Handlers: &HandlerGroup{},
		}
		cl.Handlers.mock(BeforeExecutionStart).On("Handle", BeforeExecutionStart, mock.MatchedBy(func(e *request.Execution) bool {
			return e.Plan == p0
		})).Run(func(args mock.Arguments) {
			e := args.Get(1).(*request.Execution)
			e.Plan = nil
		}).Once()
		cl.Handlers.mock(BeforeAttempt)     // Never called.
		cl.Handlers.mock(AfterExecutionEnd) // Never called.

		assert.PanicsWithValue(t, "httpx: plan deleted from execution", func() { cl.Do(p0) })

		doer.AssertExpectations(t)
		cl.Handlers.assertExpectations(t)
	})
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

func testClientRacing(t *testing.T) {
	t.Run("never start", testClientRacingNeverStart)
	t.Run("retry", testClientRacingRetry)
	t.Run("cancel redundant", testClientRacingCancelRedundant)
	t.Run("plan cancel in wave", testRacingPlanCancelDuringWaveLoop)
	t.Run("panic", testClientRacingPanic)
}

func testClientRacingNeverStart(t *testing.T) {
	// This test schedules one concurrent attempt but never starts it.
	doer := newMockHTTPDoer(t)
	retryPolicy := newMockRetryPolicy(t)
	racingPolicy := newMockRacingPolicy(t)
	cl := Client{
		HTTPDoer:      doer,
		TimeoutPolicy: timeout.Infinite,
		RetryPolicy:   retryPolicy,
		RacingPolicy:  racingPolicy,
		Handlers:      &HandlerGroup{},
	}
	trace := cl.addTraceHandlers()
	waiter := make(chan time.Time)
	defer close(waiter)
	doer.On("Do", mock.Anything).
		WaitUntil(waiter).
		Return(&http.Response{StatusCode: 204, Body: ioutil.NopCloser(strings.NewReader("foo"))}, nil).
		Once()
	retryPolicy.On("Decide", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Wave == 0 && e.StatusCode() == 204
	})).Return(false)
	racingPolicy.On("Schedule", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Attempt == 0 && e.Racing == 1
	})).Return(time.Nanosecond).Once()
	racingPolicy.On("Start", mock.MatchedBy(func(e *request.Execution) bool {
		return e.Attempt == 0 && e.Racing == 1
	})).
		Run(func(_ mock.Arguments) { waiter <- time.Now() }).
		Return(false).
		Once()

	e, err := cl.Get("test")

	doer.AssertExpectations(t)
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
	assert.Equal(t, []string{
		"BeforeExecutionStart",
		"BeforeAttempt",
		"BeforeReadBody",
		"AfterAttempt",
		"AfterExecutionEnd",
	}, trace.calls)
}

func testClientRacingRetry(t *testing.T) {
	// This test schedules two additional attempts to race the first one,
	// and all three requests "fail" (result in a positive retry decision).
	// On the next wave, no new concurrent attempts are scheduled and the
	// single second wave request succeeds.
	doer := newMockHTTPDoer(t)
	retryPolicy := newMockRetryPolicy(t)
	racingPolicy := newMockRacingPolicy(t)
	cl := Client{
		HTTPDoer:      doer,
		TimeoutPolicy: timeout.Infinite,
		RetryPolicy:   retryPolicy,
		RacingPolicy:  racingPolicy,
		Handlers:      &HandlerGroup{},
	}
	trace := cl.addTraceHandlers()
	bridge1, bridge2 := make(chan time.Time), make(chan time.Time)
	defer func() {
		close(bridge1)
		close(bridge2)
	}()
	// Chain the first three attempts (all are racing) so that the first one
	// waits for the second one to start, and the second one waits for the
	// third one to start.
	callOrder := callOrderMatcher{}
	doer.On("Do", mock.MatchedBy(func(_ *http.Request) bool { return callOrder.Match(0) })).
		WaitUntil(bridge1).
		Return(&http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader("racing-retry-0"))}, nil).
		Once()
	doer.On("Do", mock.MatchedBy(func(_ *http.Request) bool { return callOrder.Match(1) })).
		WaitUntil(bridge2).
		Run(func(_ mock.Arguments) { bridge1 <- time.Now() }).
		Return(&http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader("racing-retry-1"))}, nil).
		Once()
	doer.On("Do", mock.MatchedBy(func(_ *http.Request) bool { return callOrder.Match(2) })).
		Run(func(_ mock.Arguments) { bridge2 <- time.Now() }).
		Return(&http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader("racing-retry=2"))}, nil).
		Once()
	// The last attempt occurs in wave 3, by itself (no racing).
	doer.On("Do", mock.MatchedBy(func(_ *http.Request) bool { return callOrder.Match(3) })).
		Return(&http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader("racing-retry=3"))}, nil).
		Once()
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
	n := 2 + 4*3
	require.Len(t, trace.calls, n)
	assert.Equal(t, []string{
		"BeforeExecutionStart",
		"BeforeAttempt",
		"BeforeAttempt",
		"BeforeAttempt",
		"BeforeReadBody",
	}, trace.calls[0:5])
	assert.Equal(t, []string{
		"AfterAttempt",
		"BeforeAttempt",
		"BeforeReadBody",
		"AfterAttempt",
		"AfterExecutionEnd",
	}, trace.calls[n-5:n])
}

func testClientRacingCancelRedundant(t *testing.T) {
	// This test schedules and starts a second request racing the first
	// one. The second attempt completes before the first one. The test
	// ensures the first attempt is cancelled as redundant.
	testCases := []struct {
		name                string
		setupFirstDoCall    func(t *testing.T, doCall *mock.Call)
		afterAttemptMatcher func(e *request.Execution) bool
	}{
		{
			name: "cancel Do",
			setupFirstDoCall: func(t *testing.T, doCall *mock.Call) {
				doCall.
					Run(func(args mock.Arguments) {
						req := args.Get(0).(*http.Request)
						ctx := req.Context()
						<-ctx.Done()
					}).
					Return(nil, context.Canceled).
					Once()
			},
			afterAttemptMatcher: func(e *request.Execution) bool {
				return e.Response == nil && e.Body == nil
			},
		},
		{
			name: "cancel read body",
			setupFirstDoCall: func(t *testing.T, doCall *mock.Call) {
				var ctx context.Context
				mockBody := newMockReadCloser(t)
				mockBody.On("Read", mock.Anything).
					Run(func(_ mock.Arguments) {
						<-ctx.Done()
					}).
					Return(0, context.Canceled).
					Once()
				mockBody.On("Close").
					Return(nil).
					Once()
				doCall.
					Run(func(args mock.Arguments) {
						req := args.Get(0).(*http.Request)
						ctx = req.Context()
					}).
					Return(&http.Response{StatusCode: 200, Body: mockBody}, nil).
					Once()
			},
			afterAttemptMatcher: func(e *request.Execution) bool {
				return e.StatusCode() == 200 && e.Response != nil && len(e.Body) == 0
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			doer := newMockHTTPDoer(t)
			retryPolicy := newMockRetryPolicy(t)
			racingPolicy := newMockRacingPolicy(t)
			cl := Client{
				HTTPDoer:      doer,
				TimeoutPolicy: timeout.Infinite,
				RetryPolicy:   retryPolicy,
				RacingPolicy:  racingPolicy,
				Handlers:      &HandlerGroup{},
			}
			callOrder := callOrderMatcher{}
			firstDoCall := doer.On("Do", mock.MatchedBy(func(_ *http.Request) bool { return callOrder.Match(0) }))
			testCase.setupFirstDoCall(t, firstDoCall)
			doer.On("Do", mock.MatchedBy(func(_ *http.Request) bool { return callOrder.Match(1) })).
				Return(&http.Response{StatusCode: 418, Body: ioutil.NopCloser(strings.NewReader("I'm a teapot"))}, nil).
				Once()
			retryPolicy.On("Decide", mock.AnythingOfType("*request.Execution")).
				Return(false).
				Once()
			racingPolicy.On("Schedule", mock.MatchedBy(func(e *request.Execution) bool {
				return e.Attempt == 0 && e.Wave == 0 && e.Racing == 1
			})).Return(time.Microsecond).Once()
			racingPolicy.On("Schedule", mock.MatchedBy(func(e *request.Execution) bool {
				return e.Attempt == 1 && e.Wave == 0 && e.Racing == 2
			})).Return(time.Duration(0)).Once()
			racingPolicy.On("Start", mock.MatchedBy(func(e *request.Execution) bool {
				return e.Attempt == 0 && e.Wave == 0 && e.Racing == 1
			})).Return(true).Once()
			cl.Handlers.mock(AfterAttempt).On("Handle", AfterAttempt, mock.MatchedBy(func(e *request.Execution) bool {
				return e.StatusCode() == 418 && bytes.Equal(e.Body, []byte("I'm a teapot")) && e.Err == nil && e.Wave == 0
			})).Once()
			cl.Handlers.mock(AfterAttempt).On("Handle", AfterAttempt, mock.MatchedBy(func(e *request.Execution) bool {
				return errors.Unwrap(e.Err) == racing.Redundant && e.Wave == 0 && testCase.afterAttemptMatcher(e)
			})).Once()

			e, err := cl.Get("test")

			doer.AssertExpectations(t)
			retryPolicy.AssertExpectations(t)
			racingPolicy.AssertExpectations(t)
			cl.Handlers.assertExpectations(t)
			assert.NoError(t, err)
			require.NotNil(t, e)
			assert.Equal(t, 418, e.StatusCode())
			assert.Equal(t, []byte("I'm a teapot"), e.Body)
		})
	}
}

func testRacingPlanCancelDuringWaveLoop(t *testing.T) {
	// This test cancels the plan context while the wave loop is in the
	// process of racing attempts in the wave. It verifies that the wave
	// ends and that the client doesn't continue generating new attempts.
	// -----------------------------------------------------------------
	N := 5 // Number of attempts to start before cancelling plan context.
	planCtx, cancelPlanCtx := context.WithCancel(context.Background())
	defer cancelPlanCtx()
	doer := newMockHTTPDoer(t)
	retryPolicy := newMockRetryPolicy(t)
	racingPolicy := newMockRacingPolicy(t)
	cl := Client{
		HTTPDoer:      doer,
		TimeoutPolicy: timeout.Infinite,
		RetryPolicy:   retryPolicy,
		RacingPolicy:  racingPolicy,
		Handlers:      &HandlerGroup{},
	}
	cancelRequest := make(chan time.Time, N)
	defer close(cancelRequest)
	cl.Handlers.mock(BeforeAttempt).
		On("Handle", BeforeAttempt, mock.MatchedBy(func(e *request.Execution) bool {
			return e.Wave == 0
		})).
		Run(func(args mock.Arguments) {
			e := args.Get(1).(*request.Execution)
			if e.Attempt == N-1 {
				cancelPlanCtx()
				now := time.Now()
				for i := 0; i < N; i++ {
					cancelRequest <- now
				}
			} else if e.Attempt >= N {
				cancelRequest <- time.Now()
			}
		})
	doer.On("Do", mock.AnythingOfType("*http.Request")).
		WaitUntil(cancelRequest).
		Return(nil, context.Canceled)
	racingPolicy.On("Schedule", mock.AnythingOfType("*request.Execution")).
		Return(100 * time.Microsecond)
	racingPolicy.On("Start", mock.AnythingOfType("*request.Execution")).
		Return(true)

	p, err := request.NewPlanWithContext(planCtx, "DELETE", "test", nil)
	require.NoError(t, err)
	e, err := cl.Do(p)

	doer.AssertExpectations(t)
	racingPolicy.AssertExpectations(t)
	cl.Handlers.assertExpectations(t)
	assert.Error(t, err)
	var urlError *url.Error
	require.ErrorAs(t, err, &urlError)
	assert.Same(t, context.Canceled, urlError.Unwrap())
	require.NotNil(t, e)
	assert.Same(t, err, e.Err)
	assert.Equal(t, 0, e.Wave)
	assert.GreaterOrEqual(t, e.Attempt, 0)
	assert.LessOrEqual(t, e.Attempt, N+5)
}

func testClientRacingPanic(t *testing.T) {
	// This test starts multiple concurrent racing attempts and then
	// triggers at various points both on the robust client Do request's
	// main goroutine and within the attempt send/read goroutines.

	N := 10 // Number of attempts to start before panicking

	// Function used to setup the HTTPDoer to block until N is reached then
	// return a response.
	setupHTTPDoerBlock := func(_ *testing.T, doer *mockHTTPDoer, ch <-chan time.Time) {
		doer.On("Do", mock.AnythingOfType("*http.Request")).
			WaitUntil(ch).
			Return(&http.Response{StatusCode: 218, Body: ioutil.NopCloser(strings.NewReader("This is fine."))}, nil).
			Times(N)
	}

	// Function used to setup the HTTPDoer to block until N is reached then
	// panic.
	setupHTTPDoerSendPanic := func(_ *testing.T, doer *mockHTTPDoer, ch <-chan time.Time) {
		doer.On("Do", mock.AnythingOfType("*http.Request")).
			WaitUntil(ch).
			Panic("mock HTTP doer panic!").
			Times(N)
	}

	// Function used to setup the HTTPDoer to block until N is reached, then
	// return a body reader that panics.
	setupHTTPDoerReadPanic := func(t *testing.T, doer *mockHTTPDoer, ch <-chan time.Time) {
		mockBody := newMockReadCloser(t)
		mockBody.On("Read", mock.Anything).
			Panic("mock body panic!").
			Times(N)
		mockBody.On("Close").
			Return(nil)
		doer.On("Do", mock.AnythingOfType("*http.Request")).
			WaitUntil(ch).
			Return(&http.Response{StatusCode: 218, Body: mockBody}, nil).
			Times(N)
	}

	// Function used to configure an event handler to panic.
	setupEventHandlerPanic := func(handlers *HandlerGroup, event Event) {
		handlers.mock(event).
			On("Handle", event, mock.AnythingOfType("*request.Execution")).
			Panic("event handler panic - " + event.Name() + "!").
			Once()
	}

	testCases := []struct {
		name              string
		setupHTTPDoer     func(t *testing.T, doer *mockHTTPDoer, ch <-chan time.Time)
		setupEventHandler func(handlers *HandlerGroup, event Event)
		event             Event
		panicVal          string
	}{
		{
			name:              "BeforeReadBody handler",
			setupHTTPDoer:     setupHTTPDoerBlock,
			setupEventHandler: setupEventHandlerPanic,
			event:             BeforeReadBody,
			panicVal:          "event handler panic - BeforeReadBody!",
		},
		{
			name:              "AfterAttempt handler",
			setupHTTPDoer:     setupHTTPDoerBlock,
			setupEventHandler: setupEventHandlerPanic,
			event:             AfterAttempt,
			panicVal:          "event handler panic - AfterAttempt!",
		},
		{
			name:          "send",
			setupHTTPDoer: setupHTTPDoerSendPanic,
			panicVal:      "mock HTTP doer panic!",
		},
		{
			name:          "read body",
			setupHTTPDoer: setupHTTPDoerReadPanic,
			panicVal:      "mock body panic!",
		},
	}

	schedule := make([]time.Duration, N)
	for i := 0; i < N; i++ {
		schedule[i] = time.Duration(i) * 50 * time.Microsecond
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			doer := newMockHTTPDoer(t)
			retryPolicy := newMockRetryPolicy(t)
			cl := Client{
				HTTPDoer:      doer,
				TimeoutPolicy: timeout.Infinite,
				RetryPolicy:   retryPolicy,
				RacingPolicy:  racing.NewPolicy(racing.NewStaticScheduler(schedule...), racing.AlwaysStart),
				Handlers:      &HandlerGroup{},
			}
			showtime := make(chan time.Time, N)
			//			defer close(showtime)
			testCase.setupHTTPDoer(t, doer, showtime)
			cl.Handlers.mock(BeforeAttempt).
				On("Handle", BeforeAttempt, mock.MatchedBy(func(e *request.Execution) bool {
					return e.Wave == 0
				})).
				Run(func(args mock.Arguments) {
					e := args.Get(1).(*request.Execution)
					if e.Attempt == N-1 {
						now := time.Now()
						for i := 0; i < N; i++ {
							showtime <- now
						}
					} else if e.Attempt >= N {
						showtime <- time.Now()
					}
				})
			if testCase.setupEventHandler != nil {
				testCase.setupEventHandler(cl.Handlers, testCase.event)
			}

			assert.PanicsWithValue(t, testCase.panicVal, func() {
				_, _ = cl.Get("test")
			})

			doer.AssertExpectations(t)
			cl.Handlers.assertExpectations(t)
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

type callOrderMatcher struct {
	counter int32
}

func (com *callOrderMatcher) Match(x int32) bool {
	return atomic.CompareAndSwapInt32(&com.counter, x, x+1)
}
