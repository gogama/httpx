// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing

import (
	"time"

	"github.com/gogama/httpx/request"
)

type Scheduler interface {
	Schedule(*request.Execution) time.Duration
}
