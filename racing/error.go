// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import "errors"

// TODO: document me
var Redundant = errors.New("httpx/racing: redundant attempt")
