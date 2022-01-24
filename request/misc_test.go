// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package request

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBodyBytes(t *testing.T) {
	var b []byte
	var err error
	t.Run("happy path", func(t *testing.T) {
		b, err = BodyBytes(nil)
		assert.Nil(t, b)
		assert.NoError(t, err)
		b, err = BodyBytes("foo")
		assert.Equal(t, []byte("foo"), b)
		assert.NoError(t, err)
		b2 := []byte("bar")
		b, err = BodyBytes(b2)
		assert.Equal(t, []byte("bar"), b)
		assert.Equal(t, b, b2)
		b, err = BodyBytes(strings.NewReader("baz"))
		assert.Equal(t, []byte("baz"), b)
		assert.NoError(t, err)
		b, err = BodyBytes(ioutil.NopCloser(bytes.NewReader(b2)))
		assert.Equal(t, []byte("bar"), b)
		assert.NoError(t, err)
		b, err = BodyBytes(10)
		assert.Nil(t, b)
		assert.EqualError(t, err, badBodyTypeMsg)
	})
	t.Run("reader errors", func(t *testing.T) {
		expectedErr := errors.New("ham")
		t.Run("Read", func(t *testing.T) {
			m := &mockReadCloser{}
			m.Test(t)
			m.On("Read", mock.Anything).Return(10, expectedErr).Once()
			b, err = BodyBytes(m)
			assert.Nil(t, b)
			assert.Error(t, err)
			assert.Same(t, expectedErr, err)
			m.AssertExpectations(t)
		})
		t.Run("Close", func(t *testing.T) {
			m := &mockReadCloser{}
			m.Test(t)
			m.On("Read", mock.Anything).Return(0, io.EOF).Once()
			m.On("Close").Return(expectedErr).Once()
			b, err = BodyBytes(m)
			assert.Nil(t, b)
			assert.Error(t, err)
			assert.Same(t, expectedErr, err)
			m.AssertExpectations(t)
		})
	})
}

type mockReadCloser struct {
	mock.Mock
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
