// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Package timeout allows defining flexible policies for controlling how
// HTTP timeouts are set during an HTTP request plan execution, including
// on retries. A generic interface for timeout policies is provided, Policy,
// along with several useful policy generating functions and built-in policies.
package timeout
