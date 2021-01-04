// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package retry allows defining flexible policies for deciding when to
// retry a failed attempt during an HTTP request plan execution, and how
// long to wait before retrying.
//
// The interface Policy defines a retry Policy. A Policy instance can be
// constructed using NewPolicy by providing a decision-making function,
// DeciderFunc, and a wait time calculator, Waiter. Both DeciderFunc and Waiter
// have constructors for common use cases, so that a custom policy can
// be quickly assembled:
//
//     decider := retry.Times(3).
//                    And(retry.Before(5 * time.Second)).
//                    And(retry.StatusCode(500).Or(retry.TransientErr))
//     waiter := retry.NewExpWaiter{100 * time.Millisecond, 2 * time.Second}
//     policy := retry.NewPolicy(d, w)
package retry
