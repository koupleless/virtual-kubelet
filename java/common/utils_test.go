package common

import (
	"context"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestTimedTaskWithInterval(t *testing.T) {
	testList := make([]int, 0)
	go TimedTaskWithInterval("test timed task", time.Second, func(_ context.Context) {
		testList = append(testList, 0)
	})
	time.Sleep(2001 * time.Millisecond)
	assert.Assert(t, len(testList) == 3)
}

func TestConvertByteNumToResourceQuantity(t *testing.T) {
	quantity := ConvertByteNumToResourceQuantity(-1)
	assert.Assert(t, quantity.IsZero())
	quantity = ConvertByteNumToResourceQuantity(1)
	assert.Assert(t, quantity.IsZero())
	quantity = ConvertByteNumToResourceQuantity(1025)
	assert.Assert(t, quantity.String() == "1Ki")
}
