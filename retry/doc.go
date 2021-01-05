// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package retry provides flexible policies for retrying failed attempts
// during an HTTP request plan execution, and how long to wait before
// retrying.
//
// The interface Policy defines a retry Policy. A Policy instance can be
// constructed using NewPolicy by providing a decision-maker, Decider,
// and a wait time calculator, Waiter. Both Decider and Waiter have
// constructors for common use cases, so that a useful policy can be
// quickly assembled:
//
//     decider := retry.Times(3).
//                    And(retry.Before(5 * time.Second)).
//                    And(retry.StatusCode(500).Or(retry.TransientErr))
//     waiter := retry.NewExpWaiter{100 * time.Millisecond, 2 * time.Second}
//     policy := retry.NewPolicy(d, w)
//
// If the built-in functionality is insufficient, fully custom retry
// policies can be created by via custom implementations of Decider,
// Waiter, or Policy.
package retry
