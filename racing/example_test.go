// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package racing_test

import (
	"fmt"
	"time"

	"github.com/gogama/httpx/request"

	"github.com/gogama/httpx/racing"
)

func ExampleNewStaticScheduler() {
	sc := racing.NewStaticScheduler(250*time.Millisecond, 500*time.Millisecond, 1500*time.Millisecond)
	// Simulate running scheduler with 0, 1, 2, and 3 request attempts
	// already racing.
	var e request.Execution
	for i := 0; i <= 3; i++ {
		e.Racing = i
		fmt.Println(sc.Schedule(&e))
	}
	// Output:
	// 250ms
	// 500ms
	// 1.5s
	// 0s
}
