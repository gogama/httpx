// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import "errors"

// Redundant is the error set on Execution.Err when the request attempt
// is cancelled as redundant.
//
// Once any request attempt has reached a final (non-retryable) outcome,
// all other outstanding concurrent attempts racing in the same wave are
// cancelled as redundant.
var Redundant = errors.New("httpx/racing: redundant attempt")
