// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package transient

import (
	"errors"
	"fmt"
	"net/url"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCategorize(t *testing.T) {
	assert.Equal(t, Not, Categorize(nil))
	assert.Equal(t, Not, Categorize(errors.New("foo")))
	assert.Equal(t, Not, Categorize(wrapper{}))
	assert.Equal(t, Not, Categorize(wrapper{errors.New("bar")}))
	assert.Equal(t, Timeout, Categorize(syscall.ETIMEDOUT))
	assert.Equal(t, Timeout, Categorize(timeout{}))
	assert.Equal(t, Timeout, Categorize(&url.Error{Err: syscall.ETIMEDOUT}))
	assert.Equal(t, Timeout, Categorize(&url.Error{Err: timeout{}}))
	assert.Equal(t, Timeout, Categorize(wrapper{&url.Error{Err: syscall.ETIMEDOUT}}))
	assert.Equal(t, Timeout, Categorize(wrapper{wrapper{timeout{}}}))
	assert.Equal(t, Timeout, Categorize(timeoutWrapper{true, syscall.ECONNRESET}))
	assert.Equal(t, Timeout, Categorize(wrapper{timeoutWrapper{true, syscall.ECONNREFUSED}}))
	assert.Equal(t, ConnReset, Categorize(syscall.ECONNRESET))
	assert.Equal(t, ConnReset, Categorize(wrapper{syscall.ECONNRESET}))
	assert.Equal(t, ConnReset, Categorize(timeoutWrapper{false, syscall.ECONNRESET}))
	assert.Equal(t, ConnRefused, Categorize(syscall.ECONNREFUSED))
	assert.Equal(t, ConnRefused, Categorize(wrapper{syscall.ECONNREFUSED}))
	assert.Equal(t, ConnRefused, Categorize(&url.Error{Err: wrapper{timeoutWrapper{false, syscall.ECONNREFUSED}}}))
}

type timeout struct{}

func (err timeout) Error() string {
	return "timeout"
}

func (_ timeout) Timeout() bool {
	return true
}

type wrapper struct {
	wrappedError error
}

func (err wrapper) Error() string {
	return fmt.Sprintf("wrapper - wraps %v", err.wrappedError)
}

func (err wrapper) Unwrap() error {
	return err.wrappedError
}

type timeoutWrapper struct {
	timeout      bool
	wrappedError error
}

func (err timeoutWrapper) Error() string {
	return fmt.Sprintf("timeoutWrapper - timeout %t, wraps %v", err.timeout, err.wrappedError)
}

func (err timeoutWrapper) Timeout() bool {
	return err.timeout
}

func (err timeoutWrapper) Unwrap() error {
	return err.wrappedError
}
