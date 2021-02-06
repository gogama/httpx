// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
Package racing provides flexible policies for running multiple HTTP
requests simultaneously in a "race" to improve performance.

While racing is a powerful feature for smoothing over pockets of bad
server response times, it introduces risks which must be mitigated by
thoughtful policy design and tuning with real world data. In particular,
naively running multiple parallel requests may raise your costs, raise
the risk of browning out the remote service, waste resources, or cause
data consistency problems when making mutating requests.

The default racing policy used by the robust HTTP client is Disabled.
This policy disables racing and ensures HTTP all request attempts during
made while executing the request plan are serialized. Thus the default
behavior of the robust HTTP client fits the expectations of a typical
user.

The main concepts involved in racing are:

• A group of concurrent or racing request attempts is called a wave.
  Every wave starts with one request attempt and may grow as new
  attempts are started according to the Policy. A wave ends either when
  the first non-retryable attempt within the policy is detected, or when
  all concurrent attempts permitted by the policy have ended. A new wave
  begins if all attempts within the previous wave ended in a retryable
  state.

• A racing Policy decides when to add a new concurrent request attempt
  to the current wave. Policy decisions are broken down into two steps,
  scheduling and starting. Each time a new request attempt is added to
  the wave, the Policy is invoked to schedule the next request attempt.
  When the scheduled time occurs, the Policy is again invoked to decide
  whether the scheduled request attempt should really start, as
  circumstances may have changed in the meantime.

• Although each racing request executes on its own goroutine, the robust
  HTTP client dispatches every racing requests' events to the event
  handler on the main goroutine (the one that invoked the client). Thus
  even if multiple attempts are racing within a single request
  execution, the events for that execution are serialized and do not race.

• Attempt-level events (BeforeAttempt, BeforeReadBody, AfterAttempt,
  AfterAttemptTimeout) always occur in the correct order for a
  particular request attempt. The events of different attempts in the
  same wave may be interleaved, but the BeforeAttempt event always
  occurs for attempt `i` before it occurs for attempt `i+1`. All events
  for a request attempt in wave `j` occur before any events for an
  attempt in wave `j+1`.

• When a request attempt completes with a non-retryable result, every
  other concurrent attempt in the wave is cancelled as redundant. The
  AfterAttempt event handler is fired for every request that is
  cancelled as redundant, and the execution error during the event is
  set to Redundant.

Besides the Disabled policy, this package provides built-in constructors
for a scheduler and a starter. Use NewStaticScheduler to create a
scheduler based on a static offset schedule. Use NewThrottleStarter to
create a starter which can throttle racing if too many parallel request
attempts are being scheduled. Use NewPolicy to compose any scheduler and
any starter into a racnig policy.
*/
package racing
