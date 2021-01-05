// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package request

import (
	"errors"
	"io"
	"io/ioutil"
)

const badBodyTypeMsg = "httpx/request: invalid type (for body use nil, " +
	"string, []byte, io.Reader or io.ReadCloser)"

// BodyBytes converts a generic body parameter to a byte slice for use
// as a request plan body.
//
// The body parameter may be nil, or it may be a string, []byte,
// io.Reader, or io.ReadCloser. The conversion logic is:
//
// • If body is nil, a nil byte slice and no error is returned.
//
// • If body is a []byte, body itself and no error is returned.
//
// • If body is a string, the built-in conversion from string to byte
// slice, and no error, is returned.
//
// • If body is an io.Reader or io.ReadCloser, the result of reading
// the whole contents of the reader (and closing it if it implements
// Closer) is returned. If reading from the reader (and closing it if
// applicable) causes an error, the return value is a nil byte slice
// and the error. Otherwise, the result is the entire contents read
// from the reader and no error.
//
// • If body is any other type than those listed above, a nil byte slice
// and an error is returned.
func BodyBytes(body interface{}) ([]byte, error) {
	switch x := body.(type) {
	case nil:
		return nil, nil
	case string:
		return []byte(x), nil
	case []byte:
		return x, nil
	case io.ReadCloser:
		b, err := ioutil.ReadAll(x)
		if err != nil {
			return nil, err
		}
		err = x.Close()
		if err != nil {
			return nil, err
		}
		return b, nil
	case io.Reader:
		return BodyBytes(ioutil.NopCloser(x))
	default:
		return nil, errors.New(badBodyTypeMsg)
	}
}
