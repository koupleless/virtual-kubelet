package utils

import (
	"context"
	"gotest.tools/assert"
	"os"
	"testing"
	"time"
)

func TestTimedTaskWithInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testList := make([]int, 0)
	go TimedTaskWithInterval(ctx, time.Second, func(_ context.Context) {
		testList = append(testList, 0)
	})
	time.Sleep(2500 * time.Millisecond)
	assert.Assert(t, len(testList) == 3)
}

func TestTimedTaskWithInterval_cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testList := make([]int, 0)
	go func() {
		time.Sleep(time.Millisecond * 1010)
		cancel()
	}()
	TimedTaskWithInterval(ctx, time.Second, func(_ context.Context) {
		testList = append(testList, 0)
	})
	assert.Assert(t, len(testList) == 2)
}

func TestConvertByteNumToResourceQuantity(t *testing.T) {
	quantity := ConvertByteNumToResourceQuantity(-1)
	assert.Assert(t, quantity.IsZero())
	quantity = ConvertByteNumToResourceQuantity(1)
	assert.Assert(t, quantity.IsZero())
	quantity = ConvertByteNumToResourceQuantity(1025)
	assert.Assert(t, quantity.String() == "1Ki")
}

func TestCheckAndFinallyCall_timeout(t *testing.T) {
	success := false
	timeout := false
	CheckAndFinallyCall(func() bool {
		return success
	}, time.Second, time.Millisecond*100, func() {
		return
	}, func() {
		timeout = true
	})
	assert.Assert(t, timeout)
}

func TestCheckAndFinallyCall_success(t *testing.T) {
	success := false
	called := false
	go CheckAndFinallyCall(func() bool {
		return success
	}, time.Second, time.Millisecond*100, func() {
		called = true
		return
	}, func() {
		return
	})
	success = true
	time.Sleep(time.Millisecond * 200)
	assert.Assert(t, called)
}

func TestGetEnv(t *testing.T) {
	defaultValue := "TEST_ENV"
	value := GetEnv("TEST_ENV", "none")
	assert.Equal(t, value, "none")
	os.Setenv("TEST_ENV", defaultValue)
	value = GetEnv("TEST_ENV", "none")
	assert.Equal(t, value, defaultValue)
}
