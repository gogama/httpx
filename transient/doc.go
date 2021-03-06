// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package transient classifies errors from HTTP request execution as
// transient or non-transient. This is handy for writing retry policies,
// and for other purposes such as bucketing error metrics.
//
// Package transient is very lightweight, as it depends only on the
// standard library packages "errors" and "syscall". It can be used
// standalone from the rest of the httpx package for applications that
// need transience classification and don't want to bring in a cascade
// of dependencies.
package transient
