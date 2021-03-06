// Copyright 2021 The httpx Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package httpx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvents(t *testing.T) {
	assert.Len(t, eventNames, numEvents)
	assert.Len(t, Events(), numEvents)
	events := Events()
	assert.Equal(t, BeforeExecutionStart, events[BeforeExecutionStart])
	assert.Equal(t, BeforeAttempt, events[BeforeAttempt])
	assert.Equal(t, BeforeReadBody, events[BeforeReadBody])
	assert.Equal(t, AfterAttemptTimeout, events[AfterAttemptTimeout])
	assert.Equal(t, AfterAttempt, events[AfterAttempt])
	assert.Equal(t, AfterPlanTimeout, events[AfterPlanTimeout])
	assert.Equal(t, AfterExecutionEnd, events[AfterExecutionEnd])
}

func TestEvent_String(t *testing.T) {
	assert.Equal(t, "BeforeExecutionStart", BeforeExecutionStart.String())
	assert.Equal(t, "BeforeAttempt", BeforeAttempt.String())
	assert.Equal(t, "BeforeReadBody", BeforeReadBody.String())
	assert.Equal(t, "AfterAttemptTimeout", AfterAttemptTimeout.String())
	assert.Equal(t, "AfterAttempt", AfterAttempt.String())
	assert.Equal(t, "AfterPlanTimeout", AfterPlanTimeout.String())
	assert.Equal(t, "AfterExecutionEnd", AfterExecutionEnd.String())
}
