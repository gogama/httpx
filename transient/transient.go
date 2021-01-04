// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package transient

import (
	"errors"
	"syscall"
)

// A Category is the transience category of a particular error, as
// reported by function Categorize().
//
// The category Not means the error is not transient from the perspective
// of completing an HTTP request attempt successfully, or in other words
// that a retry after encountering this error is very unlikely to succeed.
//
// All other categories indicate the error is transient from the
// perspective of completing an HTTP request attempt successfully, or in
// other words that a retry after encountering this error has some
// prospect of success.
type Category int

const (
	// Not indicates any non-transient error.
	Not Category = iota
	// Timeout indicates a client-side timeout. The server may be going
	// through a temporary period of slowness, or the client may succeed
	// on a future attempt waiting longer (increasing its timeout).
	//
	// Function Categorize() will return Timeout if the error or any of
	// its wrapped causes has a Timeout() function that reports true.
	Timeout
	// ConnRefused indicates the remote host refused the connection, and
	// corresponds to the POSIX error code ECONNRESET.
	//
	// Although connection refusal may be a permanent condition, it is
	// classified as transient because it can happen if the service
	// running on the remote host is in the process of starting or
	// restarting. In this case the service is temporarily not listening
	// on the specified port, but will be once its startup is complete.
	//
	// Function Categorize() will return ConnRefused if the error is not
	// a Timeout, and the error or any of its wrapped causes is equal to
	// syscall.ECONNREFUSED.
	ConnRefused
	// ConnReset indicates the remote host returned an RST packet on a
	// previously active TCP connection, and corresponds to the POSIX
	// error code ECONNRESET.
	//
	// Connection reset is not uncommon if, due to poor deployment
	// processes, a service on the remote host comes down prematurely
	// (i.e. while it is still in the process of responding to a
	// request). As well it may happen in a variety of cases where the
	// remote host is a load balancer. For these reasons, a connection
	// reset tends to indicate a high probability of success on retry.
	//
	// Function Categorize() will return ConnReset if the error is not a
	// Timeout, and the error or any of its wrapped causes is equal to
	// syscall.ECONNRESET.
	ConnReset
)

// Categorize returns the transience category of the given error. All
// non-nil transient errors result in a transience category other than
// Not. A nil error, and an error that is not transient from the
// perspective of completing an HTTP request attempt, both produce the
// return value Not.
//
// In assessing transience, Categorize looks at wrapped cause errors
// contained within err, not just err itself. However, Categorize never
// checks if an error has a Temporary() function that returns true, as
// the semantics of Temporary() aren't entirely clear.
func Categorize(err error) Category {
	if err == nil {
		return Not
	}

	var hasTimeout hasTimeout
	if errors.As(err, &hasTimeout) && hasTimeout.Timeout() {
		return Timeout
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		if errno == syscall.ECONNRESET {
			return ConnReset
		} else if errno == syscall.ECONNREFUSED {
			return ConnRefused
		}
	}

	return Not
}

type hasTimeout interface {
	Timeout() bool
}
