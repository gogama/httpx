// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package transient classifies errors from HTTP request execution as
// transient or non-transient. This is handy for writing retry policies,
// and for other purposes such as bucketing error metrics.
//
// Package transient is extremely lightweight, as it depends only on
// the standard library packages "errors" and "syscall", so it doesn't
// bring any significant dependencies when imported as a standalone
// package.
package transient
